package local

import (
	"fmt"
	"os"
	"path/filepath"
)

// RemoveEmptyTrees removes empty directories below the validated local recording root.
//
// Parameters:
//   - paths: Validated local paths included in this operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func RemoveEmptyTrees(paths ...string) error {
	for _, path := range paths {
		if err := removeEmptyTree(path); err != nil {
			return err
		}
	}

	return nil
}

// removeEmptyTree walks a local subtree and removes directories only after their contents are
// empty.
//
// Parameters:
//   - path: Validated local filesystem or object path.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func removeEmptyTree(path string) error {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local storage dir %s: %w", path, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := removeEmptyTree(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	entries, err = os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read local storage dir %s: %w", path, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove empty local storage dir %s: %w", path, err)
	}

	return nil
}
