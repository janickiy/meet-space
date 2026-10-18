// Package search coordinates bounded indexing and search with repository-level authorization.
package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	content "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	domain "github.com/janickiy/meet-space/internal/domain/search"
	"github.com/janickiy/meet-space/internal/operations"
	"time"
)

// Repository accepts results only for the current lease and generation and filters access inside
// SQL.
type Repository interface {
	// Available checks whether the backing capability needed for authorized transcript indexing and
	// search is usable.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Available(context.Context) (bool, error)
	// Source loads the persisted inputs needed to process authorized transcript indexing and search.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//
	// Returns:
	//   - result 1: The resulting transcript.
	//   - result 2: The collected segment values.
	//   - result 3: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Source(context.Context, jobs.Job) (content.Transcript, []content.Segment, error)
	// Cached loads reusable vectors by content hash and the complete embedding-model identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Configured provider model identifier.
	//   - argument 3: Content digests to look up in the model-specific vector cache.
	//
	// Returns:
	//   - result 1: The resulting map[string][]float32.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Cached(context.Context, string, []string) (map[string][]float32, error)
	// Save persists search results while checking the current job or resource identity.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Operator-controlled configuration and processing limits.
	//   - argument 4: Bounded transcript search chunks.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Save(context.Context, jobs.Job, config.StageEightConfig, []domain.Chunk) error
	// Fail records a safe search failure and starts the owned-resource cleanup path.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Leased background job and its generation or ownership token.
	//   - argument 3: Operator-controlled configuration and processing limits.
	//   - argument 4: Bounded public status or failure code.
	//
	// Returns:
	//   - result 1: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Fail(context.Context, jobs.Job, config.StageEightConfig, string) error
	// Search searches authorized transcript indexing and search using bounded input and the caller's
	// visibility filters.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Acting account identifier used to scope access.
	//   - argument 3: Search text or validated query filters.
	//   - argument 4: Configured provider model identifier.
	//   - argument 5: Validated embedding vector.
	//
	// Returns:
	//   - result 1: The resulting search page.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Search(context.Context, string, content.SearchQuery, string, []float32) (content.SearchPage, error)
}

// Service separates durable indexing jobs from bounded query-vector generation.
type Service struct {
	Repo       Repository
	Provider   domain.EmbeddingProvider
	Config     config.StageEightConfig
	querySlots chan struct{}
}

// New constructs the component with the dependencies and processing limits required for
// authorized transcript indexing and search.
//
// Parameters:
//   - repo: Persistence dependency used by the application operation.
//   - provider: Configured external-provider adapter.
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The constructed or loaded service.
func New(repo Repository, provider domain.EmbeddingProvider, cfg config.StageEightConfig) *Service {
	return &Service{repo, provider, cfg, make(chan struct{}, cfg.EmbeddingWorkers)}
}

// ModelKey hashes the embedding model identity so vectors from incompatible models are never
// mixed.
//
// Parameters:
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: Stable SHA-256 identity of the complete embedding-model space.
func ModelKey(cfg config.StageEightConfig) string {
	raw, _ := json.Marshal([]any{cfg.EmbeddingModel, cfg.EmbeddingVersion, cfg.EmbeddingDimensions})
	return Hash(string(raw))
}

// Hash hashes the content to obtain a stable integrity or cache key.
//
// Parameters:
//   - text: UTF-8 text to validate, normalize or process.
//
// Returns:
//   - result 1: Protected password hash or content digest, according to this component.
func Hash(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }

// Chunks deterministically splits transcript segments into bounded search chunks while
// preserving speaker and time references.
//
// Parameters:
//   - transcript: Current transcript generation and its metadata.
//   - segments: Ordered transcript or media segments.
//   - cfg: Operator-controlled configuration and processing limits.
//
// Returns:
//   - result 1: The collected chunk values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func Chunks(transcript content.Transcript, segments []content.Segment, cfg config.StageEightConfig) ([]domain.Chunk, error) {
	result := []domain.Chunk{}
	model := ModelKey(cfg)
	for _, segment := range segments {
		runes := []rune(segment.Text)
		for offset := 0; offset < len(runes); offset += cfg.EmbeddingChunkRunes {
			text := strings.TrimSpace(string(runes[offset:min(len(runes), offset+cfg.EmbeddingChunkRunes)]))
			if text == "" {
				continue
			}
			if len(result) >= 20000 {
				return nil, jobs.Error{Code: "embedding_chunk_limit"}
			}
			speaker := ""
			if segment.SpeakerLabel != nil {
				speaker = *segment.SpeakerLabel
			}
			result = append(result, domain.Chunk{ID: uuid.NewSHA1(uuid.MustParse(transcript.ID), []byte(fmt.Sprintf("%d:%s:%d", transcript.Generation, model, len(result)))).String(), ConferenceID: transcript.ConferenceID, RecordingID: transcript.RecordingID, TranscriptID: transcript.ID, SegmentID: segment.ID, Generation: transcript.Generation, Ordinal: len(result), StartMS: segment.StartMS, EndMS: segment.EndMS, SpeakerID: segment.SpeakerID, Speaker: speaker, Text: text, ContentHash: Hash(text), ModelKey: model})
		}
	}
	return result, nil
}

// ValidVectors checks vector count, dimensions and finite nonzero values before storage or
// search.
//
// Parameters:
//   - vectors: Untrusted vectors returned by the embedding provider.
//   - count: Expected or maximum number of items.
//   - dimensions: Expected embedding-vector dimensions.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func ValidVectors(vectors [][]float32, count, dimensions int) bool {
	if len(vectors) != count {
		return false
	}
	for _, vector := range vectors {
		if len(vector) != dimensions {
			return false
		}
		var norm float64
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return false
			}
			norm += float64(value) * float64(value)
		}
		if norm <= 0 {
			return false
		}
	}
	return true
}

// Handle dispatches the validated search operation to its application handler.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	started := time.Now()
	defer func() { operations.Observe("embedding", time.Since(started).Seconds()) }()
	if !s.Config.EmbeddingsEnabled {
		return jobs.ErrSkip
	}
	available, err := s.Repo.Available(ctx)
	if err != nil {
		return err
	}
	if !available {
		return jobs.ErrSkip
	}
	transcript, segments, err := s.Repo.Source(ctx, job)
	if err != nil {
		return err
	}
	chunks, err := Chunks(transcript, segments, s.Config)
	if err != nil {
		return err
	}
	for offset := 0; offset < len(chunks); offset += s.Config.EmbeddingBatch {
		batch := chunks[offset:min(len(chunks), offset+s.Config.EmbeddingBatch)]
		hashes := make([]string, len(batch))
		for i := range batch {
			hashes[i] = batch[i].ContentHash
		}
		cache, err := s.Repo.Cached(ctx, ModelKey(s.Config), hashes)
		if err != nil {
			return err
		}
		inputs := []string{}
		positions := []int{}
		for i := range batch {
			if vector, ok := cache[batch[i].ContentHash]; ok && ValidVectors([][]float32{vector}, 1, s.Config.EmbeddingDimensions) {
				batch[i].Vector = vector
			} else {
				inputs = append(inputs, batch[i].Text)
				positions = append(positions, i)
			}
		}
		if len(inputs) == 0 {
			continue
		}
		started := time.Now()
		vectors, err := s.Provider.Embed(ctx, domain.EmbeddingRequest{Model: s.Config.EmbeddingModel, Version: s.Config.EmbeddingVersion, Dimensions: s.Config.EmbeddingDimensions, Inputs: inputs, IdempotencyKey: Hash(ModelKey(s.Config) + strings.Join(hashes, ":"))})
		operations.ProviderCall("embedding", time.Since(started), err)
		if err != nil {
			return err
		}
		if !ValidVectors(vectors, len(inputs), s.Config.EmbeddingDimensions) {
			return jobs.Error{Code: "embedding_schema"}
		}
		for i, index := range positions {
			batch[index].Vector = vectors[i]
		}
	}
	return s.Repo.Save(ctx, job, s.Config, chunks)
}

// FailJob records a bounded job failure without exposing provider responses or replacing a newer
// generation.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	return s.Repo.Fail(ctx, job, s.Config, code)
}

// Search searches authorized transcript indexing and search using bounded input and the caller's
// visibility filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - query: Search text or validated query filters.
//
// Returns:
//   - result 1: The resulting search page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) Search(ctx context.Context, user string, query content.SearchQuery) (content.SearchPage, error) {
	started := time.Now()
	defer func() { operations.Observe("hybrid_search", time.Since(started).Seconds()) }()
	fallback := func(reason string) (content.SearchPage, error) {
		query.Mode = "keyword"
		page, e := s.Repo.Search(ctx, user, query, ModelKey(s.Config), nil)
		page.EffectiveMode = "keyword"
		page.FallbackReason = reason
		return page, e
	}
	if query.Mode == "" || query.Mode == "keyword" {
		return fallback("")
	}
	if query.Mode == "semantic" && (query.Source == "conference" || query.Source == "summary") {
		return fallback("source_keyword_only")
	}
	if !s.Config.EmbeddingsEnabled {
		return fallback("semantic_disabled")
	}
	available, err := s.Repo.Available(ctx)
	if err != nil || !available {
		return fallback("vector_unavailable")
	}
	select {
	case s.querySlots <- struct{}{}:
		defer func() { <-s.querySlots }()
	default:
		return fallback("provider_busy")
	}
	op, cancel := context.WithTimeout(ctx, min(s.Config.EmbeddingTimeout, 3*time.Second))
	defer cancel()
	start := time.Now()
	vectors, err := s.Provider.Embed(op, domain.EmbeddingRequest{Model: s.Config.EmbeddingModel, Version: s.Config.EmbeddingVersion, Dimensions: s.Config.EmbeddingDimensions, Inputs: []string{query.Query}, IdempotencyKey: "query:" + Hash(ModelKey(s.Config)+query.Query)})
	operations.ProviderCall("embedding", time.Since(start), err)
	if err != nil || !ValidVectors(vectors, 1, s.Config.EmbeddingDimensions) {
		return fallback("provider_unavailable")
	}
	page, err := s.Repo.Search(ctx, user, query, ModelKey(s.Config), vectors[0])
	if err != nil {
		return fallback("vector_unavailable")
	}
	page.EffectiveMode = query.Mode
	return page, nil
}
