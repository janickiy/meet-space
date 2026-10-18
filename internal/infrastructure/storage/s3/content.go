package s3

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"io"
	"strings"
)

// limitedRecording enforces the read budget and closes its private object stream.
type limitedRecording struct {
	reader io.Reader
	object io.Closer
}

// Read copies at most the destination buffer size from the owned stream while preserving its
// read and cancellation limits.
//
// Parameters:
//   - p: Destination byte buffer filled from the stream.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *limitedRecording) Read(p []byte) (int, error) { return r.reader.Read(p) }

// Close closes the component-owned resources and ends their lifecycle.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *limitedRecording) Close() error { return r.object.Close() }

// OpenRecording opens the private recording audio or media stream using its stored artifact
// identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - maximum: Maximum permitted size or number of characters.
//
// Returns:
//   - result 1: The resulting read closer.
//   - result 2: The resulting count, size or numeric value.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) OpenRecording(ctx context.Context, key string, maximum int64) (io.ReadCloser, int64, error) {
	parts := strings.Split(key, "/")
	if maximum <= 0 || len(parts) != 6 || parts[0] != "recordings" || !attachmentID(parts[1]) || !attachmentID(parts[2]) || parts[3] != "artifacts" || !attachmentID(parts[4]) || parts[5] != "final.mp4" {
		return nil, 0, fmt.Errorf("invalid private recording key")
	}
	object, err := c.minio.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("private recording unavailable")
	}
	info, err := object.Stat()
	if err != nil {
		object.Close()
		return nil, 0, fmt.Errorf("private recording unavailable")
	}
	if info.Size <= 0 || info.Size > maximum {
		object.Close()
		return nil, 0, fmt.Errorf("recording input exceeds budget")
	}
	return &limitedRecording{reader: io.LimitReader(object, maximum+1), object: object}, info.Size, nil
}
