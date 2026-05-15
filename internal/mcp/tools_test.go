package mcpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/otis/internal/client"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolsForwardToSupervisor(t *testing.T) {
	var dispositionBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/testproj/findings":
			if got := r.URL.Query().Get("pass"); got != "vocabulary-sweep" {
				t.Fatalf("pass query = %q", got)
			}
			if got := r.URL.Query().Get("disposition"); got != "open" {
				t.Fatalf("disposition query = %q", got)
			}
			if got := r.URL.Query().Get("open"); got != "true" {
				t.Fatalf("open query = %q", got)
			}
			writeMap(t, w, map[string]any{
				"findings": []any{
					map[string]any{"id": "testproj/vocabulary-sweep/0001", "title": "first"},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/testproj/findings/vocabulary-sweep/0001":
			writeMap(t, w, map[string]any{
				"id":          "testproj/vocabulary-sweep/0001",
				"title":       "first",
				"disposition": "open",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects/testproj/runs/vocabulary-sweep/2026-05-15/010203Z-001/report":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write([]byte("# Stored Report\n"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects/testproj/findings/vocabulary-sweep/0001/disposition":
			if err := json.NewDecoder(r.Body).Decode(&dispositionBody); err != nil {
				t.Fatalf("decode disposition body: %v", err)
			}
			writeMap(t, w, map[string]any{
				"id":          "testproj/vocabulary-sweep/0001",
				"disposition": "accepted",
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer api.Close()

	cfg := &client.Config{URL: api.URL, Token: "test-token"}
	supervisor, err := client.New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx, session := newTestClient(t, New(supervisor, "test"))

	list := callTool(t, ctx, session, "otis_list_findings", map[string]any{
		"project":     "testproj",
		"pass":        "vocabulary-sweep",
		"disposition": "open",
		"open":        true,
	})
	findings := structuredMap(t, list)["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(findings))
	}

	finding := callTool(t, ctx, session, "otis_get_finding", map[string]any{
		"id": "testproj/vocabulary-sweep/0001",
	})
	if got := structuredMap(t, finding)["title"]; got != "first" {
		t.Fatalf("title = %v", got)
	}

	report := callTool(t, ctx, session, "otis_get_report", map[string]any{
		"run_id": "testproj/vocabulary-sweep/2026-05-15/010203Z-001",
	})
	if got := report.Content[0].(*mcpsdk.TextContent).Text; !strings.Contains(got, "Stored Report") {
		t.Fatalf("report content = %q", got)
	}

	disposition := callTool(t, ctx, session, "otis_disposition_finding", map[string]any{
		"id":          "testproj/vocabulary-sweep/0001",
		"disposition": "accepted",
		"note":        "handled",
	})
	if got := structuredMap(t, disposition)["disposition"]; got != "accepted" {
		t.Fatalf("disposition = %v", got)
	}
	if got := dispositionBody["note"]; got != "handled" {
		t.Fatalf("note = %v", got)
	}
}

func TestToolInputErrorIsReturnedAsToolResult(t *testing.T) {
	ctx, session := newTestClient(t, New(noopSupervisor{}, "test"))
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "otis_get_finding",
		Arguments: map[string]any{"id": "bad"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error result")
	}
	if got := result.Content[0].(*mcpsdk.TextContent).Text; !strings.Contains(got, "finding id must have form") {
		t.Fatalf("error = %q", got)
	}
}

func newTestClient(t *testing.T, server *mcpsdk.Server) (context.Context, *mcpsdk.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Close()
	})
	return ctx, clientSession
}

func callTool(t *testing.T, ctx context.Context, session *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if result.IsError {
		t.Fatalf("call %s returned tool error: %s", name, result.Content[0].(*mcpsdk.TextContent).Text)
	}
	return result
}

func structuredMap(t *testing.T, result *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	payload, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured content = %T", result.StructuredContent)
	}
	return payload
}

func writeMap(t *testing.T, w http.ResponseWriter, value map[string]any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

type noopSupervisor struct{}

func (noopSupervisor) DoJSON(context.Context, string, string, any, any) error {
	return nil
}

func (noopSupervisor) DoText(context.Context, string, string, any) (string, error) {
	return "", nil
}
