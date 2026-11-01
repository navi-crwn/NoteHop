package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/navi-crwn/NoteHop/internal/db"
)

// Accounts as the control center and the account server see them: a
// summary per user, admin actions, profile pushes and account events. The
// account server owns the account; NoteHop keeps a copy of the profile and
// decides the local role from the roles it is given.

func (a *App) hopUserView(u *db.User) map[string]any {
	c, _ := a.db.UserCounts(u.ID)
	return map[string]any{
		"sub": u.SSOID, "id": hopUserID(u), "local_id": u.ID, "username": u.Username, "email": u.Email, "display_name": u.DisplayName,
		"avatar": a.avatarURL(u.Avatar), "role": u.Role, "status": u.Status, "sso": u.SSOID != "", "vault_set": u.VaultCheck != "" || u.AuthHash != "",
		"notes": c.Notes, "notes_public": c.NotesPublic, "sessions": c.Sessions, "created_at": iso(u.Created),
		"url": a.cfg.BaseURL + "/@" + u.Username,
	}
}

func (a *App) hopUsers(w http.ResponseWriter, r *http.Request, _ []byte) {
	cursor, limit := hopPaging(r)
	users, err := a.db.ListUsers(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))), cursor, limit)
	if err != nil {
		hopServerErr(w, "users", err)
		return
	}
	out := make([]map[string]any, 0, len(users))
	for i := range users {
		out = append(out, a.hopUserView(&users[i]))
	}
	next := ""
	if len(users) == limit {
		next = strconv.FormatInt(users[len(users)-1].ID, 10)
	}
	hopList(w, out, next, 0)
}

func (a *App) hopUser(w http.ResponseWriter, r *http.Request, _ []byte) {
	u, err := a.hopFindUser(r.PathValue("id"))
	if errors.Is(err, db.ErrNotFound) {
		hopFail(w, http.StatusNotFound, "not_found", "no account with that id here")
		return
	}
	if err != nil {
		hopServerErr(w, "user", err)
		return
	}
	hopOK(w, a.hopUserView(u))
}

func (a *App) hopUserAction(w http.ResponseWriter, r *http.Request, body []byte) {
	u, err := a.hopFindUser(r.PathValue("id"))
	if errors.Is(err, db.ErrNotFound) {
		hopFail(w, http.StatusNotFound, "not_found", "no account with that id here")
		return
	}
	if err != nil {
		hopServerErr(w, "user", err)
		return
	}
	var q actionReq
	if !hopBody(w, body, &q) {
		return
	}
	result, err := a.userAction(u, q)
	if errors.Is(err, errHopUnknownAction) {
		hopFail(w, http.StatusBadRequest, "unknown_action", "that action does not exist for users")
		return
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), "bad_") {
			hopFail(w, http.StatusBadRequest, err.Error(), "the action's parameters are not valid")
			return
		}
		hopServerErr(w, "user action "+q.Action, err)
		return
	}
	a.audit(r, "user."+q.Action, hopUserID(u), map[string]any{"reason": clip(q.Reason, 500), "params": q.Params, "result": result})
	hopOK(w, result)
}

// userAction runs one admin action on an account and returns what it did.
func (a *App) userAction(u *db.User, q actionReq) (map[string]any, error) {
	switch q.Action {
	case "revoke_sessions":
		return map[string]any{"revoked": true}, a.db.DeleteOtherSessions(u.ID, "")
	case "delete_notes":
		n, err := a.db.DeleteNotesByOwner(u.ID)
		return map[string]any{"deleted_notes": n}, err
	case "purge":
		return a.purgeUser(u)
	case "set_role":
		role := q.str("role")
		if role != db.RoleAdmin && role != db.RoleUser {
			return nil, errors.New("bad_role")
		}
		return map[string]any{"role": role}, a.db.SetRole(u.ID, role)
	case "reset_vault":
		if u.SSOID == "" {
			return nil, errors.New("bad_account") // password accounts derive the vault key from the password
		}
		if err := a.db.ClearVaultCheck(u.ID); err != nil {
			return nil, err
		}
		return map[string]any{"vault_reset": true}, a.db.DeleteOtherSessions(u.ID, "")
	}
	return nil, errHopUnknownAction
}

// purgeUser removes everything about an account: notes, sessions and the
// account itself. This is what account.deleted asks for.
func (a *App) purgeUser(u *db.User) (map[string]any, error) {
	notes, err := a.db.DeleteNotesByOwner(u.ID)
	if err != nil {
		return nil, err
	}
	if err := a.db.DeleteUser(u.ID); err != nil {
		return nil, err
	}
	return map[string]any{"purged": true, "counts": map[string]any{"notes": notes, "users": 1}}, nil
}

// ---------- profile push ----------

type hopProfile struct {
	Sub            string   `json:"sub"`
	HopID          string   `json:"hop_id"`
	Username       string   `json:"username"`
	DisplayName    string   `json:"display_name"`
	Name           string   `json:"name"`
	Email          string   `json:"email"`
	Avatar         string   `json:"avatar"`
	AvatarURL      string   `json:"avatar_url"`
	Roles          []string `json:"roles"`
	Status         string   `json:"status"`
	ProfileVersion int      `json:"profile_version"`
}

func (p hopProfile) id() string {
	if p.Sub != "" {
		return p.Sub
	}
	return p.HopID
}

func (a *App) hopProfile(w http.ResponseWriter, r *http.Request, body []byte) {
	var p hopProfile
	if !hopBody(w, body, &p) {
		return
	}
	res, err := a.applyProfile(p)
	if errors.Is(err, db.ErrNotFound) {
		hopFail(w, http.StatusNotFound, "not_found", "no account with that id here; it is made at first sign-on")
		return
	}
	if err != nil {
		hopServerErr(w, "profile", err)
		return
	}
	a.audit(r, "profile.update", p.id(), res)
	hopOK(w, res)
}

// applyProfile updates the local copy of an account's profile. It never
// creates an account (that happens at first sign-on). The username only
// changes when the new one passes the name rules and is free; otherwise the
// old one stays and the reply says so.
func (a *App) applyProfile(p hopProfile) (map[string]any, error) {
	if p.id() == "" {
		return nil, db.ErrNotFound
	}
	u, err := a.db.UserBySSO(p.id())
	if err != nil {
		return nil, err
	}
	res := map[string]any{"sub": u.SSOID, "local_id": u.ID}
	if p.Name != "" && p.DisplayName == "" {
		p.DisplayName = p.Name
	}
	if p.Avatar == "" {
		p.Avatar = p.AvatarURL
	}
	u.Email, u.DisplayName, u.Avatar = clip(p.Email, 254), clip(p.DisplayName, 60), a.avatarURL(p.Avatar)
	if err := a.db.UpdateSSOProfile(u); err != nil {
		return nil, err
	}
	if name := strings.ToLower(strings.TrimSpace(p.Username)); name != "" && name != u.Username {
		kept := true
		if userRe.MatchString(name) && a.usernameProblem(name) == "" {
			switch err := a.db.SetUsername(u.ID, name); {
			case err == nil:
				kept = false
				u.Username = name
			case !errors.Is(err, db.ErrTaken):
				return nil, err
			}
		}
		res["username_kept"] = kept
	}
	if p.Roles != nil {
		role := a.roleFromRoles(p.Roles)
		if role != u.Role {
			if err := a.db.SetRole(u.ID, role); err != nil {
				return nil, err
			}
			u.Role = role
		}
	}
	if p.Status != "" {
		if err := a.setStatus(u, p.Status); err != nil {
			return nil, err
		}
	}
	res["username"], res["role"], res["status"] = u.Username, u.Role, u.Status
	return res, nil
}

// setStatus marks an account suspended (locked and banned count as
// suspended here: the account can't sign in) or active, and signs a
// suspended account out everywhere.
func (a *App) setStatus(u *db.User, status string) error {
	s := db.StatusActive
	switch status {
	case "suspended", "locked", "banned", "disabled":
		s = db.StatusSuspended
	}
	if s == u.Status {
		return nil
	}
	if err := a.db.SetStatus(u.ID, s); err != nil {
		return err
	}
	u.Status = s
	if s == db.StatusSuspended {
		return a.db.DeleteOtherSessions(u.ID, "")
	}
	return nil
}

// ---------- events from the account server ----------

type hopEvent struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	At   string          `json:"at"`
	Data json.RawMessage `json:"data"`
}

func (a *App) hopEvent(w http.ResponseWriter, r *http.Request, body []byte) {
	var e hopEvent
	if !hopBody(w, body, &e) {
		return
	}
	if e.ID == "" || e.Type == "" {
		hopFail(w, http.StatusBadRequest, "bad_request", "an event needs an id and a type")
		return
	}
	first, err := a.db.FirstEvent(e.ID)
	if err != nil {
		hopServerErr(w, "event", err)
		return
	}
	if !first {
		hopOK(w, map[string]any{"duplicate": true})
		return
	}
	res, err := a.applyEvent(e)
	if errors.Is(err, db.ErrNotFound) {
		// Nothing here for that account: the event is done as far as we're concerned.
		hopOK(w, map[string]any{"ignored": true, "reason": "no such account here"})
		return
	}
	if err != nil {
		a.db.ForgetEvent(e.ID) // so the account server's retry gets another go
		hopServerErr(w, "event "+e.Type, err)
		return
	}
	a.audit(r, "event."+e.Type, e.ID, res)
	hopOK(w, res)
}

func (a *App) applyEvent(e hopEvent) (map[string]any, error) {
	var d struct {
		hopProfile
		Profile *hopProfile `json:"profile"`
	}
	_ = json.Unmarshal(e.Data, &d)
	p := d.hopProfile
	if d.Profile != nil {
		p = *d.Profile
	}
	switch e.Type {
	case "profile.updated":
		return a.applyProfile(p)
	case "account.status":
		u, err := a.db.UserBySSO(p.id())
		if err != nil {
			return nil, err
		}
		if err := a.setStatus(u, p.Status); err != nil {
			return nil, err
		}
		return map[string]any{"sub": u.SSOID, "status": u.Status}, nil
	case "account.deleted":
		u, err := a.db.UserBySSO(p.id())
		if err != nil {
			return nil, err
		}
		return a.purgeUser(u)
	case "session.revoked":
		u, err := a.db.UserBySSO(p.id())
		if err != nil {
			return nil, err
		}
		return map[string]any{"sub": u.SSOID, "revoked": true}, a.db.DeleteOtherSessions(u.ID, "")
	case "app.secret_rotated":
		// The secret comes from the environment here; nothing to do but acknowledge.
		return map[string]any{"noted": true}, nil
	}
	return map[string]any{"ignored": true, "reason": "unknown event type"}, nil
}
