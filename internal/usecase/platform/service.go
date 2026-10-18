// Платформа пакетов собирает данные о возможностях продукта, доступных только для чтения, и
// рабочие данные.
package platform

import (
	"context"
	"github.com/janickiy/meet-space/internal/buildinfo"
	"runtime/debug"

	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/platform"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// ReleaseVersion вводится во время сборки с помощью -ldflags и не содержит секретов.
var ReleaseVersion string

type Repository interface {
	// Summary возвращает ограниченные операционные агрегированные данные, доступные только
	// администратору, и безопасные коды ошибок без пользовательского контента.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Полученная сводная таблица.
	//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
	//   передачи данных или сохранения.
	Summary(context.Context) (domain.Summary, error)
}

type VectorAvailability interface {
	// Available проверяет, можно ли использовать возможности поддержки, необходимые для доступа
	// только для чтения к функциям продукта и его рабочему состоянию.
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
}

type ReadyProbe interface {
	// Ready сообщает, можно ли в данный момент использовать компонент или настроенную
	// зависимость.
	//
	// @Parameters:
	//   - arg1: Контекст, контролирующий отмену, сроки и продолжительность выполнения
	//   операции.
	//
	// @returns:
	//   - result1: Указывает, выполняется ли задокументированное условие операции и
	//   произошло ли успешное изменение ее условного состояния.
	Ready(context.Context) bool
}

// Service — сервис исключает учетные данные и внутренние адреса из общедоступных ответов.
type Service struct {
	Repo               Repository
	Vector             VectorAvailability
	MediaWorker        ReadyProbe
	APIReady           func() bool
	DependencyStatuses func() map[string]bool
	StageSeven         config.StageSevenConfig
	StageEight         config.StageEightConfig
}

// Capabilities возвращает возможности продукта или интеграции, фактически доступные оператору.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Результирующие возможности.
func (s *Service) Capabilities(ctx context.Context) domain.Capabilities {
	semantic := false
	if s.StageEight.EmbeddingsEnabled && s.Vector != nil {
		available, err := s.Vector.Available(ctx)
		semantic = err == nil && available
	}
	return domain.Capabilities{
		LiveCaptions:     s.StageEight.LiveEnabled,
		Transcription:    s.StageSeven.STTEnabled,
		AISummary:        s.StageSeven.AIEnabled,
		SemanticSearch:   semantic,
		MeetingAnalytics: s.StageEight.AnalyticsEnabled,
		RecordingModes:   []string{records.ModeComposite, records.ModeAudioOnly, records.ModeIndividualTracks, records.ModeScreenFocus},
	}
}

// Summary возвращает ограниченные операционные агрегированные данные, доступные только
// администратору, и безопасные коды ошибок без пользовательского контента.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Полученная сводная таблица.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (s *Service) Summary(ctx context.Context) (domain.Summary, error) {
	value, err := s.Repo.Summary(ctx)
	if err != nil {
		return domain.Summary{}, err
	}
	if s.APIReady != nil {
		value.APIReady = s.APIReady()
	}
	if s.DependencyStatuses != nil {
		value.Dependencies = s.DependencyStatuses()
	}
	if value.Dependencies == nil {
		value.Dependencies = map[string]bool{}
	}
	if s.MediaWorker != nil {
		value.MediaWorkerReady = s.MediaWorker.Ready(ctx)
	}
	return value, nil
}

// BuildVersion возвращает ограниченный идентификатор общедоступной сборки без указания путей и
// флагов сборки.
//
// @returns:
//   - result1: Ограниченный идентификатор общедоступной сборки.
func BuildVersion() string {
	if safeBuildVersion(ReleaseVersion) {
		return ReleaseVersion
	}
	if buildinfo.ValidVersion(buildinfo.Version) {
		return buildinfo.Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if safeBuildVersion(info.Main.Version) {
		return info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && len(setting.Value) >= 12 && safeBuildVersion(setting.Value[:12]) {
			return setting.Value[:12]
		}
	}
	return "dev"
}

// safeBuildVersion проверяет и ограничивает идентификатор общедоступной сборки.
//
// @Parameters:
//   - value: Значение для проверки, нормализации или преобразования.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func safeBuildVersion(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for index, char := range value {
		letter := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
		digit := char >= '0' && char <= '9'
		if letter || digit || index > 0 && (char == '.' || char == '_' || char == '-' || char == '+') {
			continue
		}
		return false
	}
	return true
}
