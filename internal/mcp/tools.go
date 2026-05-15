package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/michaelquigley/otis/internal/state"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Supervisor interface {
	DoJSON(ctx context.Context, method string, path string, body any, out any) error
	DoText(ctx context.Context, method string, path string, body any) (string, error)
}

func RegisterTools(server *mcpsdk.Server, supervisor Supervisor) {
	server.AddTool(&mcpsdk.Tool{
		Name:        "otis_list_findings",
		Description: "List Otis findings for a project, optionally narrowed by pass, disposition, or open state.",
		InputSchema: objectSchema([]string{"project"}, map[string]any{
			"project":     stringSchema("project name"),
			"pass":        stringSchema("optional pass name"),
			"disposition": dispositionSchema(),
			"open":        map[string]any{"type": "boolean", "description": "only return open findings"},
		}),
	}, listFindings(supervisor))

	server.AddTool(&mcpsdk.Tool{
		Name:        "otis_get_finding",
		Description: "Get one Otis finding by canonical finding id.",
		InputSchema: objectSchema([]string{"id"}, map[string]any{
			"id": stringSchema("canonical finding id: <project>/<pass>/<NNNN>"),
		}),
	}, getFinding(supervisor))

	server.AddTool(&mcpsdk.Tool{
		Name:        "otis_get_report",
		Description: "Get the markdown report for a completed Otis run by canonical run id.",
		InputSchema: objectSchema([]string{"run_id"}, map[string]any{
			"run_id": stringSchema("canonical run id: <project>/<pass>/<YYYY-MM-DD>/<HHMMSSZ-NNN>"),
		}),
	}, getReport(supervisor))

	server.AddTool(&mcpsdk.Tool{
		Name:        "otis_disposition_finding",
		Description: "Change the disposition for one Otis finding through the supervisor API.",
		InputSchema: objectSchema([]string{"id", "disposition"}, map[string]any{
			"id":          stringSchema("canonical finding id: <project>/<pass>/<NNNN>"),
			"disposition": dispositionSchema(),
			"note":        stringSchema("optional human note explaining the disposition"),
		}),
	}, dispositionFinding(supervisor))
}

func listFindings(supervisor Supervisor) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		args, err := arguments(req)
		if err != nil {
			return toolError(err), nil
		}
		project, err := requiredString(args, "project")
		if err != nil {
			return toolError(err), nil
		}
		query := url.Values{}
		if pass, ok, err := optionalString(args, "pass"); err != nil {
			return toolError(err), nil
		} else if ok {
			query.Set("pass", pass)
		}
		if disposition, ok, err := optionalString(args, "disposition"); err != nil {
			return toolError(err), nil
		} else if ok {
			query.Set("disposition", disposition)
		}
		if open, ok, err := optionalBool(args, "open"); err != nil {
			return toolError(err), nil
		} else if ok {
			query.Set("open", fmt.Sprintf("%t", open))
		}

		path := fmt.Sprintf("/api/v1/projects/%s/findings", url.PathEscape(project))
		if encoded := query.Encode(); encoded != "" {
			path += "?" + encoded
		}
		var out map[string]any
		if err := supervisor.DoJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
			return toolError(err), nil
		}
		return structuredResult(out), nil
	}
}

func getFinding(supervisor Supervisor) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		args, err := arguments(req)
		if err != nil {
			return toolError(err), nil
		}
		findingID, err := findingID(args)
		if err != nil {
			return toolError(err), nil
		}
		path := fmt.Sprintf("/api/v1/projects/%s/findings/%s/%04d",
			url.PathEscape(findingID.Project),
			url.PathEscape(findingID.Pass),
			findingID.Sequence)
		var out map[string]any
		if err := supervisor.DoJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
			return toolError(err), nil
		}
		return structuredResult(out), nil
	}
}

func getReport(supervisor Supervisor) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		args, err := arguments(req)
		if err != nil {
			return toolError(err), nil
		}
		runIDValue, err := requiredString(args, "run_id")
		if err != nil {
			return toolError(err), nil
		}
		runID, err := state.ParseRunID(runIDValue)
		if err != nil {
			return toolError(err), nil
		}
		path := fmt.Sprintf("/api/v1/projects/%s/runs/%s/%s/%s/report",
			url.PathEscape(runID.Project),
			url.PathEscape(runID.Pass),
			url.PathEscape(runID.Date),
			url.PathEscape(runID.TimeSeq))
		report, err := supervisor.DoText(ctx, http.MethodGet, path, nil)
		if err != nil {
			return toolError(err), nil
		}
		return &mcpsdk.CallToolResult{
			Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: report}},
			StructuredContent: map[string]any{"report": report, "run_id": runID.String()},
		}, nil
	}
}

func dispositionFinding(supervisor Supervisor) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		args, err := arguments(req)
		if err != nil {
			return toolError(err), nil
		}
		findingID, err := findingID(args)
		if err != nil {
			return toolError(err), nil
		}
		disposition, err := requiredString(args, "disposition")
		if err != nil {
			return toolError(err), nil
		}
		body := map[string]any{"disposition": disposition}
		if note, ok, err := optionalString(args, "note"); err != nil {
			return toolError(err), nil
		} else if ok {
			body["note"] = note
		}
		path := fmt.Sprintf("/api/v1/projects/%s/findings/%s/%04d/disposition",
			url.PathEscape(findingID.Project),
			url.PathEscape(findingID.Pass),
			findingID.Sequence)
		var out map[string]any
		if err := supervisor.DoJSON(ctx, http.MethodPost, path, body, &out); err != nil {
			return toolError(err), nil
		}
		return structuredResult(out), nil
	}
}

func arguments(req *mcpsdk.CallToolRequest) (map[string]any, error) {
	args := map[string]any{}
	if req == nil || req.Params == nil || len(req.Params.Arguments) == 0 {
		return args, nil
	}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if args == nil {
		return map[string]any{}, nil
	}
	return args, nil
}

func findingID(args map[string]any) (state.FindingID, error) {
	value, err := requiredString(args, "id")
	if err != nil {
		return state.FindingID{}, err
	}
	return state.ParseID(value)
}

func requiredString(args map[string]any, key string) (string, error) {
	value, ok, err := optionalString(args, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalString(args map[string]any, key string) (string, bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return "", false, nil
	}
	s, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

func optionalBool(args map[string]any, key string) (bool, bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return false, false, nil
	}
	b, ok := value.(bool)
	if !ok {
		return false, false, fmt.Errorf("%s must be a boolean", key)
	}
	return b, true, nil
}

func structuredResult(payload map[string]any) *mcpsdk.CallToolResult {
	raw, err := json.Marshal(payload)
	if err != nil {
		return toolError(err)
	}
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: string(raw)}},
		StructuredContent: payload,
	}
}

func toolError(err error) *mcpsdk.CallToolResult {
	var result mcpsdk.CallToolResult
	result.SetError(err)
	return &result
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func stringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

func dispositionSchema() map[string]any {
	return map[string]any{
		"type":        "string",
		"description": "finding disposition",
		"enum": []any{
			state.DispositionOpen,
			state.DispositionAccepted,
			state.DispositionDeferred,
			state.DispositionRejected,
		},
	}
}
