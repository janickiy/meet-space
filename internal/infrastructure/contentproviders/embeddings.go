package contentproviders

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	domain "github.com/janickiy/meet-space/internal/domain/search"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// embeddings uses the shared protected HTTP client without redirects or response-body logging.
type embeddings struct{ *provider }

// NewEmbeddingProvider constructs the embedding provider with the dependencies and limits
// required for transcription, summary and embedding providers.
//
// Parameters:
//   - mode: Validated operation or recording mode.
//   - endpoint: Operator-configured service endpoint.
//   - token: Signed credential or operation-ownership token to verify.
//   - timeout: Maximum time allowed for this operation.
//
// Returns:
//   - result 1: The resulting embedding provider.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func NewEmbeddingProvider(mode, endpoint, token string, timeout time.Duration) (domain.EmbeddingProvider, error) {
	p, e := newProvider(mode, endpoint, token, "", timeout)
	return &embeddings{p}, e
}

// Embed computes a bounded batch of text vectors using the configured embedding model.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - request: Validated application request or incoming operation parameters.
//
// Returns:
//   - result 1: The collected []float32 values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (p *embeddings) Embed(ctx context.Context, request domain.EmbeddingRequest) ([][]float32, error) {
	if p.mode == "noop" {
		return nil, jobs.ErrSkip
	}
	if len(request.Inputs) < 1 || len(request.Inputs) > 64 || request.Dimensions < 8 || request.Dimensions > 2000 {
		return nil, jobs.Error{Code: "embedding_input_limit"}
	}
	if p.mode == "mock" {
		result := make([][]float32, len(request.Inputs))
		for i, text := range request.Inputs {
			result[i] = mockEmbedding(text, request.Dimensions)
		}
		return result, nil
	}
	raw, e := json.Marshal(request)
	if e != nil || len(raw) > 1<<20 {
		return nil, jobs.Error{Code: "embedding_input_limit"}
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if e != nil {
		return nil, jobs.Error{Code: "embedding_request"}
	}
	req.Header.Set("Content-Type", "application/json")
	data, e := p.call(req, request.IdempotencyKey, 2<<20)
	if e != nil {
		return nil, e
	}
	var result struct {
		Vectors [][]float32 `json:"vectors"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, jobs.Error{Code: "embedding_schema"}
	}
	return result.Vectors, nil
}

// mockEmbedding creates deterministic mock vectors for explicitly permitted local provider mode.
//
// Parameters:
//   - text: UTF-8 text to validate, normalize or process.
//   - dimensions: Expected embedding-vector dimensions.
//
// Returns:
//   - result 1: The collected float32 values.
func mockEmbedding(text string, dimensions int) []float32 {
	vector := make([]float32, dimensions)
	groups := map[string]string{"бюджет": "budget", "стоимость": "budget", "cost": "budget", "budget": "budget", "запуск": "launch", "релиз": "launch", "launch": "launch", "release": "launch", "встреча": "meeting", "совещание": "meeting", "meeting": "meeting"}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if normalized, ok := groups[word]; ok {
			word = normalized
		}
		hash := sha256.Sum256([]byte(word))
		index := int(binary.LittleEndian.Uint32(hash[:4]) % uint32(dimensions))
		vector[index]++
	}
	var norm float64
	for _, v := range vector {
		norm += float64(v * v)
	}
	if norm == 0 {
		vector[0] = 1
		return vector
	}
	norm = math.Sqrt(norm)
	for i := range vector {
		vector[i] /= float32(norm)
	}
	return vector
}
