package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "created.txt")

	if err := Write(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "hello\n" {
		t.Errorf("content = %q, want %q", got, "hello\n")
	}

	// Windows does not model Unix permission bits; only the read-only flag
	// survives, so asserting 0600 there would test the platform, not Write.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("mode = %v, want %v", perm, os.FileMode(0o600))
		}
	}
}

func TestWriteReplacesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replaced.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := Write(path, []byte("after\n"), 0o600); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "after\n" {
		t.Errorf("content = %q, want %q", got, "after\n")
	}
}

func TestWriteLeavesNoTempFileBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.txt")

	if err := Write(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %d entries %v, want only the destination", len(entries), names)
	}
}

func TestWriteFailsWhenDirectoryMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent", "file.txt")

	if err := Write(path, []byte("x"), 0o600); err == nil {
		t.Fatal("expected an error writing into a missing directory, got nil")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("destination should not exist after a failed write, stat err = %v", err)
	}
}

func TestWriteDoesNotDisturbDestinationOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kept.txt")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// A directory where the temp file would go cannot be created, so the
	// write fails before the rename; the destination must survive intact.
	if err := Write(filepath.Join(path, "nested"), []byte("new\n"), 0o600); err == nil {
		t.Fatal("expected an error, got nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "original\n" {
		t.Errorf("content = %q, want the original to be untouched", got)
	}
}
