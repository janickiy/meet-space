package ffmpeg

import "sync"

const maxStderrBytes = 64 * 1024

// logTail retains only a bounded tail of external-process diagnostic output.
type logTail struct {
	mu   sync.Mutex
	data []byte
}

// Write accepts output bytes through the io.Writer contract while enforcing the component's
// buffering rules.
//
// Parameters:
//   - p: Input bytes consumed by this operation.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (b *logTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= maxStderrBytes {
		b.data = append(b.data[:0], p[n-maxStderrBytes:]...)
		return n, nil
	}
	if overflow := len(b.data) + n - maxStderrBytes; overflow > 0 {
		b.data = b.data[:copy(b.data, b.data[overflow:])]
	}
	b.data = append(b.data, p...)
	return n, nil
}

// String returns the buffered diagnostic text in its bounded string representation.
//
// Returns:
//   - result 1: Bounded buffered diagnostic output.
func (b *logTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
