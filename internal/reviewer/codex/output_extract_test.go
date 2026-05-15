package codex

import "testing"

func TestExtractReviewOutput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "direct json",
			input: `{"findings":[]}`,
			want:  `{"findings":[]}`,
		},
		{
			name: "fenced json",
			input: "```json\n" +
				`{"findings":[]}` + "\n" +
				"```",
			want: `{"findings":[]}`,
		},
		{
			name: "prose around json",
			input: "before\n" +
				`{"findings":[]}` + "\n" +
				"after",
			want: `{"findings":[]}`,
		},
		{
			name:    "empty",
			input:   " \n\t ",
			wantErr: true,
		},
		{
			name:    "non json",
			input:   "not json",
			wantErr: true,
		},
		{
			name:    "json array",
			input:   `[{"findings":[]}]`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractReviewOutput([]byte(test.input))
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != test.want {
				t.Fatalf("output mismatch: got %s want %s", got, test.want)
			}
		})
	}
}
