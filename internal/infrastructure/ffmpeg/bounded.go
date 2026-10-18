package ffmpeg

import (
	"context"
	"github.com/janickiy/meet-space/internal/operations"
	"os/exec"
	"syscall"
	"time"
)

// runBounded runs an external process with cancellation and bounded diagnostic output.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - binary: Path to the configured external executable.
//   - args: Command-line, process or protocol arguments.
//
// Returns:
//   - result 1: The collected byte values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func runBounded(ctx context.Context, binary string, args ...string) ([]byte, error) {
	started := time.Now()
	operations.FFmpegActive(1)
	defer func() { operations.FFmpegActive(-1); operations.Observe("ffmpeg", time.Since(started).Seconds()) }()
	cmd := exec.CommandContext(ctx, binary, args...)
	output := &logTail{}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if err != nil {
		operations.Event("ffmpeg_failed")
	}
	return []byte(output.String()), err
}
