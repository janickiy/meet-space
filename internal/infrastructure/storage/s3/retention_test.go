package s3

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/records"
	"github.com/minio/minio-go/v7"
)

// Only a fresh test bucket on loopback is used; no application objects are read.
func TestRemoveRecordingArtifacts(t *testing.T) {
	endpoint := os.Getenv("MEETSPACE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set MEETSPACE_TEST_S3_ENDPOINT for isolated storage integration test")
	}
	if endpoint != "127.0.0.1:9000" {
		t.Fatal("storage test requires loopback MinIO")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	bucket := "test-record-delete-" + uuid.NewString()
	client, err := NewClient(ctx, endpoint, os.Getenv("MEETSPACE_TEST_S3_KEY"), os.Getenv("MEETSPACE_TEST_S3_SECRET"), bucket, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		for object := range client.minio.ListObjectsIter(cleanup, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
			if object.Err != nil {
				t.Error(object.Err)
				break
			}
			if err := client.minio.RemoveObject(cleanup, bucket, object.Key, minio.RemoveObjectOptions{VersionID: object.VersionID}); err != nil {
				t.Error(err)
			}
		}
		if err := client.minio.RemoveBucket(cleanup, bucket); err != nil {
			t.Error(err)
		}
	})
	if err := client.minio.SetBucketVersioning(ctx, bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
		t.Fatal(err)
	}
	cid := uuid.NewString()
	record := records.Record{UUID: uuid.NewString(), ConferenceID: cid, PlatformConferenceID: &cid, Mode: records.ModeComposite}
	prefixes, err := recordingPrefixes(record)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{prefixes[0] + "segment.webm", prefixes[1] + "final.mp4", prefixes[1] + "preview.jpg", "recordings/" + cid + "/" + uuid.NewString() + "/final.mp4", "chat/attachment.png"}
	for _, key := range keys {
		for range 2 {
			if _, err := client.minio.PutObject(ctx, bucket, key, strings.NewReader("test"), 4, minio.PutObjectOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for range 2 { // Repeating storage deletion must be safe after partial retries.
		if err := client.RemoveRecording(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	remaining := 0
	for object := range client.minio.ListObjectsIter(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			t.Fatal(object.Err)
		}
		if strings.HasPrefix(object.Key, prefixes[0]) || strings.HasPrefix(object.Key, prefixes[1]) {
			t.Fatalf("recording artifact version remains: %s", object.Key)
		}
		remaining++
	}
	if remaining != 4 {
		t.Fatalf("other recording/chat versions affected: %d remain", remaining)
	}
}
