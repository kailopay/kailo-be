package platform

import (
	"testing"
	"testing/fstest"
)

func TestDiscoverMigrationsRequiresOrderedPairs(t *testing.T) {
	migrationFS := fstest.MapFS{
		"000002_second.up.sql":   {Data: []byte("select 2")},
		"000002_second.down.sql": {Data: []byte("select -2")},
		"000001_first.up.sql":    {Data: []byte("select 1")},
		"000001_first.down.sql":  {Data: []byte("select -1")},
	}

	migrations, err := DiscoverMigrations(migrationFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("migrations = %#v", migrations)
	}
	if migrations[0].Checksum == "" || migrations[0].UpSQL != "select 1" {
		t.Fatalf("first migration = %#v", migrations[0])
	}
}

func TestDiscoverMigrationsRejectsMissingDownFile(t *testing.T) {
	migrationFS := fstest.MapFS{
		"000001_first.up.sql": {Data: []byte("select 1")},
	}

	if _, err := DiscoverMigrations(migrationFS); err == nil {
		t.Fatal("DiscoverMigrations() error = nil")
	}
}

func TestDiscoverMigrationsRejectsVersionGap(t *testing.T) {
	migrationFS := fstest.MapFS{
		"000002_second.up.sql":   {Data: []byte("select 2")},
		"000002_second.down.sql": {Data: []byte("select -2")},
	}

	if _, err := DiscoverMigrations(migrationFS); err == nil {
		t.Fatal("DiscoverMigrations() error = nil")
	}
}
