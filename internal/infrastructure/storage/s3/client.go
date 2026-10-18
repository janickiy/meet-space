package s3

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client owns the configured private object-storage clients and connection settings.
type Client struct {
	minio        *minio.Client
	presignMinio *minio.Client
	bucket       string
	endpoint     string
	accessKey    string
	secretKey    string
	useSSL       bool
}

// UploadedObject contains the stored key, size and metadata of an uploaded recording artifact.
type UploadedObject struct {
	Bucket    string
	ObjectKey string
	SizeBytes int64
}

// CompletedRecord groups final objects found in the bucket for one completed recording.
type CompletedRecord struct {
	RecordID         string    `json:"recordId"`
	Status           string    `json:"status"`
	Bucket           string    `json:"bucket"`
	PreviewObjectKey string    `json:"previewObjectKey,omitempty"`
	PreviewURL       string    `json:"previewUrl,omitempty"`
	FinalObjectKey   string    `json:"finalObjectKey,omitempty"`
	FinalURL         string    `json:"finalUrl,omitempty"`
	SizeBytes        int64     `json:"sizeBytes,omitempty"`
	LastModified     time.Time `json:"lastModified,omitempty"`
}

// NewClient constructs the client with the dependencies and limits required for private
// recording and attachment storage.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - endpoint: Operator-configured service endpoint.
//   - accessKey: Private object-storage access-key identifier.
//   - secretKey: Private object-storage signing secret; never log or expose it.
//   - bucket: Private object-storage bucket.
//   - useSSL: Whether the object-storage client must use TLS.
//
// Returns:
//   - result 1: The constructed or loaded client.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewClient(ctx context.Context, endpoint string, accessKey string, secretKey string, bucket string, useSSL bool) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := minio.New(endpoint, &minio.Options{
		Creds:      credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:     useSSL,
		Transport:  boundedTransport(),
		MaxRetries: 3,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create minio bucket: %w", err)
		}
	}

	return &Client{
		minio:     client,
		bucket:    bucket,
		endpoint:  endpoint,
		accessKey: accessKey,
		secretKey: secretKey,
		useSSL:    useSSL,
	}, nil
}

// boundedTransport creates an HTTP transport with bounded connection and request timeouts.
//
// Returns:
//   - result 1: The constructed or loaded transport.
func boundedTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	t.ResponseHeaderTimeout = 10 * time.Second
	t.TLSHandshakeTimeout = 5 * time.Second
	t.MaxIdleConnsPerHost = 8
	return t
}

// Check checks the configured component or dependency without creating user work.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) Check(ctx context.Context) error {
	exists, err := c.minio.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("storage bucket unavailable")
	}
	return nil
}

// SetPublicEndpoint updates public endpoint for private recording and attachment storage.
//
// Parameters:
//   - endpoint: Operator-configured service endpoint.
func (c *Client) SetPublicEndpoint(endpoint string) {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		c.presignMinio = nil
		return
	}
	secure := c.useSSL
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			c.presignMinio = nil
			return
		}
		endpoint = parsed.Host
		secure = parsed.Scheme == "https"
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(c.accessKey, c.secretKey, ""),
		Secure:    secure,
		Region:    "us-east-1",
		Transport: c.publicEndpointTransport(endpoint),
	})
	if err != nil {
		c.presignMinio = nil
		return
	}
	c.presignMinio = client
}

// publicEndpointTransport rewrites only the configured object-storage origin for public signed
// URLs.
//
// Parameters:
//   - publicEndpoint: Operator-configured public object-storage origin.
//
// Returns:
//   - result 1: The resulting round tripper.
func (c *Client) publicEndpointTransport(publicEndpoint string) http.RoundTripper {
	transport := boundedTransport()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	internalEndpoint := c.endpoint

	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		if address == publicEndpoint && internalEndpoint != "" {
			address = internalEndpoint
		}

		return dialer.DialContext(ctx, network, address)
	}

	return transport
}

// Bucket returns the configured private bucket name used by this storage client.
//
// Returns:
//   - result 1: Configured private object-storage bucket name.
func (c *Client) Bucket() string {
	return c.bucket
}

// UploadFile uploads a local recording artifact and returns its stored object metadata.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - objectKey: Server-generated private object-storage key.
//   - path: Validated local filesystem or object path.
//   - contentType: Declared or detected MIME type of the content.
//
// Returns:
//   - result 1: The resulting uploaded object.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) UploadFile(ctx context.Context, objectKey string, path string, contentType string) (UploadedObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	local, err := os.Stat(path)
	if err != nil {
		return UploadedObject{}, err
	}
	info, err := c.minio.FPutObject(ctx, c.bucket, objectKey, path, minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"file-name": filepath.Base(path),
		},
	})
	if err != nil {
		return UploadedObject{}, fmt.Errorf("upload %s to minio: %w", objectKey, err)
	}
	verified, err := c.minio.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil || verified.Size != local.Size() {
		return UploadedObject{}, fmt.Errorf("uploaded object verification failed")
	}

	return UploadedObject{Bucket: c.bucket, ObjectKey: objectKey, SizeBytes: info.Size}, nil
}

// RemovePrefix removes objects only within the validated server-owned recording prefix.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - prefix: Server-controlled object or key prefix.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) RemovePrefix(ctx context.Context, prefix string) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Synchronous iterators stop with their consumer. The channel SDK APIs can
	// retain listing/result goroutines when deletion returns its first error.
	var listingErr error
	objects := func(yield func(minio.ObjectInfo) bool) {
		for object := range c.minio.ListObjectsIter(ctx, c.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			// The bulk-delete iterator does not propagate ObjectInfo.Err itself.
			if object.Err != nil {
				listingErr = object.Err
				return
			}
			if !yield(object) {
				return
			}
		}
	}
	results, err := c.minio.RemoveObjectsWithIter(ctx, c.bucket, objects, minio.RemoveObjectsOptions{})
	if err != nil {
		return fmt.Errorf("remove prefix from minio: %w", err)
	}
	for removeErr := range results {
		if removeErr.Err != nil {
			return fmt.Errorf("remove %s from minio: %w", removeErr.ObjectName, removeErr.Err)
		}
	}
	if listingErr != nil {
		return fmt.Errorf("list prefix from minio: %w", listingErr)
	}
	return ctx.Err()
}

// ListCompletedRecords lists completed recording artifacts stored under the configured private
// prefix.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - limit: Maximum number of results or operations in this batch.
//
// Returns:
//   - result 1: The collected completed record values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) ListCompletedRecords(ctx context.Context, limit int) ([]CompletedRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	byRecordID := make(map[string]*CompletedRecord)
	objects := c.minio.ListObjects(ctx, c.bucket, minio.ListObjectsOptions{
		Prefix:    "records/",
		Recursive: true,
	})
	for object := range objects {
		if object.Err != nil {
			return nil, fmt.Errorf("list completed records from minio: %w", object.Err)
		}
		recordID, fileName, ok := completedRecordObject(object.Key)
		if !ok {
			continue
		}
		item := byRecordID[recordID]
		if item == nil {
			item = &CompletedRecord{
				RecordID: recordID,
				Status:   "ready",
				Bucket:   c.bucket,
			}
			byRecordID[recordID] = item
		}
		if object.LastModified.After(item.LastModified) {
			item.LastModified = object.LastModified
		}
		if fileName == "final.mp4" {
			item.FinalObjectKey = object.Key
			item.SizeBytes = object.Size
		}
		if fileName == "preview.jpg" {
			item.PreviewObjectKey = object.Key
		}
	}

	result := make([]CompletedRecord, 0, len(byRecordID))
	for _, item := range byRecordID {
		if item.FinalObjectKey == "" || item.PreviewObjectKey == "" {
			continue
		}
		if item.FinalObjectKey != "" {
			url, err := c.PresignedGetURL(ctx, item.FinalObjectKey, 24*time.Hour)
			if err != nil {
				return nil, err
			}
			item.FinalURL = url
		}
		if item.PreviewObjectKey != "" {
			url, err := c.PresignedGetURL(ctx, item.PreviewObjectKey, 24*time.Hour)
			if err != nil {
				return nil, err
			}
			item.PreviewURL = url
		}
		result = append(result, *item)
	}
	sort.Slice(result,

		func(i int, j int) bool {
			return result[i].LastModified.After(result[j].LastModified)
		})
	if len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// completedRecordObject recognizes the object keys that belong to a completed recording
// artifact.
//
// Parameters:
//   - key: Storage, cache or cryptographic key used by the operation.
//
// Returns:
//   - result 1: Recording identifier decoded from the object key.
//   - result 2: Recognized final.mp4 or preview.jpg artifact filename.
//   - result 3: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func completedRecordObject(key string) (string, string, bool) {
	parts := strings.Split(strings.Trim(key, "/"), "/")
	if len(parts) != 3 || parts[0] != "records" {
		return "", "", false
	}
	if parts[2] != "final.mp4" && parts[2] != "preview.jpg" {
		return "", "", false
	}

	return parts[1], parts[2], true
}

// PresignedGetURL creates a temporary signed read URL for a private object.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - objectKey: Server-generated private object-storage key.
//   - expiry: Expiration deadline or validity interval, as defined by its type.
//
// Returns:
//   - result 1: Temporary signed URL for the private object.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	if strings.TrimSpace(objectKey) == "" {
		return "", nil
	}
	client := c.minio
	if c.presignMinio != nil {
		client = c.presignMinio
	}
	objectURL, err := client.PresignedGetObject(ctx, c.bucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %s: %w", objectKey, err)
	}

	return objectURL.String(), nil
}
