package s3

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// CheckAttachmentPrivacy rejects object-storage settings that expose private chat attachments
// through a public bucket policy.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) CheckAttachmentPrivacy(ctx context.Context) error {
	policy, err := c.minio.GetBucketPolicy(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("check private attachment bucket: %w", err)
	}
	if strings.TrimSpace(policy) != "" {
		return fmt.Errorf("attachments require a private bucket without a bucket policy")
	}
	return nil
}

// PutAttachment stores attachment bytes under a server-generated private object key.
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
func (c *Client) PutAttachment(ctx context.Context, key string, reader io.Reader, size int64, contentType, checksum string) error {
	if !attachmentKey(key) {
		return fmt.Errorf("invalid attachment key")
	}
	_, err := c.minio.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType, UserMetadata: map[string]string{"sha256": checksum}})
	return err
}

// StatAttachment reads the actual object size and metadata before upload finalization.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: The resulting count, size or numeric value.
//   - result 2: Actual stored object content type.
//   - result 3: Stored SHA-256 content digest used for finalization.
//   - result 4: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) StatAttachment(ctx context.Context, key string) (int64, string, string, error) {
	if !attachmentKey(key) {
		return 0, "", "", fmt.Errorf("invalid attachment key")
	}
	object, err := c.minio.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return 0, "", "", err
	}
	return object.Size, object.ContentType, object.UserMetadata["Sha256"], nil
}

// AttachmentDownloadURL creates a short-lived signed URL that requests attachment download.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - filename: Safe download filename included in the content disposition.
//   - expiry: Expiration deadline or validity interval, as defined by its type.
//
// Returns:
//   - result 1: Short-lived signed attachment-download URL.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) AttachmentDownloadURL(ctx context.Context, key, filename string, expiry time.Duration) (string, error) {
	if !attachmentKey(key) || expiry <= 0 || expiry > 5*time.Minute {
		return "", fmt.Errorf("invalid attachment download parameters")
	}
	client := c.minio
	if c.presignMinio != nil {
		client = c.presignMinio
	}
	params := url.Values{}
	params.Set("response-content-disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	params.Set("response-content-type", "application/octet-stream")
	params.Set("response-cache-control", "private, no-store")
	objectURL, err := client.PresignedGetObject(ctx, c.bucket, key, expiry, params)
	if err != nil {
		return "", err
	}
	return objectURL.String(), nil
}

// CleanAttachmentObjects removes only the exact attachment prefix objects that are not the
// retained finalized object.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - prefix: Server-controlled object or key prefix.
//   - keep: Exact finalized object key that cleanup must retain.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) CleanAttachmentObjects(ctx context.Context, prefix, keep string) error {
	parts := attachmentParts(strings.TrimSuffix(prefix, "/"))
	if len(parts) != 3 || parts[0] != "attachments" || !attachmentID(parts[1]) || !attachmentID(parts[2]) || !strings.HasSuffix(prefix, "/") || (keep != "" && (!attachmentKey(keep) || !strings.HasPrefix(keep, prefix))) {
		return fmt.Errorf("invalid attachment cleanup prefix")
	}
	// The synchronous iterator ends with its caller. A deletion error must not leave an SDK producer
	// blocked on a full channel.
	for object := range c.minio.ListObjectsIter(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return object.Err
		}
		if object.Key == keep {
			continue
		}
		if err := c.minio.RemoveObject(ctx, c.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// attachmentID validates an attachment identifier before constructing an object path.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func attachmentID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// attachmentKey builds the server-owned private object key for an attachment.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func attachmentKey(key string) bool {
	parts := attachmentParts(key)
	return len(parts) == 4 && parts[0] == "attachments" && attachmentID(parts[1]) && attachmentID(parts[2]) && attachmentID(parts[3])
}

// attachmentParts validates and splits an attachment object key into its expected path
// components.
//
// Direct files have a distinct prefix; both scopes still require canonical UUIDs.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: The collected string values.
func attachmentParts(key string) []string {
	parts := strings.Split(key, "/")
	if len(parts) > 1 && parts[1] == "direct" {
		parts = append(parts[:1], parts[2:]...)
	}
	return parts
}
