//go:build !no_fts5
// +build !no_fts5

package memory

import (
	"os"
	"os/exec"
	"testing"
)

func TestFTS5Available(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"

	db, err := initColdDB(dbPath)
	if err != nil {
		t.Skipf("FTS5 not available: %v", err)
	}
	db.Close()
	os.RemoveAll(tmpDir)
}

func TestRequiresFTS5(t *testing.T) {
	if os.Getenv("SKIP_FTS5_TESTS") != "" {
		t.Skip("FTS5 tests skipped by environment variable")
	}

	tmpDir := t.TempDir()
	db, err := initColdDB(tmpDir + "/test.db")
	if err != nil {
		t.Skipf("FTS5 not available: %v", err)
	}
	db.Close()

	cmd := exec.Command("go", "test", "-v", "-tags", "fts5 jieba", "-run", "TestNewMemoryCache|TestCold|TestContext|TestSearch", ".")
	cmd.Dir = "."
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		t.Errorf("FTS5 tests failed: %v", err)
	}
}
