package knowledge

import (
	"regexp"
	"testing"
)

func TestUUIDFromSourceHash(t *testing.T) {
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	content := []byte(`{"title":"test"}`)

	id1 := uuidFromSourceHash("drug", content)
	id2 := uuidFromSourceHash("drug", content)
	id3 := uuidFromSourceHash("medical", content)

	if !uuidRe.MatchString(id1) {
		t.Fatalf("id %q is not a valid UUID format", id1)
	}
	if id1 != id2 {
		t.Fatalf("same source+content must produce same ID: %q vs %q", id1, id2)
	}
	if id1 == id3 {
		t.Fatalf("different source must produce different ID")
	}
	t.Logf("drug=%s medical=%s", id1, id3)
}
