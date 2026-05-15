package reviewer

import (
	"testing"

	"github.com/michaelquigley/otis/internal/prompt"
)

func TestParseCLIOutputEnvelope(t *testing.T) {
	schema := prompt.ReviewerOutputSchema(3)
	raw, output, err := ParseCLIOutput([]byte(`{"type":"final","result":"{\"findings\":[]}"}`), schema)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(raw) != `{"findings":[]}` {
		t.Fatalf("raw = %s", raw)
	}
	if len(output.Findings) != 0 {
		t.Fatalf("findings = %d", len(output.Findings))
	}
}
