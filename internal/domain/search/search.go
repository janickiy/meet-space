// Package search defines vector representations independently of a particular model or SDK.
package search

import "context"

// EmbeddingRequest binds bounded text inputs to one embedding-model space.
type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Version        string   `json:"version"`
	Dimensions     int      `json:"dimensions"`
	Inputs         []string `json:"inputs"`
	IdempotencyKey string   `json:"-"`
}

// EmbeddingProvider isolates the external service from storage, authorization and ranking.
type EmbeddingProvider interface {
	// Embed computes a bounded batch of text vectors using the configured embedding model.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Validated application request or incoming operation parameters.
	//
	// Returns:
	//   - result 1: The collected []float32 values.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Embed(context.Context, EmbeddingRequest) ([][]float32, error)
}

// Chunk binds bounded text to a transcript generation, source segment and navigation position.
type Chunk struct {
	ID, ConferenceID, RecordingID, TranscriptID, SegmentID string
	Generation                                             int64
	Ordinal                                                int
	StartMS, EndMS                                         int64
	SpeakerID                                              *string
	Speaker, Text, ContentHash, ModelKey                   string
	Vector                                                 []float32
}
