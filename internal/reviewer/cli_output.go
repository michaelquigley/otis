package reviewer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/michaelquigley/otis/internal/prompt"
)

func ParseCLIOutput(output []byte, schema json.RawMessage) (json.RawMessage, prompt.ReviewerOutput, error) {
	var lastErr error
	for _, raw := range outputCandidates(output) {
		parsed, err := prompt.ParseReviewerOutput(raw, schema)
		if err == nil {
			return raw, parsed, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, prompt.ReviewerOutput{}, lastErr
	}
	return nil, prompt.ReviewerOutput{}, errors.New("reviewer output does not contain schema-valid json")
}

func DryRunResult(schema json.RawMessage, usageNotes string) (Result, error) {
	raw := json.RawMessage(`{"findings":[]}`)
	output, err := prompt.ParseReviewerOutput(raw, schema)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Raw:        raw,
		Output:     output,
		Findings:   []prompt.ReviewerFinding{},
		UsageNotes: usageNotes,
	}, nil
}

func CommandLine(binary string, args []string) string {
	parts := []string{shellQuote(binary)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func CommandOutputSuffix(stdout []byte, stderr []byte) string {
	var parts []string
	if text := strings.TrimSpace(string(stderr)); text != "" {
		parts = append(parts, fmt.Sprintf("stderr: %s", text))
	}
	if text := strings.TrimSpace(string(stdout)); text != "" {
		parts = append(parts, fmt.Sprintf("stdout: %s", text))
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, "; ")
}

func outputCandidates(output []byte) []json.RawMessage {
	seen := map[string]struct{}{}
	var candidates []json.RawMessage
	add := func(raw []byte) {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			return
		}
		key := string(raw)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, append(json.RawMessage(nil), raw...))
	}

	trimmed := bytes.TrimSpace(output)
	add(trimmed)
	if fenced, ok := stripMarkdownFence(trimmed); ok {
		add(fenced)
	}
	if raw, ok := firstJSONObject(trimmed); ok {
		add(raw)
	}
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		add(line)
		addEnvelopeCandidates(line, add)
	}
	addEnvelopeCandidates(trimmed, add)
	return candidates
}

func addEnvelopeCandidates(raw []byte, add func([]byte)) {
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return
	}
	for _, key := range []string{"structured_output", "output", "result", "text", "message"} {
		value, ok := envelope[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			add([]byte(v))
			if obj, ok := firstJSONObject([]byte(v)); ok {
				add(obj)
			}
		default:
			if encoded, err := json.Marshal(v); err == nil {
				add(encoded)
			}
		}
	}
}

func stripMarkdownFence(raw []byte) ([]byte, bool) {
	lines := bytes.Split(raw, []byte("\n"))
	if len(lines) < 3 {
		return nil, false
	}
	firstLine := strings.TrimSpace(string(lines[0]))
	backticks := leadingBackticks(firstLine)
	if backticks < 3 {
		return nil, false
	}
	lastLine := strings.TrimSpace(string(lines[len(lines)-1]))
	if lastLine != strings.Repeat("`", backticks) {
		return nil, false
	}
	return bytes.Join(lines[1:len(lines)-1], []byte("\n")), true
}

func leadingBackticks(s string) int {
	count := 0
	for _, r := range s {
		if r != '`' {
			break
		}
		count++
	}
	return count
}

func firstJSONObject(raw []byte) (json.RawMessage, bool) {
	for start, b := range raw {
		if b != '{' {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(raw[start:]))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			continue
		}
		if _, ok := value.(map[string]any); !ok {
			continue
		}
		end := start + int(decoder.InputOffset())
		return append(json.RawMessage(nil), bytes.TrimSpace(raw[start:end])...), true
	}
	return nil, false
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n'\"\\$`") {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return value
}
