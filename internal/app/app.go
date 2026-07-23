package app

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var version = "dev"

type Config struct {
	ListenAddr      string
	DataDir         string
	PublicBaseURL   string
	UploadToken     string
	MaxUpload       int64
	MaxExtracted    int64
	MaxFiles        int
	DefaultExpiry   time.Duration
	MaxExpiry       time.Duration
	CleanupInterval time.Duration
}

func Version() string { return version }

func ConfigFromEnv() (Config, error) {
	c := Config{
		ListenAddr:      env("PAGEDROP_LISTEN_ADDR", ":8080"),
		DataDir:         env("PAGEDROP_DATA_DIR", "./data"),
		PublicBaseURL:   strings.TrimRight(env("PAGEDROP_PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		UploadToken:     os.Getenv("PAGEDROP_TOKEN"),
		MaxUpload:       envInt64("PAGEDROP_MAX_UPLOAD_BYTES", 10<<20),
		MaxExtracted:    envInt64("PAGEDROP_MAX_EXTRACTED_BYTES", 50<<20),
		MaxFiles:        int(envInt64("PAGEDROP_MAX_FILES", 500)),
		CleanupInterval: time.Minute,
	}
	var err error
	if c.DefaultExpiry, err = parseExpiry(env("PAGEDROP_DEFAULT_EXPIRY", "1d")); err != nil {
		return Config{}, fmt.Errorf("PAGEDROP_DEFAULT_EXPIRY: %w", err)
	}
	if c.MaxExpiry, err = parseExpiry(env("PAGEDROP_MAX_EXPIRY", "7d")); err != nil {
		return Config{}, fmt.Errorf("PAGEDROP_MAX_EXPIRY: %w", err)
	}
	if c.UploadToken == "" {
		return Config{}, errors.New("PAGEDROP_TOKEN is required")
	}
	if len(c.UploadToken) < 32 {
		return Config{}, errors.New("PAGEDROP_TOKEN must be at least 32 characters")
	}
	if c.MaxUpload <= 0 || c.MaxExtracted <= 0 || c.MaxFiles <= 0 {
		return Config{}, errors.New("upload limits must be positive")
	}
	return c, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string, fallback int64) int64 {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

type Server struct {
	cfg        Config
	db         *sql.DB
	httpServer *http.Server
}

func New(cfg Config) (*Server, error) {
	if cfg.MaxUpload == 0 {
		cfg.MaxUpload = 10 << 20
	}
	if cfg.MaxExtracted == 0 {
		cfg.MaxExtracted = 50 << 20
	}
	if cfg.MaxFiles == 0 {
		cfg.MaxFiles = 500
	}
	if cfg.DefaultExpiry == 0 {
		cfg.DefaultExpiry = 24 * time.Hour
	}
	if cfg.MaxExpiry == 0 {
		cfg.MaxExpiry = 7 * 24 * time.Hour
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = time.Minute
	}
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "pages"), 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := openDatabase(filepath.Join(cfg.DataDir, "pagedrop.db"))
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.landingPage)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/v1/pages", s.createPage)
	mux.HandleFunc("GET /api/v1/pages", s.auth(s.listPages))
	mux.HandleFunc("GET /api/v1/pages/{id}", s.auth(s.getPage))
	mux.HandleFunc("PUT /api/v1/pages/{id}/content", s.auth(s.replacePage))
	mux.HandleFunc("DELETE /api/v1/pages/{id}", s.auth(s.deletePage))
	mux.HandleFunc("GET /p/{id}/{path...}", s.servePage)
	s.httpServer = &http.Server{
		Addr: cfg.ListenAddr, Handler: securityHeaders(logging(mux)),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	return s, nil
}

func (s *Server) Close() error { return s.db.Close() }

func (s *Server) Serve(ctx context.Context) error {
	go s.cleanupLoop(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutdownCtx)
	}()
	err := s.httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "A valid bearer token is required.")
			return
		}
		got := strings.TrimPrefix(auth, "Bearer ")
		if len(got) != len(s.cfg.UploadToken) || subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.UploadToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "A valid bearer token is required.")
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "database": "error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok", "version": version})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
