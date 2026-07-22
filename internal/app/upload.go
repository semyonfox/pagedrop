package app

import (
	"archive/zip"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type preparedUpload struct {
	dir   string
	size  int64
	files int
}

func (s *Server) createPage(w http.ResponseWriter, r *http.Request) {
	upload, title, expiresAt, err := s.receiveUpload(w, r)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	defer os.RemoveAll(upload.dir)
	id, err := randomID()
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not generate page ID.")
		return
	}
	root := filepath.Join(s.cfg.DataDir, "pages", id)
	if err := os.Mkdir(root, 0o750); err != nil {
		writeError(w, 500, "INTERNAL", "Could not prepare page.")
		return
	}
	final := filepath.Join(root, "v1")
	if err := os.Rename(upload.dir, final); err != nil {
		_ = os.RemoveAll(root)
		writeError(w, 500, "INTERNAL", "Could not activate page.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var expiry any
	if expiresAt != nil {
		expiry = expiresAt.UTC().Format(time.RFC3339)
	}
	_, err = s.db.Exec(`INSERT INTO pages(id,title,status,created_at,updated_at,expires_at,size_bytes,file_count,content_version) VALUES(?,?,'active',?,?,?,?,?,1)`, id, title, now, now, expiry, upload.size, upload.files)
	if err != nil {
		_ = os.RemoveAll(root)
		writeError(w, 500, "INTERNAL", "Could not record page.")
		return
	}
	p, _ := s.getPageRecord(id)
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) replacePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	current, err := s.getPageRecord(id)
	if errors.Is(err, sql.ErrNoRows) || current.Status != "active" {
		writeError(w, 404, "NOT_FOUND", "Active page not found.")
		return
	}
	if err != nil {
		writeError(w, 500, "INTERNAL", "Could not read page.")
		return
	}
	upload, _, _, err := s.receiveUpload(w, r)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	defer os.RemoveAll(upload.dir)
	version := current.ContentVersion + 1
	final := filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", version))
	if err := os.Rename(upload.dir, final); err != nil {
		writeError(w, 500, "INTERNAL", "Could not activate replacement.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`UPDATE pages SET updated_at=?,size_bytes=?,file_count=?,content_version=? WHERE id=? AND content_version=? AND status='active'`, now, upload.size, upload.files, version, id, current.ContentVersion)
	if err != nil {
		_ = os.RemoveAll(final)
		writeError(w, 500, "INTERNAL", "Could not record replacement.")
		return
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		_ = os.RemoveAll(final)
		writeError(w, 409, "CONFLICT", "Page changed during replacement; retry.")
		return
	}
	_ = os.RemoveAll(filepath.Join(s.cfg.DataDir, "pages", id, fmt.Sprintf("v%d", current.ContentVersion)))
	p, _ := s.getPageRecord(id)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) receiveUpload(w http.ResponseWriter, r *http.Request) (preparedUpload, string, *time.Time, error) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload+(2<<20))
	if err := r.ParseMultipartForm(s.cfg.MaxUpload); err != nil {
		return preparedUpload{}, "", nil, uploadError{413, "UPLOAD_TOO_LARGE", "Upload exceeds the configured limit."}
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		return preparedUpload{}, "", nil, uploadError{400, "FILE_REQUIRED", "An HTML or ZIP file is required."}
	}
	defer file.Close()
	expiresAt, err := s.expiryFromForm(r.FormValue("expires_in"))
	if err != nil {
		return preparedUpload{}, "", nil, uploadError{400, "INVALID_EXPIRY", err.Error()}
	}
	tmp, err := os.MkdirTemp(filepath.Join(s.cfg.DataDir, "pages"), ".upload-")
	if err != nil {
		return preparedUpload{}, "", nil, err
	}
	upload := preparedUpload{dir: tmp}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	switch ext {
	case ".html", ".htm":
		size, err := copyLimited(filepath.Join(tmp, "index.html"), file, s.cfg.MaxUpload)
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, "", nil, err
		}
		upload.size, upload.files = size, 1
	case ".zip":
		archivePath := filepath.Join(tmp, ".upload.zip")
		size, err := copyLimited(archivePath, file, s.cfg.MaxUpload)
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, "", nil, err
		}
		archive, err := zip.OpenReader(archivePath)
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, "", nil, uploadError{400, "INVALID_ARCHIVE", "ZIP archive is invalid."}
		}
		upload.size, upload.files, err = s.extractZIP(archive, tmp)
		archive.Close()
		_ = os.Remove(archivePath)
		_ = size
		if err != nil {
			os.RemoveAll(tmp)
			return preparedUpload{}, "", nil, err
		}
	default:
		os.RemoveAll(tmp)
		return preparedUpload{}, "", nil, uploadError{400, "UNSUPPORTED_FILE", "Upload a standalone HTML file or ZIP archive."}
	}
	if _, err := os.Stat(filepath.Join(tmp, "index.html")); err != nil {
		os.RemoveAll(tmp)
		return preparedUpload{}, "", nil, uploadError{400, "INDEX_REQUIRED", "Archive must contain index.html at its root."}
	}
	return upload, strings.TrimSpace(r.FormValue("title")), expiresAt, nil
}

func copyLimited(path string, source io.Reader, limit int64) (int64, error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	if err != nil {
		return 0, err
	}
	size, copyErr := io.Copy(out, io.LimitReader(source, limit+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		return size, errors.Join(copyErr, closeErr)
	}
	if size > limit {
		return size, uploadError{413, "UPLOAD_TOO_LARGE", "Upload exceeds the configured limit."}
	}
	return size, nil
}

func (s *Server) extractZIP(archive *zip.ReadCloser, destination string) (int64, int, error) {
	var total int64
	count := 0
	for _, entry := range archive.File {
		name := filepath.ToSlash(entry.Name)
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || filepath.IsAbs(name) {
			return 0, 0, uploadError{400, "UNSAFE_ARCHIVE", "Archive contains an unsafe path."}
		}
		clean := filepath.Clean(name)
		if clean == "." {
			continue
		}
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return 0, 0, uploadError{400, "UNSAFE_ARCHIVE", "Archive contains a traversal path."}
		}
		if entry.Mode()&os.ModeSymlink != 0 || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
			return 0, 0, uploadError{400, "UNSAFE_ARCHIVE", "Archive contains a link or special file."}
		}
		target := filepath.Join(destination, clean)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return 0, 0, err
			}
			continue
		}
		count++
		if count > s.cfg.MaxFiles {
			return 0, 0, uploadError{413, "TOO_MANY_FILES", "Archive contains too many files."}
		}
		if entry.UncompressedSize64 > uint64(s.cfg.MaxExtracted) || total+int64(entry.UncompressedSize64) > s.cfg.MaxExtracted {
			return 0, 0, uploadError{413, "EXTRACTED_TOO_LARGE", "Extracted content exceeds the configured limit."}
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return 0, 0, err
		}
		source, err := entry.Open()
		if err != nil {
			return 0, 0, err
		}
		size, err := copyLimited(target, source, s.cfg.MaxExtracted-total)
		source.Close()
		if err != nil {
			return 0, 0, err
		}
		total += size
	}
	return total, count, nil
}

func (s *Server) expiryFromForm(value string) (*time.Time, error) {
	if value == "" {
		value = formatExpiry(s.cfg.DefaultExpiry)
	}
	if strings.EqualFold(value, "never") {
		return nil, nil
	}
	duration, err := parseExpiry(value)
	if err != nil || duration <= 0 {
		return nil, fmt.Errorf("use an expiry such as 1h, 7d, 30d, or never")
	}
	if s.cfg.MaxExpiry > 0 && duration > s.cfg.MaxExpiry {
		return nil, fmt.Errorf("expiry exceeds maximum of %s", formatExpiry(s.cfg.MaxExpiry))
	}
	t := time.Now().UTC().Add(duration)
	return &t, nil
}

func parseExpiry(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "never" {
		return 0, nil
	}
	if strings.HasSuffix(value, "d") {
		days, err := time.ParseDuration(strings.TrimSuffix(value, "d") + "h")
		if err != nil {
			return 0, err
		}
		return days * 24, nil
	}
	return time.ParseDuration(value)
}

func formatExpiry(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
	return d.String()
}

type uploadError struct {
	status        int
	code, message string
}

func (e uploadError) Error() string { return e.message }
func writeUploadError(w http.ResponseWriter, err error) {
	var e uploadError
	if errors.As(err, &e) {
		writeError(w, e.status, e.code, e.message)
	} else {
		writeError(w, 500, "INTERNAL", "Could not process upload.")
	}
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
