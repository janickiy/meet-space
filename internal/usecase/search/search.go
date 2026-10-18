// Поиск пакетов координирует индексирование и поиск с авторизацией на уровне репозитория.
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

// Repository — репозиторий принимает результаты только для текущей аренды и генерации и фильтрует доступ
// внутри SQL.
type Repository interface {
	// Available проверяет, можно ли использовать возможности резервного копирования,
	// необходимые для авторизованной индексации и поиска по расшифровкам.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Available(context.Context) (bool, error)
	// Source загружает сохраненные входные данные, необходимые для авторизованной индексации и
	// поиска по расшифровкам.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//
	// @returns:
	//   - result1: Результирующая расшифровка.
	//   - result2: Собранные значения сегментов.
	//   - result3: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Source(context.Context, jobs.Job) (content.Transcript, []content.Segment, error)
	// Cached загружает повторно используемые векторы по хешу содержимого и полному
	// идентификатору модели встраивания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Настроенный идентификатор модели провайдера.
	//   - arg3: Дайджесты содержимого для поиска в специфичном для модели векторном кэше.
	//
	// @returns:
	//   - result1: Результирующая карта[string][]float32.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Cached(context.Context, string, []string) (map[string][]float32, error)
	// Save сохраняет результаты поиска при проверке текущего задания или идентификатора
	// ресурса.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Ограничения конфигурации и обработки, контролируемые оператором.
	//   - arg4: Ограниченные фрагменты поиска по транскриптону.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Save(context.Context, jobs.Job, config.StageEightConfig, []domain.Chunk) error
	// Fail регистрирует сбой безопасного поиска и запускает процесс очистки ресурсов,
	// принадлежащих пользователю.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Зарезервированное фоновое задание и токен его генерации или владельца.
	//   - arg3: Ограничения конфигурации и обработки, контролируемые оператором.
	//   - arg4: Ограниченный публичный статус или код ошибки.
	//
	// @returns:
	//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Fail(context.Context, jobs.Job, config.StageEightConfig, string) error
	// Search — поиск выполняет авторизованную индексацию и поиск по транскриптону с использованием
	// ограниченных входных данных и фильтров видимости вызывающей стороны.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Идентификатор действующего аккаунта, используемый для ограничения
	//   доступа.
	//   - arg3: Текст для поиска или валидированные фильтры запросов.
	//   - arg4: Настроенный идентификатор модели провайдера.
	//   - arg5: Валидированный вектор эмбеддинга.
	//
	// @returns:
	//   - result1: Результирующая страница поиска.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Search(context.Context, string, content.SearchQuery, string, []float32) (content.SearchPage, error)
}

// Service — сервис отделяет задачи по созданию устойчивых индексов от задач по генерации векторов с
// ограниченным количеством запросов.
type Service struct {
	Repo       Repository
	Provider   domain.EmbeddingProvider
	Config     config.StageEightConfig
	querySlots chan struct{}
}

// New создает компонент с зависимостями и ограничениями обработки, необходимыми для
// авторизованной индексации и поиска по расшифровкам.
//
// @Parameters:
//   - repo: Зависимость персистентности, используемая операцией приложения.
//   - provider: Настроенный адаптер внешнего провайдера.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Созданный или загруженный сервис.
func New(repo Repository, provider domain.EmbeddingProvider, cfg config.StageEightConfig) *Service {
	return &Service{repo, provider, cfg, make(chan struct{}, cfg.EmbeddingWorkers)}
}

// ModelKey хеширует идентификаторы моделей встраивания, чтобы векторы из несовместимых моделей
// никогда не смешивались.
//
// @Parameters:
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Стабильный идентификатор SHA-256 для всего пространства моделей встраивания.
func ModelKey(cfg config.StageEightConfig) string {
	raw, _ := json.Marshal([]any{cfg.EmbeddingModel, cfg.EmbeddingVersion, cfg.EmbeddingDimensions})
	return Hash(string(raw))
}

// Hash — хеширует содержимое для обеспечения стабильной целостности или получения ключа кэширования.
//
// @Parameters:
//   - text: Текст UTF-8 для проверки, нормализации или обработки.
//
// @returns:
//   - result1: Защищенный хеш пароля или дайджест содержимого, согласно требованиям этого
//     компонента.
func Hash(text string) string { sum := sha256.Sum256([]byte(text)); return hex.EncodeToString(sum[:]) }

// Chunks — чанки детерминированно разбивают сегменты расшифровок на блоки для поиска с сохранением
// ссылок на говорящих и временные метки.
//
// @Parameters:
//   - transcript: Текущая генерация расшифровки и ее метаданные.
//   - segments: Упорядоченная расшифровка или сегменты медиаданных.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//
// @returns:
//   - result1: Собранные значения чанков.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// ValidVectors проверяет количество векторов, их размерность и наличие конечных ненулевых
// значений перед сохранением или поиском.
//
// @Parameters:
//   - vectors: Недоверенные векторы, возвращаемые провайдером встраивания.
//   - count: Ожидаемое или максимальное количество элементов.
//   - dimensions: Ожидаемое количество измерений вектора встраивания.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
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

// Handle — обработчик перенаправляет проверенную операцию поиска обработчику приложения.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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

// FailJob регистрирует сбой задания с ограничением, не раскрывая ответы провайдера и не заменяя
// более новую версию.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - job: Зарезервированное фоновое задание и токен его генерации или владельца.
//   - code: Ограниченный публичный статус или код ошибки.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) FailJob(ctx context.Context, job jobs.Job, code string) error {
	return s.Repo.Fail(ctx, job, s.Config, code)
}

// Search — поиск выполняет авторизованную индексацию и поиск по транскриптону с использованием
// ограниченных входных данных и фильтров видимости вызывающей стороны.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - user: Идентификатор действующего аккаунта, используемый для ограничения доступа.
//   - query: Текст для поиска или валидированные фильтры запросов.
//
// @returns:
//   - result1: Результирующая страница поиска.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
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
