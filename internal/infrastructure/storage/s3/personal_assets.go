package s3

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
)

// personalAvatarKey builds the server-owned object key for a personal-group avatar.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func personalAvatarKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 4 && parts[0] == "avatars" && parts[1] == "conversations" && attachmentID(parts[2]) && attachmentID(parts[3])
}

// PutPersonalAvatar stores normalized avatar bytes under the server-owned group-avatar key.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - reader: Source content stream or recording-card reader, as defined by its type.
//   - size: Content size in bytes or the requested bounded capacity.
//   - contentType: Declared or detected MIME type of the content.
//   - checksum: Expected content digest used to verify transfer integrity.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) PutPersonalAvatar(ctx context.Context, key string, reader io.Reader, size int64, contentType, checksum string) error {
	if !personalAvatarKey(key) || size < 1 || size > 2<<20 || (contentType != "image/jpeg" && contentType != "image/png") || len(checksum) != 64 {
		return fmt.Errorf("invalid personal avatar object")
	}
	_, err := c.minio.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": checksum}})
	return err
}

// OpenPersonalAsset never creates a public URL. The caller owns and closes the
// reader; Stat forces the lazy MinIO request before any HTTP headers are sent.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: The resulting read closer.
//   - result 2: The resulting count, size or numeric value.
//   - result 3: Detected asset content type.
//   - result 4: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) OpenPersonalAsset(ctx context.Context, key string) (io.ReadCloser, int64, string, error) {
	if !personalAvatarKey(key) && !attachmentKey(key) {
		return nil, 0, "", fmt.Errorf("invalid personal asset key")
	}
	object, err := c.minio.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, "", err
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, 0, "", err
	}
	return object, info.Size, info.ContentType, nil
}

// DeletePersonalAvatar removes only the private object identified by the obsolete avatar key.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) DeletePersonalAvatar(ctx context.Context, key string) error {
	if !personalAvatarKey(key) {
		return fmt.Errorf("invalid personal avatar key")
	}
	return c.minio.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}
