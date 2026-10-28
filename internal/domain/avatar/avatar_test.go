package avatar

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// TestNormalize validates format, crop, dimensions and removal of trailing data.
func TestNormalize(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, 900, 600))
			var input bytes.Buffer
			if format == "png" {
				if err := png.Encode(&input, img); err != nil {
					t.Fatal(err)
				}
			} else if err := jpeg.Encode(&input, img, nil); err != nil {
				t.Fatal(err)
			}
			input.WriteString("synthetic trailing metadata must not survive")
			out, err := Normalize(bytes.NewReader(input.Bytes()), "image/"+format)
			if err != nil {
				t.Fatal(err)
			}
			cfg, typ, err := image.DecodeConfig(bytes.NewReader(out))
			if err != nil || typ != "jpeg" || cfg.Width != 512 || cfg.Height != 512 || len(out) > MaxStoredBytes {
				t.Fatalf("output=%+v type=%s bytes=%d error=%v", cfg, typ, len(out), err)
			}
			if bytes.Contains(out, []byte("synthetic trailing")) {
				t.Fatal("source metadata survived")
			}
		})
	}
}

// TestLargeAvatar exercises a valid PNG larger than the old 1 MiB proxy cap.
func TestLargeAvatar(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 900, 600))
	random := rand.New(rand.NewPCG(1, 2))
	for y := range 600 {
		for x := range 900 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(random.Uint32()), G: uint8(random.Uint32()), B: uint8(random.Uint32()), A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, img); err != nil {
		t.Fatal(err)
	}
	if input.Len() <= 1<<20 || int64(input.Len()) > MaxUploadBytes {
		t.Fatalf("fixture bytes=%d", input.Len())
	}
	out, err := Normalize(bytes.NewReader(input.Bytes()), "image/png")
	if err != nil || len(out) > MaxStoredBytes {
		t.Fatalf("normalize error=%v bytes=%d", err, len(out))
	}
}

// TestNormalizeRejectsUntrustedInputs prevents unsupported or oversized decoding.
func TestNormalizeRejectsUntrustedInputs(t *testing.T) {
	var oversized bytes.Buffer
	if err := png.Encode(&oversized, image.NewRGBA(image.Rect(0, 0, 2049, 1))); err != nil {
		t.Fatal(err)
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{
		{"empty", "image/png", nil}, {"svg", "image/svg+xml", []byte("<svg/>")},
		{"wrong content", "image/png", []byte("not an image")}, {"mime mismatch", "image/jpeg", pngData.Bytes()},
		{"byte limit", "image/png", make([]byte, MaxUploadBytes+1)}, {"dimension limit", "image/png", oversized.Bytes()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Normalize(bytes.NewReader(tc.data), tc.mime); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
