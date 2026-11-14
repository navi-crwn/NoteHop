package app

import "testing"

// The control center's reset_vault does what the account's own reset does: the encrypted notes sealed with the lost
// phrase are deleted (with their attachments and links), basic notes stay, the vault generation goes up, every
// session ends, and exactly one audit line is written. A password account is refused and nothing changes.
func TestHopResetVaultWipesTheVaultLikeTheOwnReset(t *testing.T) {
	srv, _, d := hopApp(t, Config{})
	u := ssoUser(t, srv, "51", "helena@example.com", nil)
	u.create(nil)
	u.create(nil)
	u.create(map[string]any{"slug": "keep-helena"})
	d.Exec(`UPDATE users SET vault_check = 'x', vault_wrap = 'w' WHERE sso_id = '51'`)
	count := func(q string) (n int) {
		d.QueryRow(q).Scan(&n)
		return n
	}
	if count(`SELECT COUNT(*) FROM notes WHERE encrypted = 1`) != 2 {
		t.Fatal("setup")
	}
	code, env := call(t, srv, "imperium", "POST", "/hop/users/51/actions", map[string]any{"action": "reset_vault", "reason": "forgot the phrase"})
	if code != 200 {
		t.Fatalf("reset_vault: %d %v", code, env)
	}
	x := data(env)
	cm, _ := x["counts"].(map[string]any)
	if x["vault_reset"] != true || cm["notes"] != float64(2) {
		t.Fatalf("answer: %v", x)
	}
	if n := count(`SELECT COUNT(*) FROM notes WHERE encrypted = 1`); n != 0 {
		t.Fatalf("%d encrypted notes left behind, orphaned for ever", n)
	}
	if n := count(`SELECT COUNT(*) FROM notes WHERE encrypted = 0`); n != 1 {
		t.Fatalf("the basic note must stay: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM links l WHERE NOT EXISTS (SELECT 1 FROM notes n WHERE n.id = l.note_id)`); n != 0 {
		t.Fatalf("%d links of deleted notes left", n)
	}
	if n := count(`SELECT vault_epoch FROM users WHERE sso_id = '51'`); n != 1 {
		t.Fatalf("vault generation: %d", n)
	}
	if n := count(`SELECT COUNT(*) FROM users WHERE sso_id = '51' AND vault_check = '' AND vault_wrap = ''`); n != 1 {
		t.Fatal("the phrase check and the sealed key must be cleared")
	}
	if code, _, _ := u.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("the user's session must end: %d", code)
	}
	if n := count(`SELECT COUNT(*) FROM hop_audit WHERE action = 'user.reset_vault'`); n != 1 {
		t.Fatalf("audit lines: %d", n)
	}
	// a password account has no vault phrase to reset here
	newClient(t, srv).do("POST", "/api/register", map[string]any{"username": "lokal", "auth": key()}, nil)
	if code, env := call(t, srv, "imperium", "POST", "/hop/users/local:2/actions", map[string]any{"action": "reset_vault"}); code != 400 {
		t.Fatalf("password account: %d %v", code, env)
	}
}
