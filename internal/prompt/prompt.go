package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/michaelquigley/otis/internal/bok"
	"github.com/michaelquigley/otis/internal/state"
)

type ProjectContext struct {
	Name            string
	Description     string
	PrimaryLanguage string
}

type PassContext struct {
	Name        string
	Description string
}

type PriorFinding struct {
	ID          string
	Title       string
	Description string
	Location    state.Location
	Disposition string
	Notes       []state.NoteHistoryEntry
}

type Request struct {
	Project       ProjectContext
	Pass          PassContext
	TopFindings   int
	BokEntries    []*bok.Entry
	Scope         ScopeContent
	PriorFindings []PriorFinding
}

type BuildResult struct {
	Prompt string
	Schema json.RawMessage
}

// Build assembles the standard Otis reviewer prompt and schema.
func Build(req Request) BuildResult {
	schema := ReviewerOutputSchema(req.TopFindings)
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Otis reviewer: %s/%s\n\n", req.Project.Name, req.Pass.Name))
	b.WriteString("## Role and goal\n\n")
	b.WriteString(fmt.Sprintf("You are the Otis reviewer for pass `%s` on project `%s`.\n\n", req.Pass.Name, req.Project.Name))
	b.WriteString("Your goal is to surface the strongest findings that would reduce cognitive load in this codebase. Prefer fewer, more material findings over a broad checklist.\n\n")
	if req.TopFindings > 0 {
		b.WriteString(fmt.Sprintf("Return the best %d findings at most. Returning zero findings is valid when nothing strong exists.\n\n", req.TopFindings))
	}

	b.WriteString("## BoK slice\n\n")
	if len(req.BokEntries) == 0 {
		b.WriteString("(no BoK entries resolved)\n\n")
	} else {
		for _, entry := range req.BokEntries {
			writeBokEntry(&b, entry)
		}
	}

	b.WriteString("## Project context\n\n")
	writeProjectContext(&b, req.Project, req.Pass)

	b.WriteString("## Scope content\n\n")
	writeScopeContent(&b, req.Scope)

	b.WriteString("## Prior findings context\n\n")
	writePriorFindings(&b, req.PriorFindings)

	b.WriteString("## Output schema and budget\n\n")
	if req.TopFindings > 0 {
		b.WriteString(fmt.Sprintf("Return at most %d entries in `findings`, ranked by impact on cognitive load.\n\n", req.TopFindings))
	}
	b.WriteString("Respond with a single JSON object only. Do not include markdown fences or commentary outside the object. The response must conform to this schema:\n\n")
	b.WriteString("```json\n")
	b.WriteString(prettyJSON(schema))
	b.WriteString("\n```\n")

	return BuildResult{
		Prompt: b.String(),
		Schema: schema,
	}
}

// PriorFindingsFromState converts state finding contexts to prompt prior findings.
func PriorFindingsFromState(contexts []state.FindingContext) []PriorFinding {
	out := make([]PriorFinding, 0, len(contexts))
	for _, ctx := range contexts {
		if ctx.Finding == nil {
			continue
		}
		out = append(out, PriorFinding{
			ID:          ctx.Finding.ID,
			Title:       ctx.Finding.Title,
			Description: ctx.Finding.Description,
			Location:    ctx.Finding.Location,
			Disposition: ctx.Finding.Disposition,
			Notes:       append([]state.NoteHistoryEntry(nil), ctx.Notes...),
		})
	}
	return out
}

func writeBokEntry(b *strings.Builder, entry *bok.Entry) {
	b.WriteString(fmt.Sprintf("### %s\n\n", entry.Relpath))
	if entry.Title != "" {
		b.WriteString(fmt.Sprintf("Title: %s\n", entry.Title))
	}
	if len(entry.Tags) > 0 {
		b.WriteString(fmt.Sprintf("Tags: %s\n", strings.Join(entry.Tags, ", ")))
	}
	b.WriteString("\n")
	b.WriteString(fenceFor(entry.Body))
	b.WriteString("\n")
	b.WriteString(strings.TrimRight(entry.Body, "\n"))
	b.WriteString("\n")
	b.WriteString(fenceFor(entry.Body))
	b.WriteString("\n\n")
}

func writeProjectContext(b *strings.Builder, project ProjectContext, pass PassContext) {
	b.WriteString(fmt.Sprintf("Project: `%s`\n", project.Name))
	if strings.TrimSpace(project.Description) != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", strings.TrimSpace(project.Description)))
	}
	if strings.TrimSpace(project.PrimaryLanguage) != "" {
		b.WriteString(fmt.Sprintf("Primary language: %s\n", strings.TrimSpace(project.PrimaryLanguage)))
	}
	b.WriteString(fmt.Sprintf("Pass: `%s`\n", pass.Name))
	if strings.TrimSpace(pass.Description) != "" {
		b.WriteString(fmt.Sprintf("Pass description: %s\n", strings.TrimSpace(pass.Description)))
	}
	b.WriteString("\n")
}

func writeScopeContent(b *strings.Builder, scope ScopeContent) {
	b.WriteString(fmt.Sprintf("Scope kind: `%s`\n", scope.Kind))
	b.WriteString(fmt.Sprintf("Git HEAD: `%s`\n\n", scope.GitHead))
	b.WriteString("### File manifest\n\n")
	if len(scope.Files) == 0 {
		b.WriteString("(empty scope)\n\n")
	} else {
		for _, file := range scope.Files {
			b.WriteString(fmt.Sprintf("- `%s`", file.Path))
			if file.Size > 0 {
				b.WriteString(fmt.Sprintf(" (%d bytes)", file.Size))
			}
			if file.Truncated != "" {
				b.WriteString(fmt.Sprintf(" truncated: %s", file.Truncated))
			}
			if file.Inline {
				b.WriteString(" inline")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("### Inline content\n\n")
	if len(scope.Inline) == 0 {
		b.WriteString("(no inline content)\n\n")
		return
	}
	for _, inline := range scope.Inline {
		label := "content"
		if inline.Diff {
			label = "diff"
		}
		b.WriteString(fmt.Sprintf("#### %s %s\n\n", label, inline.Path))
		fence := fenceFor(inline.Content)
		b.WriteString(fence)
		b.WriteString("\n")
		b.WriteString(strings.TrimRight(inline.Content, "\n"))
		b.WriteString("\n")
		b.WriteString(fence)
		b.WriteString("\n\n")
	}
}

func writePriorFindings(b *strings.Builder, findings []PriorFinding) {
	if len(findings) == 0 {
		b.WriteString("(no prior findings)\n\n")
		return
	}
	b.WriteString("Use prior findings to preserve cross-run identity. For `open`, reference the existing id when surfacing the same issue. For `accepted`, do not re-surface unless the code shows the fix did not land. For `deferred` or `rejected`, treat the issue as known and do not re-surface it.\n\n")
	for _, finding := range findings {
		b.WriteString(fmt.Sprintf("### %s\n\n", finding.ID))
		b.WriteString(fmt.Sprintf("Disposition: `%s`\n", finding.Disposition))
		b.WriteString(fmt.Sprintf("Title: %s\n", finding.Title))
		b.WriteString(fmt.Sprintf("Location: `%s:%s`\n\n", finding.Location.File, finding.Location.Lines))
		b.WriteString(strings.TrimSpace(finding.Description))
		b.WriteString("\n\n")
		if len(finding.Notes) > 0 {
			b.WriteString("Note history:\n")
			for _, note := range finding.Notes {
				b.WriteString(fmt.Sprintf("- `%s`: %s\n", note.Disposition, strings.TrimSpace(note.Note)))
			}
			b.WriteString("\n")
		}
	}
}

func fenceFor(content string) string {
	longest := 0
	current := 0
	for _, r := range content {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	if longest < 3 {
		longest = 3
	}
	return strings.Repeat("`", longest+1)
}

func prettyJSON(raw json.RawMessage) string {
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return string(raw)
	}
	return out.String()
}
