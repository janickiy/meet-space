// Package avatar validates and normalizes small raster profile images.
package avatar

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const MaxUploadBytes int64 = 2 << 20
const MaxStoredBytes = 256 << 10

// Normalize validates body against contentType, caps decoding dimensions, and
// returns a square JPEG without source metadata. Callers must bound concurrency.
func Normalize(body io.Reader, contentType string) ([]byte, error) {
	typ := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if typ != "image/jpeg" && typ != "image/png" {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "avatar must be JPEG or PNG")
	}
	raw, err := io.ReadAll(io.LimitReader(body, MaxUploadBytes+1))
	if err != nil || len(raw) == 0 || int64(len(raw)) > MaxUploadBytes {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "avatar exceeds 2 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || "image/"+format != typ || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "invalid avatar image or dimensions")
	}
	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, apperrors.ErrInvalidInput
	}
	side := min(config.Width, config.Height)
	size := min(side, 512)
	bounds := source.Bounds()
	x0, y0 := bounds.Min.X+(config.Width-side)/2, bounds.Min.Y+(config.Height-side)/2
	thumbnail := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			pixel := color.NRGBAModel.Convert(source.At(x0+x*side/size, y0+y*side/size)).(color.NRGBA)
			a := uint32(pixel.A)
			flatten := func(value uint8) uint8 { return uint8((uint32(value)*a + 255*(255-a)) / 255) }
			thumbnail.SetRGBA(x, y, color.RGBA{R: flatten(pixel.R), G: flatten(pixel.G), B: flatten(pixel.B), A: 255})
		}
	}
	var output boundedBuffer
	if err := jpeg.Encode(&output, thumbnail, &jpeg.Options{Quality: 85}); err != nil {
		return nil, apperrors.ErrInvalidInput
	}
	return output.Bytes(), nil
}

type boundedBuffer struct{ bytes.Buffer }

// Write appends data only while the normalized image stays within its budget.
func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > MaxStoredBytes {
		return 0, apperrors.ErrInvalidInput
	}
	return b.Buffer.Write(data)
}
