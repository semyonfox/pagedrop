package app

import (
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) listPages(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 200 {
		limit = value
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT `+pageColumns+` FROM pages ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not list pages.")
		return
	}
	defer rows.Close()
	pages := make([]page, 0)
	for rows.Next() {
		p, err := s.scanPage(rows)
		if err != nil {
			writeError(w, 500, "INTERNAL", "Could not list pages.")
			return
		}
		pages = append(pages, p)
	}
	writeJSON(w, 200, map[string]any{"pages": pages})
}

func (s *Server) getPage(w http.ResponseWriter, r *http.Request) {
	p, err := s.getPageRecord(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "NOT_FOUND", "Page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not read page.")
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) deletePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	result, err := s.db.ExecContext(r.Context(), `UPDATE pages SET status='deleted',updated_at=? WHERE id=? AND status!='deleted'`, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not delete page.")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var exists int
		if s.db.QueryRow(`SELECT 1 FROM pages WHERE id=?`, id).Scan(&exists) != nil {
			writeError(w, 404, "NOT_FOUND", "Page not found.")
			return
		}
	}
	_ = removePageFiles(s.cfg.DataDir, id)
	w.WriteHeader(http.StatusNoContent)
}

func removePageFiles(dataDir, id string) error {
	if !validID(id) {
		return errors.New("invalid page id")
	}
	return os.RemoveAll(filepath.Join(dataDir, "pages", id))
}

func validID(id string) bool {
	if len(id) != 22 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func (s *Server) servePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validID(id) {
		http.NotFound(w, r)
		return
	}
	p, err := s.getPageRecord(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if p.Status == "deleted" {
		http.NotFound(w, r)
		return
	}
	if p.Status == "expired" || isExpired(p.ExpiresAt) {
		if p.Status == "active" {
			_, _ = s.db.Exec(`UPDATE pages SET status='expired' WHERE id=?`, id)
			_ = removePageFiles(s.cfg.DataDir, id)
		}
		writeError(w, http.StatusGone, "EXPIRED", "This page has expired.")
		return
	}
	requestPath := r.PathValue("path")
	if requestPath == "" {
		requestPath = "index.html"
	}
	clean := filepath.Clean(filepath.FromSlash(requestPath))
	if clean == "." {
		clean = "index.html"
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	root := filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", p.ContentVersion))
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir() {
		path = filepath.Join(path, "index.html")
		info, err = os.Stat(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	etag := fmt.Sprintf(`"v%d-%x-%x"`, p.ContentVersion, info.Size(), info.ModTime().UnixNano())
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeFile(w, r, path)
}

func isExpired(value *string) bool {
	if value == nil {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, *value)
	return err == nil && !time.Now().UTC().Before(expiry)
}
