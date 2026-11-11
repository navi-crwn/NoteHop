package app

import "net/http"

func (a *App) storageAvailable(incoming int64) bool {
	free, total := a.space(a.cfg.StoragePath)
	mb, percent := a.cfg.MinFreeMB, a.cfg.MinFreePercent
	if mb <= 0 {
		mb = 1024
	}
	if percent <= 0 {
		percent = 5
	}
	reserve := max(int64(mb)<<20, total/100*int64(percent))
	// Allow for the SQLite blob plus WAL and checkpoint overhead.
	return total > 0 && free > reserve && incoming <= (free-reserve)/3
}

func (a *App) apiFileStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var used int64
	err := a.db.QueryRow(`SELECT COALESCE(SUM(size), 0) FROM files`).Scan(&used)
	code := ""
	switch {
	case a.cfg.FileMB <= 0:
		code = "err.files_off"
	case err != nil || !a.storageAvailable(int64(a.cfg.FileMB)<<20):
		code = "err.files_storage"
	case used >= int64(a.cfg.FilesTotalMB)<<20:
		code = "err.files_full"
	}
	out := map[string]any{"note_limit_bytes": 5 << 20, "total_limit_bytes": int64(a.cfg.FilesTotalMB) << 20, "ttl_hours": int(a.cfg.FileTTL.Hours())}
	// A signed-in person also sees what their account holds against its limit.
	if u := a.currentUser(r); u != nil && a.cfg.FilesAccountMB > 0 {
		held, _ := a.db.AccountFileBytes(u.ID)
		limit := int64(a.cfg.FilesAccountMB) << 20
		out["account_used_bytes"], out["account_limit_bytes"] = held, limit
		if code == "" && held >= limit {
			code = "err.files_account_quota"
		}
	}
	out["available"], out["code"] = code == "", code
	writeJSON(w, http.StatusOK, out)
}
