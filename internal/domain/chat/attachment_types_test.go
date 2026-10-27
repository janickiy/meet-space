package chat

import (
	"strings"
	"testing"
)

// TestAttachmentMetadataPolicy covers every extension and its browser MIME aliases.
func TestAttachmentMetadataPolicy(t *testing.T) {
	for extension, format := range attachmentTypes {
		t.Run(extension, func(t *testing.T) {
			for _, hint := range append([]string{"", "application/octet-stream", format.mime}, format.aliases...) {
				request, err := NormalizeInit(InitRequest{
					ClientRequestID: "00000000-0000-4000-8000-000000000001",
					Filename:        "Document" + strings.ToUpper(extension),
					MimeType:        hint,
					Size:            MaxAttachmentBytes,
				})
				if err != nil || request.MimeType != format.mime {
					t.Fatalf("hint=%q got=%q error=%v", hint, request.MimeType, err)
				}
			}
		})
	}
}

// TestAttachmentMetadataBoundaries preserves size, safe-name and MIME restrictions.
func TestAttachmentMetadataBoundaries(t *testing.T) {
	for _, filename := range []string{"program.exe", "script.js", "page.html", "image.svg", "no-extension", "../../file.zip", "bad\x00.zip", "file.zip.exe"} {
		if _, err := NormalizeInit(InitRequest{ClientRequestID: "00000000-0000-4000-8000-000000000001", Filename: filename, Size: 10}); err == nil {
			t.Errorf("accepted unsafe filename %q", filename)
		}
	}
	for _, size := range []int64{0, -1, MaxAttachmentBytes + 1} {
		if _, err := NormalizeInit(InitRequest{ClientRequestID: "00000000-0000-4000-8000-000000000001", Filename: "document.pdf", Size: size}); err == nil {
			t.Errorf("accepted size %d", size)
		}
	}
	if _, err := normalizeAttachmentMIME("document.pdf", "application/zip"); err == nil {
		t.Error("accepted a mismatching MIME hint")
	}
}
