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
	writeJSON(w, http.StatusOK, map[string]any{"available": code == "", "code": code,
		"note_limit_bytes": 5 << 20, "total_limit_bytes": int64(a.cfg.FilesTotalMB) << 20})
}
