package mattermost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/michaelquigley/otis/internal/notify"
	"github.com/michaelquigley/otis/internal/state"
)

func TestPostWebhook(t *testing.T) {
	var gotPath string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	t.Setenv("MATTERMOST_TOKEN", "secret-token")
	notifier := New(Options{URL: server.URL + "/hooks", TokenEnv: "MATTERMOST_TOKEN"})
	err := notifier.Post(context.Background(), notify.Notification{
		Project: "testproj",
		Pass:    "vocabulary-sweep",
		Date:    "2026-05-15",
		Channel: "#otis-testproj",
		Findings: []*state.Finding{
			{ID: "testproj/vocabulary-sweep/0001", Title: "first", Severity: "high"},
		},
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if gotPath != "/hooks/secret-token" {
		t.Fatalf("path = %q", gotPath)
	}
	if payload["channel"] != "#otis-testproj" {
		t.Fatalf("channel = %v", payload["channel"])
	}
	if payload["text"] == "" {
		t.Fatalf("missing text payload: %+v", payload)
	}
}

func TestPostSkipsEmptyFindingSet(t *testing.T) {
	notifier := New(Options{URL: "http://127.0.0.1:1/hooks", Token: "secret"})
	if err := notifier.Post(context.Background(), notify.Notification{}); err != nil {
		t.Fatalf("empty notification should skip: %v", err)
	}
}
