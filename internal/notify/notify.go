package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/michaelquigley/otis/internal/state"
)

type Notifier interface {
	Post(ctx context.Context, notification Notification) error
}

type Notification struct {
	Project   string
	Pass      string
	RunID     string
	Date      string
	Channel   string
	ReportURL string
	Findings  []*state.Finding
}

func Render(notification Notification) string {
	var b strings.Builder
	fmt.Fprintf(&b, "otis: %s %s, %s\n", notification.Project, notification.Pass, notification.Date)
	fmt.Fprintf(&b, "%s\n\n", findingsSummary(notification.Findings))
	for i, finding := range notification.Findings {
		fmt.Fprintf(&b, "#%d  %s    [%s]  %s\n", i+1, finding.Title, finding.ID, finding.Severity)
	}
	if notification.ReportURL != "" {
		fmt.Fprintf(&b, "\nFull report: %s\n", notification.ReportURL)
	}
	if len(notification.Findings) > 0 {
		id := notification.Findings[0].ID
		fmt.Fprintf(&b, "Triage: otis accept %s / otis defer %s / otis reject %s\n", id, id, id)
	}
	return b.String()
}

func findingsSummary(findings []*state.Finding) string {
	counts := map[string]int{}
	for _, finding := range findings {
		counts[finding.Severity]++
	}
	parts := []string{}
	for _, severity := range []string{"high", "medium", "low"} {
		if counts[severity] == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", counts[severity], severity))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%d findings", len(findings))
	}
	return fmt.Sprintf("%d findings (%s)", len(findings), strings.Join(parts, ", "))
}
