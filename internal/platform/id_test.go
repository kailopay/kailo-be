package platform

import (
	"regexp"
	"testing"
)

func TestNewIDReturnsDistinctUUIDs(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	first, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if !pattern.MatchString(first) || !pattern.MatchString(second) {
		t.Fatalf("IDs are not UUID v4: %q %q", first, second)
	}
	if first == second {
		t.Fatal("NewID() returned a duplicate")
	}
}
