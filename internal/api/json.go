package api

import (
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/michaelquigley/df/dd"
)

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw, err := encodeJSON(value)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	raw, _ := json.Marshal(map[string]any{
		"error": message,
	})
	_, _ = w.Write(append(raw, '\n'))
}

func encodeJSON(value any) ([]byte, error) {
	switch v := value.(type) {
	case nil:
		return []byte("{}\n"), nil
	case map[string]any:
		raw, err := json.MarshalIndent(v, "", "  ")
		return append(raw, '\n'), err
	case []map[string]any:
		raw, err := json.MarshalIndent(v, "", "  ")
		return append(raw, '\n'), err
	}
	rv := reflect.ValueOf(value)
	if rv.IsValid() && rv.Kind() == reflect.Slice {
		out := make([]map[string]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			data, err := dd.Unbind(rv.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			out = append(out, data)
		}
		raw, err := json.MarshalIndent(out, "", "  ")
		return append(raw, '\n'), err
	}
	return dd.UnbindJSON(value)
}
