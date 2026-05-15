package state

import (
	"fmt"
	"strings"
)

func (p *Project) renderBacklogLocked() error {
	findings, err := p.listFindingsLocked(FindingFilter{OpenOnly: true})
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# Otis Backlog\n\n")
	b.WriteString(fmt.Sprintf("Project: `%s`\n\n", p.name))
	if len(findings) == 0 {
		b.WriteString("_no open findings_\n")
		return atomicWriteFile(BacklogPath(p.root, p.name), []byte(b.String()), 0o600)
	}

	for _, finding := range findings {
		b.WriteString(fmt.Sprintf("## %s\n\n", finding.ID))
		b.WriteString(fmt.Sprintf("- Severity: `%s`\n", finding.Severity))
		b.WriteString(fmt.Sprintf("- Pass: `%s`\n", finding.Pass))
		b.WriteString(fmt.Sprintf("- Location: `%s:%s`\n", finding.Location.File, finding.Location.Lines))
		b.WriteString(fmt.Sprintf("- Title: %s\n\n", finding.Title))
		b.WriteString(strings.TrimSpace(finding.Description))
		b.WriteString("\n\n")
		if strings.TrimSpace(finding.SuggestedFix) != "" {
			b.WriteString("Suggested fix: ")
			b.WriteString(strings.TrimSpace(finding.SuggestedFix))
			b.WriteString("\n\n")
		}
	}
	return atomicWriteFile(BacklogPath(p.root, p.name), []byte(b.String()), 0o600)
}
