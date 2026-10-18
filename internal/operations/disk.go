package operations

import (
	"context"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// ValidateStorage checks that the configured recording directory is usable before processing
// starts.
//
// Parameters:
//   - path: Validated local filesystem or object path.
//   - minimum: Minimum required free storage in bytes.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func ValidateStorage(path string, minimum uint64) error {
	if path == "" {
		return fmt.Errorf("STORAGE_PATH must not be empty")
	}
	if err := os.MkdirAll(path, 0750); err != nil {
		return fmt.Errorf("storage directory unavailable")
	}
	file, err := os.CreateTemp(path, ".readiness-")
	if err != nil {
		return fmt.Errorf("storage is not writable")
	}
	_ = file.Close()
	_ = os.Remove(file.Name())
	return DiskCheck(path, minimum)(context.Background())
}

// DiskCheck creates the readiness probe for the configured storage free-space reserve.
//
// Parameters:
//   - path: Validated local filesystem or object path.
//   - minimum: Minimum required free storage in bytes.
//
// Returns:
//   - result 1: The resulting check.
func DiskCheck(path string, minimum uint64) Check {
	return func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var stat unix.Statfs_t
		if err := unix.Statfs(path, &stat); err != nil {
			return fmt.Errorf("storage stat failed")
		}
		free := uint64(stat.Bavail) * uint64(stat.Bsize)
		State("disk_free_bytes", float64(free))
		if free < minimum {
			Event("disk_low")
			return fmt.Errorf("storage free space below reserve")
		}
		return nil
	}
}
