package records

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/conferences"
)

// LibraryQuery pages recordings independently of a meeting's lifecycle or chat visibility.
type LibraryQuery struct {
	Cursor string
	Limit  int
}

type LibraryCursor struct {
	At time.Time `json:"at"`
	ID int64     `json:"id"`
}

func (q LibraryQuery) Validate() error {
	if q.Limit < 1 || q.Limit > 100 {
		return apperrors.ErrInvalidInput
	}
	_, err := DecodeLibraryCursor(q.Cursor)
	return err
}

// DecodeLibraryCursor accepts a bounded canonical encoding and rejects unknown fields.
func DecodeLibraryCursor(value string) (*LibraryCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 256 {
		return nil, apperrors.ErrInvalidInput
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(data) != value {
		return nil, apperrors.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var cursor LibraryCursor
	if decoder.Decode(&cursor) != nil || cursor.ID <= 0 || cursor.At.IsZero() || cursor.At.Year() < 1 || cursor.At.Year() > 9999 {
		return nil, apperrors.ErrInvalidInput
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, apperrors.ErrInvalidInput
	}
	return &cursor, nil
}

func EncodeLibraryCursor(record Record) string {
	data, _ := json.Marshal(LibraryCursor{At: record.CreatedAt, ID: record.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}

// LibraryRecord is an authorized repository result before file URLs are signed.
type LibraryRecord struct {
	Recording  Record
	Conference conferences.Conference
}

type LibraryItem struct {
	Conference conferences.View `json:"conference"`
	Recording  RecordCard       `json:"recording"`
}

type LibraryPage struct {
	Items      []LibraryItem `json:"items"`
	NextCursor *string       `json:"nextCursor"`
}
