package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type testResponse struct {
	Name string
}

type testRequest struct {
	Name string
}

func TestLoadConfigAndDoJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
url: http://127.0.0.1:1
token: test-token
tls:
  ca_cert: ca.pem
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.TLS.CACert != filepath.Join(dir, "ca.pem") {
		t.Fatalf("ca cert = %q", cfg.TLS.CACert)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"ok"}`))
	}))
	defer server.Close()

	cfg.URL = server.URL
	cfg.TLS.CACert = ""
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var out testResponse
	if err := client.DoJSON(context.Background(), http.MethodPost, "/api/v1/test", testRequest{Name: "request"}, &out); err != nil {
		t.Fatalf("do json: %v", err)
	}
	if out.Name != "ok" {
		t.Fatalf("response = %+v", out)
	}
}
