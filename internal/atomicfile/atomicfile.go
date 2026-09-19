// Package atomicfile writes a file in one observable step: the content
// either lands at the destination path complete, or the destination is left
// exactly as it was.
//
// VibeConform writes two kinds of file that must never be observed
// half-written: resources it manages on the user's behalf, and
// .vibe/state.yaml. A truncated state file is worse than a missing one,
// because every subsequent reconciliation decision would be made against
// it. See docs/specs/0008-vibe-sync-v1.md.
package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write writes data to path with the given permissions, atomically. It
// writes a temporary file in path's directory, flushes it to disk, and
// renames it over path. If any step fails, path keeps its previous content
// and the temporary file is removed.
//
// The temporary file is created in the destination directory rather than
// the system temp directory so the rename stays within one filesystem,
// where it is atomic.
func Write(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("atomicfile: create temp for %s: %w", path, err)
	}
	tmp := f.Name()
	// Removing the temp file is a no-op once the rename below succeeds.
	defer func() { _ = os.Remove(tmp) }()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("atomicfile: write %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("atomicfile: sync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("atomicfile: close %s: %w", tmp, err)
	}
	// os.CreateTemp always uses 0600; widen or narrow to the caller's mode
	// before the file becomes visible at its destination.
	if err := os.Chmod(tmp, perm); err != nil {
		return fmt.Errorf("atomicfile: chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("atomicfile: rename %s to %s: %w", tmp, path, err)
	}
	return nil
}
