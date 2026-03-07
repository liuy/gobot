package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLongterm_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "nonexistent.md")

	content, err := loadLongterm(filePath)

	if err != nil {
		t.Errorf("Expected no error for missing file, got: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string for missing file, got: %q", content)
	}
}

func TestLoadLongterm_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "longterm.md")

	expectedContent := "# About Me\n\nI am an AI assistant."
	if err := os.WriteFile(filePath, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	content, err := loadLongterm(filePath)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if content != expectedContent {
		t.Errorf("Expected %q, got %q", expectedContent, content)
	}
}

func TestLoadLongterm_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.md")

	if err := os.WriteFile(filePath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	content, err := loadLongterm(filePath)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if content != "" {
		t.Errorf("Expected empty string, got %q", content)
	}
}

func TestSaveLongterm_CreateFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "longterm.md")

	content := "# Test\n\nTest content"
	err := saveLongterm(filePath, content)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	loaded, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(loaded) != content {
		t.Errorf("Expected %q, got %q", content, string(loaded))
	}
}

func TestSaveLongterm_OverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "longterm.md")

	if err := os.WriteFile(filePath, []byte("old content"), 0644); err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	newContent := "new content"
	err := saveLongterm(filePath, newContent)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	loaded, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	if string(loaded) != newContent {
		t.Errorf("Expected %q, got %q", newContent, string(loaded))
	}
}

func TestSaveLongterm_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "longterm.md")

	content := "test content"
	err := saveLongterm(filePath, content)

	if err != nil {
		t.Fatalf("saveLongterm failed: %v", err)
	}

	tmpFile := filePath + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("Temporary file %q should not exist after successful write", tmpFile)
	}
}
