// Пакетная аналитика определяет агрегированные показатели технической конференции без оценки
// идентичности или продуктивности.
package analytics

import "time"

// Interval содержит данные о присутствии участников; перекрывающиеся подключения, принадлежащие
// одному участнику, объединяются.
type Interval struct {
	ParticipantID string
	Start, End    time.Time
}

// Point фиксирует количество одновременно присутствующих участников на временной шкале
// конференции.
type Point struct {
	AtMS  int64 `json:"atMs"`
	Count int   `json:"count"`
}

// Participant содержит объективные счетчики активности и приблизительные данные об
// аудиоактивности.
type Participant struct {
	ParticipantID   string `json:"participantId"`
	DisplayName     string `json:"displayName"`
	ParticipationMS int64  `json:"participationMs"`
	SpeakingMS      int64  `json:"speakingMs"`
	ObservedAudioMS int64  `json:"observedAudioMs"`
	ScreenMS        int64  `json:"screenMs"`
	MessageCount    int64  `json:"messageCount"`
}

// Conference содержит агрегированные показатели, которые явно помечают речевую активность как
// приблизительную.
type Conference struct {
	ConferenceID        string        `json:"conferenceId"`
	Enabled             bool          `json:"enabled"`
	DurationMS          int64         `json:"durationMs"`
	ParticipantCount    int           `json:"participantCount"`
	Timeline            []Point       `json:"timeline" gorm:"-"`
	Participants        []Participant `json:"participants" gorm:"-"`
	RecordingAvailable  bool          `json:"recordingAvailable"`
	TranscriptAvailable bool          `json:"transcriptAvailable"`
	ApproximateSpeaking bool          `json:"approximateSpeaking"`
	UpdatedAt           time.Time     `json:"updatedAt"`
}
