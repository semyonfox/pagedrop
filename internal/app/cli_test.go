package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfigurePreservesStoredToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SEOL_SERVER", "")
	t.Setenv("SEOL_TOKEN", "")

	if err := ConfigureCLI([]string{"--server", "https://old.example", "--token", "stored-admin-token"}); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCLI([]string{"--server", "https://new.example"}); err != nil {
		t.Fatal(err)
	}
	cfg := loadClientConfig()
	if cfg.Server != "https://new.example" || cfg.Token != "stored-admin-token" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestClientRequestHasTimeout(t *testing.T) {
	originalClient := managementHTTPClient
	managementHTTPClient = &http.Client{Timeout: 20 * time.Millisecond}
	t.Cleanup(func() { managementHTTPClient = originalClient })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	_, err := clientRequest(http.MethodGet, server.URL, "test-token")
	if err == nil || !strings.Contains(err.Error(), "Client.Timeout") {
		t.Fatalf("error = %v", err)
	}
}

func closeTestServer(t *testing.T, server *Server) {
	t.Helper()
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close server: %v", err)
		}
	})
}
