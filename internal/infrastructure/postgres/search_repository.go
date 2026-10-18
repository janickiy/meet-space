package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	content "github.com/janickiy/meet-space/internal/domain/content"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	domain "github.com/janickiy/meet-space/internal/domain/search"
	search "github.com/janickiy/meet-space/internal/usecase/search"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SearchRepository stores separate vector generations and performs exact cosine search after SQL
// authorization.
type SearchRepository struct{ db *gorm.DB }

// NewSearchRepository constructs the search repository with the dependencies and limits required
// for authorized transcript indexing and search.
//
// Parameters:
//   - db: Database handle used for the operation or transaction.
//
// Returns:
//   - result 1: The constructed or loaded search repository.
func NewSearchRepository(db *gorm.DB) *SearchRepository { return &SearchRepository{db} }

// Available checks whether the backing capability needed for authorized transcript indexing and
// search is usable.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Available(ctx context.Context) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector') AND to_regclass('content_embeddings') IS NOT NULL`).Scan(&ok).Error
	return ok, err
}

// Source loads the persisted inputs needed to process authorized transcript indexing and search.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//
// Returns:
//   - result 1: The resulting transcript.
//   - result 2: The collected segment values.
//   - result 3: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Source(ctx context.Context, job jobs.Job) (content.Transcript, []content.Segment, error) {
	var t content.Transcript
	segments := []content.Segment{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := contentLease(tx, job); e != nil {
			return e
		}
		if e := tx.Table("transcripts").Where("id=? AND conference_id=? AND generation=? AND status='ready'", job.EntityID, job.ConferenceID, job.Version).Take(&t).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return jobs.ErrSkip
			}
			return e
		}
		if _, e := workerSource(tx, t.RecordingID, t.ConferenceID); e != nil {
			return e
		}
		return tx.Table("transcript_segments").Where("transcript_id=?", t.ID).Order("ordinal").Limit(10000).Find(&segments).Error
	})
	return t, segments, err
}

// Cached loads reusable vectors by content hash and the complete embedding-model identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - model: Configured provider model identifier.
//   - hashes: Content digests to look up in the model-specific vector cache.
//
// Returns:
//   - result 1: The resulting map[string][]float32.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Cached(ctx context.Context, model string, hashes []string) (map[string][]float32, error) {
	var rows []struct{ ContentHash, VectorText string }
	result := map[string][]float32{}
	err := r.db.WithContext(ctx).Raw(`SELECT content_hash,embedding::text AS vector_text FROM embedding_cache WHERE model_key=? AND content_hash IN ?`, model, hashes).Scan(&rows).Error
	for _, row := range rows {
		var vector []float32
		if json.Unmarshal([]byte(row.VectorText), &vector) == nil {
			result[row.ContentHash] = vector
		}
	}
	return result, err
}

// vectorLiteral serializes a finite vector to the PostgreSQL vector representation.
//
// Parameters:
//   - vector: Validated embedding vector.
//
// Returns:
//   - result 1: Validated PostgreSQL vector literal.
func vectorLiteral(vector []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vector {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// Save persists search results while checking the current job or resource identity.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//   - cfg: Operator-controlled configuration and processing limits.
//   - chunks: Bounded transcript search chunks.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Save(ctx context.Context, job jobs.Job, cfg config.StageEightConfig, chunks []domain.Chunk) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := contentLease(tx, job); e != nil {
			return e
		}
		var t content.Transcript
		if e := tx.Table("transcripts").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND generation=? AND status='ready'", job.EntityID, job.Version).Take(&t).Error; e != nil {
			return jobs.ErrLeaseLost
		}
		if _, e := workerSource(tx, t.RecordingID, job.ConferenceID); e != nil {
			return e
		}
		key := search.ModelKey(cfg)
		if e := tx.Exec(`DELETE FROM content_embeddings WHERE transcript_id=? AND model_key=?`, t.ID, key).Error; e != nil {
			return e
		}
		for _, chunk := range chunks {
			vector := vectorLiteral(chunk.Vector)
			if e := tx.Exec(`INSERT INTO embedding_cache(model_key,content_hash,embedding) VALUES(?,?,?::vector) ON CONFLICT DO NOTHING`, key, chunk.ContentHash, vector).Error; e != nil {
				return e
			}
			if e := tx.Exec(`INSERT INTO content_embeddings(id,transcript_id,conference_id,recording_id,generation,ordinal,segment_id,start_ms,end_ms,speaker_id,speaker,text,model_key,content_hash,embedding)
    VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?::vector)`, chunk.ID, t.ID, t.ConferenceID, t.RecordingID, t.Generation, chunk.Ordinal, chunk.SegmentID, chunk.StartMS, chunk.EndMS, chunk.SpeakerID, chunk.Speaker, chunk.Text, key, chunk.ContentHash, vector).Error; e != nil {
				return e
			}
		}
		return tx.Exec(`INSERT INTO content_search_indexes(transcript_id,model_key,generation,state,model,model_version,dimensions)
   VALUES(?,?,?,'ready',?,?,?) ON CONFLICT(transcript_id,model_key) DO UPDATE SET generation=EXCLUDED.generation,state='ready',error_code=NULL,updated_at=clock_timestamp()`, t.ID, key, t.Generation, cfg.EmbeddingModel, cfg.EmbeddingVersion, cfg.EmbeddingDimensions).Error
	})
}

// Fail records a safe search failure and starts the owned-resource cleanup path.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - job: Leased background job and its generation or ownership token.
//   - cfg: Operator-controlled configuration and processing limits.
//   - code: Bounded public status or failure code.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Fail(ctx context.Context, job jobs.Job, cfg config.StageEightConfig, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := contentLease(tx, job); e != nil {
			return e
		}
		return tx.Exec(`INSERT INTO content_search_indexes(transcript_id,model_key,generation,state,model,model_version,dimensions,error_code)
 SELECT id,?,generation,'failed',?,?,?,? FROM transcripts WHERE id=? AND generation=?
 ON CONFLICT(transcript_id,model_key) DO UPDATE SET state='failed',error_code=EXCLUDED.error_code,updated_at=clock_timestamp() WHERE content_search_indexes.generation=EXCLUDED.generation`, search.ModelKey(cfg), cfg.EmbeddingModel, cfg.EmbeddingVersion, cfg.EmbeddingDimensions, code, job.EntityID, job.Version).Error
	})
}

// hybridSQL materializes only current authorized vectors before cosine-distance evaluation.
// Exact search preserves filtered-result completeness; add ANN only after measuring the target
// corpus.
const hybridSQL = `, permitted_vectors AS MATERIALIZED (
 SELECT e.*,c.title FROM content_embeddings e JOIN permitted c ON c.id=e.conference_id
 JOIN transcripts t ON t.id=e.transcript_id AND t.generation=e.generation AND t.status='ready'
 JOIN record r ON r.uuid=e.recording_id AND r.status='ready' AND r.deleted_at IS NULL
 WHERE e.model_key=? AND (?='' OR e.speaker_id::text=?) AND ? IN ('all','transcript')
 ), distances AS MATERIALIZED (
 SELECT e.*,GREATEST(0,1-(e.embedding<=>?::vector)) AS semantic,ts_rank(e.search_vector,q.query) AS lexical
 FROM permitted_vectors e CROSS JOIN searchquery q
 ), semantic_candidates AS (
 SELECT 'transcript'::text AS type,conference_id,title AS conference_title,recording_id,transcript_id,segment_id,start_ms,
 left(text,300) AS snippet,speaker_id,speaker,
 CASE WHEN ?='semantic' THEN semantic ELSE 0.4*(lexical/(0.1+lexical))+0.6*semantic END AS rank
 FROM distances WHERE semantic>=0.15 OR (?='hybrid' AND lexical>0)
 ), lexical_candidates AS (
 SELECT m.type,m.conference_id,m.conference_title,m.recording_id,m.transcript_id,m.segment_id,m.start_ms,m.snippet,
 m.speaker_id,m.speaker,0.4*(m.rank/(0.1+m.rank))+0.6*COALESCE(scores.semantic,0) AS rank
 FROM matches m LEFT JOIN (SELECT segment_id,max(semantic) AS semantic FROM distances GROUP BY segment_id) scores ON scores.segment_id=m.segment_id WHERE ?='hybrid'
 ), combined AS (SELECT * FROM semantic_candidates UNION ALL SELECT * FROM lexical_candidates),
 ranked AS (SELECT DISTINCT ON(type,conference_id,recording_id,segment_id) * FROM combined ORDER BY type,conference_id,recording_id,segment_id,rank DESC) `

// Search searches authorized transcript indexing and search using bounded input and the caller's
// visibility filters.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - query: Search text or validated query filters.
//   - model: Configured provider model identifier.
//   - vector: Validated embedding vector.
//
// Returns:
//   - result 1: The resulting search page.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Search(ctx context.Context, user string, query content.SearchQuery, model string, vector []float32) (content.SearchPage, error) {
	if query.Mode == "keyword" || len(vector) == 0 {
		return NewContentRepository(r.db).Search(ctx, user, query)
	}
	page := content.SearchPage{Items: []content.SearchResult{}, Limit: query.Limit, Offset: query.Offset, EffectiveMode: query.Mode}
	args := contentSearchArgs(user, query)
	args = append(args, model, query.ParticipantID, query.ParticipantID, query.Source, vectorLiteral(vector), query.Mode, query.Mode, query.Mode)
	sql := contentSearchSQL + hybridSQL
	if err := r.db.WithContext(ctx).Raw(sql+"SELECT count(*) FROM ranked", args...).Scan(&page.Total).Error; err != nil {
		return page, err
	}
	args = append(args, query.Limit, query.Offset)
	err := r.db.WithContext(ctx).Raw(sql+"SELECT * FROM ranked ORDER BY rank DESC,conference_id,type,segment_id NULLS FIRST LIMIT ? OFFSET ?", args...).Scan(&page.Items).Error
	return page, err
}

// Reindex queues a bounded transcript reindex after checking the actor's management permission.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - user: Acting account identifier used to scope access.
//   - cid: Conference identifier limiting the operation scope.
//   - rid: Recording identifier used by the current operation.
//   - cfg: Operator-controlled configuration and processing limits.
//   - cooldown: Minimum interval between expensive retriable processing requests.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (r *SearchRepository) Reindex(ctx context.Context, user, cid, rid string, cfg config.StageEightConfig, cooldown time.Duration) error {
	if !cfg.EmbeddingsEnabled {
		return apperrors.ErrUnavailable
	}
	if available, err := r.Available(ctx); err != nil || !available {
		return apperrors.ErrUnavailable
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := findConference(tx, cid, true); e != nil {
			return e
		}
		_, manage, e := contentAccess(tx, user, cid, rid)
		if e != nil {
			return e
		}
		if !manage {
			return apperrors.ErrForbidden
		}
		var t content.Transcript
		if e = tx.Table("transcripts").Where("recording_id=? AND status='ready'", rid).Take(&t).Error; e != nil {
			return apperrors.ErrConflict
		}
		var recent int64
		if e = tx.Table("background_jobs").Where("entity_id=? AND kind='content.embed' AND created_at>clock_timestamp()-?::interval", t.ID, fmt.Sprintf("%f seconds", cooldown.Seconds())).Count(&recent).Error; e != nil {
			return e
		}
		if recent > 0 {
			return apperrors.ErrConflict
		}
		// A generation-and-model key prevents repeatedly charging for reindexing unchanged text.
		key := "content.reindex:" + t.ID + ":" + strconv.FormatInt(t.Generation, 10) + ":" + search.ModelKey(cfg)
		result := tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,dedup_key) VALUES('content.embed',?,?,?,?) ON CONFLICT(dedup_key) DO NOTHING`, t.ID, cid, t.Generation, key)
		if result.Error == nil && result.RowsAffected == 0 {
			return apperrors.ErrConflict
		}
		return result.Error
	})
}
