package notify

import (
	"strings"
	"testing"

	"github.com/michaelquigley/otis/internal/state"
)

func TestRenderNotification(t *testing.T) {
	text := Render(Notification{
		Project:   "testproj",
		Pass:      "vocabulary-sweep",
		Date:      "2026-05-15",
		ReportURL: "https://otis.example.com/report",
		Findings: []*state.Finding{
			{ID: "testproj/vocabulary-sweep/0001", Title: "first", Severity: "high"},
			{ID: "testproj/vocabulary-sweep/0002", Title: "second", Severity: "medium"},
		},
	})
	for _, want := range []string{
		"otis: testproj vocabulary-sweep, 2026-05-15",
		"2 findings (1 high, 1 medium)",
		"#1  first    [testproj/vocabulary-sweep/0001]  high",
		"Full report: https://otis.example.com/report",
		"Triage: otis accept testproj/vocabulary-sweep/0001",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("notification missing %q:\n%s", want, text)
		}
	}
}
