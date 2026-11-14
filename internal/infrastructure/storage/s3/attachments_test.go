package s3

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type attachmentCopyTransport func(*http.Request) (*http.Response, error)

func (f attachmentCopyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCopyAttachmentUsesPrivateIndependentKeyAndPreservesMetadata(t *testing.T) {
	source := "attachments/" + uuid.NewString() + "/" + uuid.NewString() + "/" + uuid.NewString()
	target := "attachments/direct/" + uuid.NewString() + "/" + uuid.NewString() + "/" + uuid.NewString()
	calls := 0
	client, err := minio.New("storage.example.invalid", &minio.Options{
		Creds: credentials.NewStaticV4("synthetic-access", "synthetic-secret", ""), Region: "us-east-1", Secure: true,
		Transport: attachmentCopyTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != http.MethodPut || r.URL.Path != "/private-bucket/"+target || strings.TrimPrefix(r.Header.Get("X-Amz-Copy-Source"), "/") != "private-bucket/"+source {
				t.Fatalf("unexpected copy request: %s %s source=%s", r.Method, r.URL.Path, r.Header.Get("X-Amz-Copy-Source"))
			}
			if r.Header.Get("X-Amz-Metadata-Directive") == "REPLACE" || r.Header.Get("X-Amz-Acl") == "public-read" {
				t.Fatal("copy discards integrity metadata or makes content public")
			}
			body := `<CopyObjectResult><ETag>"synthetic"</ETag><LastModified>2026-11-13T00:00:00Z</LastModified></CopyObjectResult>`
			return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": []string{"application/xml"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	storage := &Client{minio: client, bucket: "private-bucket"}
	if err = storage.CopyAttachment(context.Background(), source, target); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{source, source}, {"https://public.invalid/photo.jpg", target}, {source, "recordings/file.mp4"}, {source, "attachments/../../private"}} {
		if err = storage.CopyAttachment(context.Background(), pair[0], pair[1]); err == nil {
			t.Fatal("unsafe copy key accepted", pair)
		}
	}
	if calls != 1 {
		t.Fatal("invalid keys reached object storage", calls)
	}
}
