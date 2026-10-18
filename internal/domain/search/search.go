// Поиск пакетов определяет векторные представления независимо от конкретной модели или SDK.
package search

import "context"

// EmbeddingRequest привязывает ограниченные текстовые входные данные к одному пространству
// модели внедрения.
type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Version        string   `json:"version"`
	Dimensions     int      `json:"dimensions"`
	Inputs         []string `json:"inputs"`
	IdempotencyKey string   `json:"-"`
}

// EmbeddingProvider изолирует внешний сервис от хранения данных, авторизации и ранжирования.
type EmbeddingProvider interface {
	// Embed вычисляет ограниченный пакет текстовых векторов с помощью настроенной модели
	// встраивания.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//   - arg2: Валидированный запрос приложения или входящие параметры операции.
	//
	// @returns:
	//   - result1: Собранные значения []float32.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Embed(context.Context, EmbeddingRequest) ([][]float32, error)
}

// Chunk привязывает ограниченный текст к транскрипции, исходному сегменту и позиции навигации.
type Chunk struct {
	ID, ConferenceID, RecordingID, TranscriptID, SegmentID string
	Generation                                             int64
	Ordinal                                                int
	StartMS, EndMS                                         int64
	SpeakerID                                              *string
	Speaker, Text, ContentHash, ModelKey                   string
	Vector                                                 []float32
}
