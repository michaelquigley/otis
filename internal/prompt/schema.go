package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type ReviewerOutput struct {
	Findings []ReviewerFinding `json:"findings"`
}

type ReviewerFinding struct {
	ID           string           `json:"id,omitempty"`
	Severity     string           `json:"severity"`
	Title        string           `json:"title"`
	Location     ReviewerLocation `json:"location"`
	BokRefs      []string         `json:"bok_refs"`
	Description  string           `json:"description"`
	SuggestedFix string           `json:"suggested_fix"`
}

type ReviewerLocation struct {
	File  string `json:"file"`
	Lines string `json:"lines"`
}

// ReviewerOutputSchema returns the reviewer output schema with a finding cap.
func ReviewerOutputSchema(maxFindings int) json.RawMessage {
	findingsArray := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required": []string{
				"severity",
				"title",
				"location",
				"bok_refs",
				"description",
				"suggested_fix",
			},
			"properties": map[string]any{
				"id": map[string]any{
					"type": "string",
				},
				"severity": map[string]any{
					"type": "string",
					"enum": []string{"low", "medium", "high"},
				},
				"title": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
				"location": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"file", "lines"},
					"properties": map[string]any{
						"file": map[string]any{
							"type":      "string",
							"minLength": 1,
						},
						"lines": map[string]any{
							"type":      "string",
							"minLength": 1,
						},
					},
				},
				"bok_refs": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
				},
				"description": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
				"suggested_fix": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
		},
	}
	if maxFindings > 0 {
		findingsArray["maxItems"] = maxFindings
	}
	doc := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"findings"},
		"properties": map[string]any{
			"findings": findingsArray,
		},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return json.RawMessage(`{"type":"object","required":["findings"],"properties":{"findings":{"type":"array"}}}`)
	}
	return raw
}

// ValidateReviewerOutput validates raw reviewer output against a schema.
func ValidateReviewerOutput(raw json.RawMessage, schemaRaw json.RawMessage) error {
	compiled, err := compileSchema(schemaRaw)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("invalid reviewer output json: %w", err)
	}
	if err := compiled.Validate(inst); err != nil {
		return fmt.Errorf("reviewer output schema violation: %w", err)
	}
	return nil
}

// ParseReviewerOutput validates and unmarshals reviewer output.
func ParseReviewerOutput(raw json.RawMessage, schemaRaw json.RawMessage) (ReviewerOutput, error) {
	if err := ValidateReviewerOutput(raw, schemaRaw); err != nil {
		return ReviewerOutput{}, err
	}
	var output ReviewerOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return ReviewerOutput{}, fmt.Errorf("parse reviewer output: %w", err)
	}
	return output, nil
}

func compileSchema(schemaRaw json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaRaw))
	if err != nil {
		return nil, fmt.Errorf("invalid embedded reviewer output schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("reviewer-output.schema.json", doc); err != nil {
		return nil, fmt.Errorf("register reviewer output schema: %w", err)
	}
	compiled, err := compiler.Compile("reviewer-output.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile reviewer output schema: %w", err)
	}
	return compiled, nil
}
