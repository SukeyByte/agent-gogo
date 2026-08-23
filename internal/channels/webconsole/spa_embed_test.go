package webconsole

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

type mapStore struct {
	Store
}

type nopSender struct{}

func (nopSender) HandleChannelEvent(ctx context.Context, event InboundEvent) error { return nil }

func (nopSender) HandleUserConfirmation(ctx context.Context, confirmation InboundConfirmation) error {
	return nil
}

func TestServeSPAEmbeddedFallback(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":            {Data: []byte("<html>app</html>")},
		"assets/app-abc123.js":  {Data: []byte("console.log(1)")},
		"assets/app-abc123.css": {Data: []byte("body{}")},
	}
	server := NewAPIServer(mapStore{}, nopSender{}, NewSSEHub(16), ConfigView{}, "web", "s1", "")
	server.UseEmbeddedDist(dist)

	cases := []struct {
		path        string
		wantStatus  int
		wantBody    string
		wantContent string
	}{
		{path: "/", wantStatus: 200, wantBody: "<html>app</html>", wantContent: "text/html"},
		// ServeFile redirects /index.html to ./ by design.
		{path: "/index.html", wantStatus: 301},
		{path: "/assets/app-abc123.js", wantStatus: 200, wantBody: "console.log(1)", wantContent: "javascript"},
		{path: "/assets/app-abc123.css", wantStatus: 200, wantBody: "body{}", wantContent: "text/css"},
		{path: "/assets/missing.js", wantStatus: 200, wantBody: "<html>app</html>", wantContent: "text/html"},
		{path: "/projects/some-id", wantStatus: 200, wantBody: "<html>app</html>", wantContent: "text/html"},
		// net/http rejects encoded dot-dot segments before the handler sees them.
		{path: "/%2e%2e/secret", wantStatus: 400},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != tc.wantStatus {
			t.Errorf("GET %s: status = %d, want %d", tc.path, rec.Code, tc.wantStatus)
		}
		if !strings.Contains(rec.Body.String(), tc.wantBody) {
			t.Errorf("GET %s: body = %q, want it to contain %q", tc.path, rec.Body.String(), tc.wantBody)
		}
		if tc.wantContent != "" {
			got := rec.Header().Get("Content-Type")
			if !strings.Contains(got, tc.wantContent) {
				t.Errorf("GET %s: content-type = %q, want %q", tc.path, got, tc.wantContent)
			}
		}
	}
	if !server.HasDistAssets() {
		t.Error("HasDistAssets should be true when embedded dist has index.html")
	}
}

func TestServeSPADiskDistTakesPrecedence(t *testing.T) {
	embedded := fstest.MapFS{"index.html": {Data: []byte("<html>embedded</html>")}}
	distDir := t.TempDir()
	if err := osWriteFile(distDir+"/index.html", "<html>disk</html>"); err != nil {
		t.Fatalf("write disk dist: %v", err)
	}
	server := NewAPIServer(mapStore{}, nopSender{}, NewSSEHub(16), ConfigView{}, "web", "s1", distDir)
	server.UseEmbeddedDist(embedded)

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(rec.Body.String(), "<html>disk</html>") {
		t.Errorf("expected disk dist to win, got %q", rec.Body.String())
	}
}

func TestServeSPAWithoutAnyDist(t *testing.T) {
	server := NewAPIServer(mapStore{}, nopSender{}, NewSSEHub(16), ConfigView{}, "web", "s1", "")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /: status = %d, want 404", rec.Code)
	}
	if server.HasDistAssets() {
		t.Error("HasDistAssets should be false with no disk or embedded dist")
	}
}

func TestHasDotDotElement(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: "assets/app.js", want: false},
		{path: "..", want: true},
		{path: "../secret", want: true},
		{path: "a/../../secret", want: true},
		{path: "a..b.js", want: false},
		{path: "", want: false},
	} {
		if got := hasDotDotElement(tc.path); got != tc.want {
			t.Errorf("hasDotDotElement(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
