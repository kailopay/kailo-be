package platform

import (
	"context"
	"testing"
)

func TestOpenRejectsEmptyDSN(t *testing.T) {
	cfg := DBConfig{}

	_, _, err := Open(context.Background(), cfg, nil)
	if err == nil {
		t.Fatal("Open() error = nil, want empty DSN error")
	}
}
