package workflow

import (
	"os"
	"strings"
	"testing"
)

// A full history replay is exercised against the real Temporal service by the
// integration test. This guard prevents the common sources of nondeterminism
// from entering the workflow definition before that replay test runs.
func TestWorkflowDefinitionContainsNoDirectNondeterministicIO(t *testing.T) {
	data, err := os.ReadFile("release.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, forbidden := range []string{"time.Now(", "rand.", "net/http", "pgx", "database/sql"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("workflow source contains forbidden operation %q", forbidden)
		}
	}
}
