// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

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

// SearchRepository сохраняет отдельные векторы генерации и выполняет точный косинусный поиск
// после авторизации SQL.
type SearchRepository struct{ db *gorm.DB }

// NewSearchRepository создает поисковый репозиторий с зависимостями и ограничениями,
// необходимыми для авторизованной индексации и поиска по расшифровкам.
//
// @Parameters:
//   - db: Database дескриптор, используемый для операции или транзакции.
//
// @returns:
//   - result1: Созданный или загруженный поисковый репозиторий.
func NewSearchRepository(db *gorm.DB) *SearchRepository { return &SearchRepository{db} }

// Available проверяет, можно ли использовать возможности резервного копирования, необходимые
// для авторизованной индексации и поиска по расшифровкам.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *SearchRepository) Available(ctx context.Context) (bool, error) {
	var ok bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector') AND to_regclass('content_embeddings') IS NOT NULL`).Scan(&ok).Error
	return ok, err
}

// Source загружает сохраненные входные данные, необходимые для авторизованной индексации и
// поиска по расшифровкам.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: Результирующая расшифровка.
//   - result2: Собранные значения сегментов.
//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// Cached загружает повторно используемые векторы по хешу содержимого и полному идентификатору
// модели встраивания.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - model: Настроенный идентификатор модели провайдера.
//   - hashes: Дайджесты содержимого для поиска в специфичном для модели векторном кэше.
//
// @returns:
//   - result1: Результирующая карта[string][]float32.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// vectorLiteral сериализует конечный вектор в векторное представление PostgreSQL.
//
// @Parameters:
//   - vector: Валидированный вектор эмбеддинга.
//
// @returns:
//   - result1: Валидированный векторный литерал PostgreSQL.
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

// Save сохраняет результаты поиска при проверке текущего задания или идентификатора ресурса.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - chunks: Ограниченные фрагменты поиска по транскриптону.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// Fail регистрирует сбой безопасного поиска и запускает процесс очистки ресурсов, принадлежащих
// пользователю.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// hybridSQL материализует только текущие авторизованные векторы перед вычислением косинусного
// расстояния. Точный поиск сохраняет полноту отфильтрованных результатов; добавляйте ANN только
// после измерения целевого корпуса.
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

// Search — поиск выполняет авторизованную индексацию и поиск по транскриптону с использованием
// ограниченных входных данных и фильтров видимости вызывающей стороны.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - query: Текст для поиска или валидированные фильтры запросов.
//   - model: Настроенный идентификатор модели провайдера.
//   - vector: Валидированный вектор эмбеддинга.
//
// @returns:
//   - result1: Результирующая страница поиска.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// Reindex — переиндексация запускает ограниченную переиндексацию транскрипта после проверки прав доступа
// субъекта.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - cid: Идентификатор конференции, ограничивающий область действия операции.
//   - rid: Идентификатор записи, используемый текущей операцией.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - cooldown: Минимальный интервал между дорогостоящими запросами на повторную обработку.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
		// Ключ генерации и модели предотвращает повторную тарификацию за переиндексацию
		// неизменного текста.
		key := "content.reindex:" + t.ID + ":" + strconv.FormatInt(t.Generation, 10) + ":" + search.ModelKey(cfg)
		result := tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,dedup_key) VALUES('content.embed',?,?,?,?) ON CONFLICT(dedup_key) DO NOTHING`, t.ID, cid, t.Generation, key)
		if result.Error == nil && result.RowsAffected == 0 {
			return apperrors.ErrConflict
		}
		return result.Error
	})
}
