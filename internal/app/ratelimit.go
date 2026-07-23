package app

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateWindow struct {
	start time.Time
	count int
}

type uploadRateLimiter struct {
	mu        sync.Mutex
	clients   map[string]rateWindow
	limit     int
	window    time.Duration
	lastSweep time.Time
}

func newUploadRateLimiter(limit int, window time.Duration) *uploadRateLimiter {
	return &uploadRateLimiter{
		clients: make(map[string]rateWindow),
		limit:   limit,
		window:  window,
	}
}

func (l *uploadRateLimiter) allow(client string, now time.Time) (remaining int, retryAfter time.Duration, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.window {
		for key, window := range l.clients {
			if now.Sub(window.start) >= l.window {
				delete(l.clients, key)
			}
		}
		l.lastSweep = now
	}

	current, exists := l.clients[client]
	if !exists || now.Sub(current.start) >= l.window {
		l.clients[client] = rateWindow{start: now, count: 1}
		return l.limit - 1, 0, true
	}
	if current.count >= l.limit {
		return 0, l.window - now.Sub(current.start), false
	}
	current.count++
	l.clients[client] = current
	return l.limit - current.count, 0, true
}

func (s *Server) limitUploads(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		remaining, retryAfter, allowed := s.uploadLimiter.allow(s.clientIP(r), time.Now())
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(s.cfg.UploadsPerMinute))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Upload rate limit exceeded; retry later.")
			return
		}
		next(w, r)
	}
}

func (s *Server) limitUploadConcurrency(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.uploadSlots <- struct{}{}:
			defer func() { <-s.uploadSlots }()
			next(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "UPLOAD_BUSY", "The server is processing other uploads; retry shortly.")
		}
	}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxyHeaders {
		if candidate := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
