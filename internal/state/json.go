package state

import (
	"encoding/json"

	"github.com/michaelquigley/df/dd"
)

func marshalDDJSON(source any) ([]byte, error) {
	return dd.UnbindJSON(source)
}

func marshalDDJSONLine(source any) ([]byte, error) {
	data, err := dd.Unbind(source)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	return raw, nil
}

func bindDDJSON(target any, raw []byte) error {
	return dd.BindJSON(target, raw)
}

func marshalDDJSONMap[T any](source map[string]T) ([]byte, error) {
	out := make(map[string]any, len(source))
	for key, value := range source {
		data, err := dd.Unbind(value)
		if err != nil {
			return nil, err
		}
		out[key] = data
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	return raw, nil
}

func bindDDJSONMap[T any](raw []byte) (map[string]T, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	out := make(map[string]T, len(fields))
	for key, fieldRaw := range fields {
		var value T
		if err := dd.BindJSON(&value, fieldRaw); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, nil
}

func marshalDDJSONSlice[T any](source []T) ([]byte, error) {
	out := make([]map[string]any, 0, len(source))
	for _, value := range source {
		data, err := dd.Unbind(value)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	return raw, nil
}
