package openaicheck

import (
	"testing"

	"github.com/google/uuid"
)

// Saved reports are addressed through the shared history routes, which
// validate ids with uuid.Parse.
func TestNewIDIsUUIDv4(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id := newID()
		parsed, err := uuid.Parse(id)
		if err != nil {
			t.Fatalf("newID() = %q is not a UUID: %v", id, err)
		}
		if parsed.Version() != 4 {
			t.Fatalf("newID() = %q has version %d, want 4", id, parsed.Version())
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}
