// Package platform collects read-only product capabilities and operational data.
package platform

import (
	"context"
	"github.com/janickiy/meet-space/internal/buildinfo"
	"runtime/debug"

	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/platform"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// ReleaseVersion is injected at build time with -ldflags and contains no secrets.
var ReleaseVersion string

type Repository interface {
	// Summary returns bounded administrator-only operational aggregates and safe failure codes
	// without user content.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The resulting summary.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Summary(context.Context) (domain.Summary, error)
}

type VectorAvailability interface {
	// Available checks whether the backing capability needed for read-only product capabilities and
	// operational status is usable.
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
}

type ReadyProbe interface {
	// Ready reports whether the component or configured dependency is currently usable.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: Whether the operation's documented condition holds or its conditional state
	//     change succeeded.
	Ready(context.Context) bool
}

// Service excludes credentials and internal addresses from public responses.
type Service struct {
	Repo               Repository
	Vector             VectorAvailability
	MediaWorker        ReadyProbe
	APIReady           func() bool
	DependencyStatuses func() map[string]bool
	StageSeven         config.StageSevenConfig
	StageEight         config.StageEightConfig
}

// Capabilities returns the product or integration capabilities actually enabled by the operator.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The resulting capabilities.
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

// Summary returns bounded administrator-only operational aggregates and safe failure codes
// without user content.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The resulting summary.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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

// BuildVersion returns the bounded public release identifier without paths or build flags.
//
// Returns:
//   - result 1: Bounded public build identifier.
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

// safeBuildVersion validates and bounds the public build identifier.
//
// Parameters:
//   - value: Value to validate, normalize or convert.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
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
