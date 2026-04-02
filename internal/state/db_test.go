package state

import (
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_CreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "nested", "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open in non-existent subdir: %v", err)
	}
	db.Close()
}

func TestOpen_MigrateIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := Open(path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open (migrate idempotent): %v", err)
	}
	db2.Close()
}
