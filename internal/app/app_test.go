package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUploadServeListDelete(t *testing.T) {
	s, err := New(Config{
		ListenAddr:    ":0",
		DataDir:       t.TempDir(),
		PublicBaseURL: "https://pages.example.test",
		UploadToken:   "test-token",
		MaxUpload:     1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "report.html")
	_, _ = part.Write([]byte("<!doctype html><title>Hello</title><h1>PageDrop works</h1>"))
	_ = writer.Close()

	req, _ := http.NewRequest(http.MethodPost, web.URL+"/api/v1/pages", &body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}
	var created page
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if created.ID == "" || created.URL != "https://pages.example.test/p/"+created.ID+"/" {
		t.Fatalf("unexpected upload response: %+v", created)
	}

	resp, err = http.Get(web.URL + "/p/" + created.ID + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Robots-Tag") == "" {
		t.Fatalf("serve response status=%d headers=%v", resp.StatusCode, resp.Header)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodDelete, web.URL+"/api/v1/pages/"+created.ID, nil)
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(web.URL + "/p/" + created.ID + "/")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted page status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestZIPAssetsReplacementCachingAndExpiry(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "https://pages.test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 2 << 20, MaxFiles: 10, DefaultExpiry: time.Hour, MaxExpiry: 24 * time.Hour, CleanupInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()

	archive := zipBytes(t, map[string]string{"index.html": "<link rel=stylesheet href=assets/site.css><h1>v1</h1>", "assets/site.css": "body{color:blue}"})
	created := uploadTestFile(t, web.URL, "site.zip", archive, "1h")
	resp, err := http.Get(web.URL + "/p/" + created.ID + "/assets/site.css")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "body{color:blue}" {
		t.Fatalf("asset=%q", body)
	}
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	req, _ := http.NewRequest(http.MethodGet, web.URL+"/p/"+created.ID+"/assets/site.css", nil)
	req.Header.Set("If-None-Match", etag)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional status=%d", resp.StatusCode)
	}
	resp.Body.Close()

	replacement := zipBytes(t, map[string]string{"index.html": "<h1>v2</h1>", "assets/site.css": "body{color:red}"})
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, _ := writer.CreateFormFile("file", "site.zip")
	_, _ = part.Write(replacement)
	_ = writer.Close()
	req, _ = http.NewRequest(http.MethodPut, web.URL+"/api/v1/pages/"+created.ID+"/content", &upload)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("replace=%d %s", resp.StatusCode, data)
	}
	resp.Body.Close()
	resp, _ = http.Get(web.URL + "/p/" + created.ID + "/assets/site.css")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "body{color:red}" || resp.Header.Get("ETag") == etag {
		t.Fatal("replacement not visible or ETag unchanged")
	}
}

func TestRejectsUnsafeZIP(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 1 << 20, MaxFiles: 10})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	archive := zipBytes(t, map[string]string{"index.html": "ok", "../escape.txt": "bad"})
	resp := rawUpload(t, web.URL, "unsafe.zip", archive, "")
	if resp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, data)
	}
	resp.Body.Close()
}

func TestExpiredPageReturnsGone(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://test", UploadToken: "test-token", MaxUpload: 1 << 20, MaxExtracted: 1 << 20, MaxFiles: 10, DefaultExpiry: time.Hour, MaxExpiry: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	web := httptest.NewServer(s.httpServer.Handler)
	defer web.Close()
	p := uploadTestFile(t, web.URL, "page.html", []byte("hello"), "1ms")
	time.Sleep(5 * time.Millisecond)
	resp, _ := http.Get(web.URL + "/p/" + p.ID + "/")
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for name, content := range files {
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func rawUpload(t *testing.T, server, name string, data []byte, expires string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", name)
	_, _ = part.Write(data)
	if expires != "" {
		_ = writer.WriteField("expires_in", expires)
	}
	_ = writer.Close()
	req, _ := http.NewRequest(http.MethodPost, server+"/api/v1/pages", &body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func uploadTestFile(t *testing.T, server, name string, data []byte, expires string) page {
	t.Helper()
	resp := rawUpload(t, server, name, data, expires)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload=%d %s", resp.StatusCode, body)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUploadRequiresAuthentication(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir(), PublicBaseURL: "http://example.test", UploadToken: "secret", MaxUpload: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pages", nil).WithContext(context.Background())
	recorder := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}
