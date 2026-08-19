package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
)

var migrationNamePattern = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.(up|down)\.sql$`)

type Migration struct {
	Version  int64
	Name     string
	UpSQL    string
	DownSQL  string
	Checksum string
}

func DiscoverMigrations(migrationFS fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("reading migration directory: %w", err)
	}

	type pair struct {
		name string
		up   string
		down string
	}
	pairs := make(map[int64]pair)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationNamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing migration version: %w", err)
		}
		contents, err := fs.ReadFile(migrationFS, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading migration %q: %w", entry.Name(), err)
		}
		current := pairs[version]
		if current.name != "" && current.name != matches[2] {
			return nil, fmt.Errorf("migration version %d has conflicting names", version)
		}
		current.name = matches[2]
		if matches[3] == "up" {
			current.up = string(contents)
		} else {
			current.down = string(contents)
		}
		pairs[version] = current
	}

	versions := make([]int64, 0, len(pairs))
	for version := range pairs {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	migrations := make([]Migration, 0, len(versions))
	for index, version := range versions {
		if version != int64(index+1) {
			return nil, fmt.Errorf("migration sequence has a gap before version %d", version)
		}
		current := pairs[version]
		if current.up == "" || current.down == "" {
			return nil, fmt.Errorf("migration version %d requires up and down files", version)
		}
		digest := sha256.Sum256([]byte(current.up))
		migrations = append(migrations, Migration{
			Version:  version,
			Name:     current.name,
			UpSQL:    current.up,
			DownSQL:  current.down,
			Checksum: hex.EncodeToString(digest[:]),
		})
	}
	return migrations, nil
}

func ApplyMigrations(ctx context.Context, db *sql.DB, migrationFS fs.FS) error {
	if db == nil {
		return errors.New("migration database is required")
	}
	migrations, err := DiscoverMigrations(migrationFS)
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("creating schema migrations table: %w", err)
	}

	applied := make(map[int64]string)
	rows, err := db.QueryContext(ctx, "SELECT version, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return fmt.Errorf("listing applied migrations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version int64
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return fmt.Errorf("scanning applied migration: %w", err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating applied migrations: %w", err)
	}

	for _, migration := range migrations {
		if checksum, ok := applied[migration.Version]; ok {
			if checksum != migration.Checksum {
				return fmt.Errorf("migration version %d checksum changed", migration.Version)
			}
			continue
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning migration %d: %w", migration.Version, err)
		}
		if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying migration %d: %w", migration.Version, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations(version, name, checksum) VALUES ($1, $2, $3)",
			migration.Version,
			migration.Name,
			migration.Checksum,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", migration.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", migration.Version, err)
		}
	}
	return nil
}
