package content

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"io"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// ChunkSegments splits validated transcript segments into bounded summary inputs while
// preserving their order.
//
// Parameters:
//   - segments: Ordered transcript or media segments.
//   - maximum: Maximum permitted size or number of characters.
//   - maxChunks: Maximum number of processing chunks.
//
// Returns:
//   - result 1: The collected segment values.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func ChunkSegments(segments []domain.Segment, maximum, maxChunks int) ([][]domain.Segment, error) {
	if maximum < 1 || maxChunks < 1 {
		return nil, fmt.Errorf("invalid chunk budget")
	}
	chunks := [][]domain.Segment{}
	chunk := []domain.Segment{}
	used := 0
	flush := func() {
		if len(chunk) > 0 {
			chunks = append(chunks, chunk)
			chunk = []domain.Segment{}
			used = 0
		}
	}
	for _, segment := range segments {
		if segment.ID == "" || !utf8.ValidString(segment.Text) {
			return nil, fmt.Errorf("invalid source segment")
		}
		runes := []rune(segment.Text)
		for len(runes) > 0 {
			if used == maximum {
				flush()
			}
			n := min(len(runes), maximum-used)
			part := segment
			part.Text = string(runes[:n])
			chunk = append(chunk, part)
			used += n
			runes = runes[n:]
			if len(chunks) >= maxChunks {
				return nil, fmt.Errorf("too many chunks")
			}
		}
	}
	flush()
	if len(chunks) == 0 || len(chunks) > maxChunks {
		return nil, fmt.Errorf("empty or excessive chunks")
	}
	return chunks, nil
}

// ValidateSummary checks the summary schema and verifies references against the source
// transcript.
//
// Parameters:
//   - data: Input bytes or structured event payload.
//   - source: Media or content source belonging to the current operation.
//
// Returns:
//   - result 1: The resulting summary output.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func ValidateSummary(data json.RawMessage, source []domain.Segment) (domain.SummaryOutput, error) {
	var out domain.SummaryOutput
	if len(data) > 256<<10 {
		return out, fmt.Errorf("summary too large")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 4 {
		return out, fmt.Errorf("invalid summary object")
	}
	for _, key := range []string{"summary", "keyPoints", "actionItems", "topics"} {
		if _, ok := fields[key]; !ok {
			return out, fmt.Errorf("missing summary field")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return out, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return out, fmt.Errorf("trailing summary data")
	}
	if out.KeyPoints == nil || out.ActionItems == nil || out.Topics == nil || !validText(out.Summary, 20000, true) || len(out.KeyPoints) > 100 || len(out.Topics) > 100 || len(out.ActionItems) > 200 {
		return out, fmt.Errorf("invalid summary bounds")
	}
	for _, text := range out.KeyPoints {
		if !validText(text, 2000, false) {
			return out, fmt.Errorf("invalid key point")
		}
	}
	for _, text := range out.Topics {
		if !validText(text, 500, false) {
			return out, fmt.Errorf("invalid topic")
		}
	}
	allowed := map[string]string{}
	for _, segment := range source {
		allowed[segment.ID] += "\n" + segment.Text
	}
	for i := range out.ActionItems {
		a := &out.ActionItems[i]
		if !validText(a.Text, 2000, false) || len(a.SourceSegmentIDs) < 1 || len(a.SourceSegmentIDs) > 100 {
			return out, fmt.Errorf("action must cite source segments")
		}
		var evidence strings.Builder
		seen := map[string]bool{}
		for _, id := range a.SourceSegmentIDs {
			value, ok := allowed[id]
			if !ok || seen[id] {
				return out, fmt.Errorf("invalid action source")
			}
			seen[id] = true
			evidence.WriteString(value)
		}
		if a.Assignee != nil && (!validText(*a.Assignee, 200, false) || !explicitAssignee(evidence.String(), *a.Assignee)) {
			a.Assignee = nil
		}
		if a.DueDate != nil {
			d, err := time.Parse("2006-01-02", *a.DueDate)
			if err != nil || d.Format("2006-01-02") != *a.DueDate || !explicitDeadline(evidence.String(), *a.DueDate) {
				a.DueDate = nil
			}
		}
	}
	return out, nil
}

// explicitAssignee accepts an action-item assignee only when it is explicitly supported by the
// source text.
//
// Parameters:
//   - evidence: Source text that must explicitly support the extracted claim.
//   - name: Name of the setting, cookie, resource or provider being addressed.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func explicitAssignee(evidence, name string) bool {
	text := strings.ToLower(evidence)
	name = strings.ToLower(strings.TrimSpace(name))
	for _, marker := range []string{"ответственный:", "исполнитель:", "assignee:"} {
		if strings.Contains(text, marker+" "+name) || strings.Contains(text, marker+name) {
			return true
		}
	}
	return false
}

// explicitDeadline accepts an action-item deadline only when it is explicitly supported by the
// source text.
//
// Parameters:
//   - evidence: Source text that must explicitly support the extracted claim.
//   - date: Proposed deadline text to verify against the source evidence.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func explicitDeadline(evidence, date string) bool {
	text := strings.ToLower(evidence)
	for _, marker := range []string{"срок:", "deadline:", "due:"} {
		if strings.Contains(text, marker+" "+date) || strings.Contains(text, marker+date) {
			return true
		}
	}
	return strings.Contains(text, "до "+date)
}

// validText checks that text is valid UTF-8 and remains within its allowed budget.
//
// Parameters:
//   - text: UTF-8 text to validate, normalize or process.
//   - maximum: Maximum permitted size or number of characters.
//   - empty: Whether an empty text value is permitted.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func validText(text string, maximum int, empty bool) bool {
	return utf8.ValidString(text) && !strings.ContainsRune(text, 0) && utf8.RuneCountInString(text) <= maximum && (empty || strings.TrimSpace(text) != "")
}

// summarize processes bounded transcript chunks and hierarchical merges while separating model
// instructions from source data.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - segments: Ordered transcript or media segments.
//
// Returns:
//   - result 1: The resulting summary output.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) summarize(ctx context.Context, key string, segments []domain.Segment) (domain.SummaryOutput, error) {
	chunks, err := ChunkSegments(segments, s.cfg.ChunkRunes, s.cfg.MaxChunks)
	if err != nil {
		return domain.SummaryOutput{}, &jobs.Error{Code: "ai_input_limit", Retryable: false}
	}
	results := make([]domain.SummaryOutput, len(chunks))
	errs := make([]error, len(chunks))
	semaphore := make(chan struct{}, s.cfg.AIConcurrency)
	var wg sync.WaitGroup
	schedulingErr := false
schedule:
	for i, chunk := range chunks {
		i, chunk := i, chunk
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			schedulingErr = true
			break schedule
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-semaphore }()
			input, _ := json.Marshal(map[string]any{"segments": chunk})
			results[i], errs[i] = s.aiCall(ctx, fmt.Sprintf("%s:chunk:%d", key, i), string(input), false, chunk)
		}()
	}
	wg.Wait()
	if schedulingErr {
		return domain.SummaryOutput{}, &jobs.Error{Code: "ai_timeout", Retryable: true}
	}
	for _, e := range errs {
		if e != nil {
			return domain.SummaryOutput{}, e
		}
	}
	if len(results) == 1 {
		return results[0], nil
	}
	// Merge groups use the same input budget. If the provider expands text excessively, stop rather
	// than issuing unbounded paid requests.
	for level := 0; len(results) > 1; level++ {
		if level > 16 {
			return domain.SummaryOutput{}, &jobs.Error{Code: "ai_merge_limit", Retryable: false}
		}
		groups := [][]domain.SummaryOutput{}
		group := []domain.SummaryOutput{}
		size := 0
		for _, result := range results {
			encoded, _ := json.Marshal(result)
			n := utf8.RuneCount(encoded)
			if n > s.cfg.ChunkRunes {
				return domain.SummaryOutput{}, &jobs.Error{Code: "ai_merge_limit", Retryable: false}
			}
			if size+n > s.cfg.ChunkRunes && len(group) > 0 {
				groups = append(groups, group)
				group = []domain.SummaryOutput{}
				size = 0
			}
			group = append(group, result)
			size += n
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
		if len(groups) >= len(results) {
			return domain.SummaryOutput{}, &jobs.Error{Code: "ai_merge_limit", Retryable: false}
		}
		next := make([]domain.SummaryOutput, len(groups))
		for i, g := range groups {
			input, _ := json.Marshal(map[string]any{"partials": g})
			next[i], err = s.aiCall(ctx, fmt.Sprintf("%s:merge:%d:%d", key, level, i), string(input), true, segments)
			if err != nil {
				return domain.SummaryOutput{}, err
			}
		}
		results = next
	}
	return results[0], nil
}

// aiCall sends untrusted JSON separately from the fixed model instructions and strictly
// validates the response schema.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - key: Storage, cache or cryptographic key used by the operation.
//   - input: Input data required by this operation.
//   - merge: Whether the request merges summaries rather than summarizes source segments.
//   - source: Media or content source belonging to the current operation.
//
// Returns:
//   - result 1: The resulting summary output.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (s *Service) aiCall(ctx context.Context, key, input string, merge bool, source []domain.Segment) (domain.SummaryOutput, error) {
	data, err := s.ai.Summarize(ctx, domain.AIRequest{PromptVersion: PromptVersion, SchemaVersion: SchemaVersion, Instructions: SummaryInstructions, InputJSON: input, IdempotencyKey: key, Merge: merge})
	if err != nil {
		return domain.SummaryOutput{}, providerError(err, "ai_unavailable")
	}
	out, err := ValidateSummary(data, source)
	if err != nil {
		return domain.SummaryOutput{}, &jobs.Error{Code: "invalid_summary", Retryable: false}
	}
	return out, nil
}
