// Package errorlogs contains the bounded, privacy-safe administrative error journal.
package errorlogs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const RetentionDays = 14
const MaxRecords = 20000
const MaxBatchSize = 256

type Frame struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}
type Event struct {
	ID          string    `json:"id" gorm:"type:uuid;primaryKey"`
	Fingerprint string    `json:"-"`
	Service     string    `json:"service"`
	Instance    string    `json:"instance"`
	Level       string    `json:"level"`
	Code        string    `json:"code"`
	Message     string    `json:"message"`
	Route       string    `json:"route"`
	RequestID   string    `json:"requestId" gorm:"column:request_id"`
	Release     string    `json:"release"`
	FirstSeen   time.Time `json:"firstSeen"`
	LastSeen    time.Time `json:"lastSeen"`
	Count       int64     `json:"count" gorm:"column:occurrences"`
	Browser     string    `json:"browser,omitempty"`
	Stack       []Frame   `json:"stack,omitempty" gorm:"-"`
}

func (Event) TableName() string { return "error_events" }

type Batch struct {
	ID     string
	Events []Event
}
type Query struct {
	Service, Level, Code, RequestID, From, To, Before string
	PageSize                                          int
}
type Summary struct {
	LastHour int64 `json:"lastHour"`
	LastDay  int64 `json:"lastDay"`
	Total    int64 `json:"total"`
}
type Page struct {
	Items         []Event `json:"items"`
	NextCursor    string  `json:"nextCursor"`
	Summary       Summary `json:"summary"`
	RetentionDays int     `json:"retentionDays"`
	MaxRecords    int     `json:"maxRecords"`
}
type Cursor struct {
	LastSeen time.Time `json:"lastSeen"`
	ID       string    `json:"id"`
}

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,120}$`)
var goCodePattern = regexp.MustCompile(`^go\.error\.[a-f0-9]{12}$`)
var codePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`)
var routePattern = regexp.MustCompile(`^/[A-Za-z0-9_/:.-]{0,199}$`)
var assetPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+-[A-Za-z0-9_-]{8,}\.js$`)
var messages = map[string]string{
	"component.error":            "Ошибка компонента",
	"server.panic":               "Необработанная ошибка сервера",
	"http.5xx":                   "Ошибка обработки HTTP-запроса",
	"dependency.failed":          "Ошибка зависимости сервиса",
	"worker.failed":              "Ошибка фоновой задачи",
	"storage.failed":             "Ошибка файлового хранилища",
	"media.failed":               "Ошибка медиасервиса",
	"recording.failed":           "Ошибка обработки записи",
	"smtp.failed":                "Ошибка доставки почты",
	"client.uncaught_error":      "Необработанная ошибка браузера",
	"client.unhandled_rejection": "Необработанная ошибка асинхронной операции браузера",
	"client.react_render_error":  "Ошибка отображения интерфейса",
}

func ValidService(s string) bool {
	switch s {
	case "api", "media-worker", "record-worker", "product-worker", "frontend", "admin-frontend":
		return true
	}
	return false
}
func SafeRoute(route string) string {
	if route == "unknown" {
		return route
	}
	if !routePattern.MatchString(route) || strings.Contains(route, "..") || strings.Contains(route, "//") {
		return "unknown"
	}
	// Concrete account/resource identifiers never belong in the journal.
	for _, p := range strings.Split(route, "/") {
		if _, e := uuid.Parse(p); e == nil {
			return "unknown"
		}
	}
	return route
}
func Sanitize(event Event) Event {
	if id, e := uuid.Parse(event.ID); e != nil {
		event.ID = uuid.NewString()
	} else {
		event.ID = id.String()
	}
	if !ValidService(event.Service) {
		event.Service = "api"
	}
	if !tokenPattern.MatchString(event.Instance) {
		event.Instance = "unknown"
	}
	if event.Level != "WARN" {
		event.Level = "ERROR"
	}
	if _, ok := messages[event.Code]; !ok && !goCodePattern.MatchString(event.Code) {
		event.Code = "component.error"
	}
	event.Message = messages[event.Code]
	if event.Message == "" {
		event.Message = messages["component.error"]
	}
	event.Route = SafeRoute(event.Route)
	if id, e := uuid.Parse(event.RequestID); e != nil {
		event.RequestID = ""
	} else {
		event.RequestID = id.String()
	}
	if !tokenPattern.MatchString(event.Release) {
		event.Release = "unknown"
	}
	switch event.Browser {
	case "chromium", "firefox", "safari", "edge", "other":
	default:
		event.Browser = ""
	}
	frames := make([]Frame, 0, min(len(event.Stack), 5))
	for _, f := range event.Stack {
		if len(frames) == 5 {
			break
		}
		if len(f.File) <= 160 && assetPattern.MatchString(f.File) && f.Line >= 1 && f.Line <= 10000000 && f.Column >= 1 && f.Column <= 10000000 {
			frames = append(frames, f)
		}
	}
	event.Stack = frames
	if event.FirstSeen.IsZero() {
		event.FirstSeen = time.Now().UTC()
	}
	if event.LastSeen.IsZero() {
		event.LastSeen = event.FirstSeen
	}
	if event.FirstSeen.After(event.LastSeen) {
		event.FirstSeen = event.LastSeen
	}
	now := time.Now().UTC()
	if event.LastSeen.After(now.Add(time.Minute)) {
		event.FirstSeen = now
		event.LastSeen = now
	}
	event.Count = 1
	sum := sha256.Sum256([]byte(strings.Join([]string{event.Service, event.Level, event.Code, event.Route, event.Release, event.Browser}, "\x00")))
	event.Fingerprint = hex.EncodeToString(sum[:])
	return event
}
func EncodeCursor(event Event) string {
	raw, _ := json.Marshal(Cursor{LastSeen: event.LastSeen, ID: event.ID})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func DecodeCursor(raw string) (Cursor, error) {
	var c Cursor
	if raw == "" || len(raw) > 512 {
		return c, apperrors.ErrInvalidInput
	}
	decoded, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return c, apperrors.ErrInvalidInput
	}
	if e = json.Unmarshal(decoded, &c); e != nil || c.LastSeen.IsZero() {
		return c, apperrors.ErrInvalidInput
	}
	id, e := uuid.Parse(c.ID)
	if e != nil {
		return c, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return c, nil
}
func ValidateQuery(q Query) (Query, error) {
	if q.PageSize == 0 {
		q.PageSize = 25
	}
	if q.PageSize < 1 || q.PageSize > 100 {
		return q, apperrors.ErrInvalidInput
	}
	if q.Service != "" && !ValidService(q.Service) {
		return q, apperrors.ErrInvalidInput
	}
	if q.Level != "" && q.Level != "ERROR" && q.Level != "WARN" {
		return q, apperrors.ErrInvalidInput
	}
	if q.Code != "" && !codePattern.MatchString(q.Code) {
		return q, apperrors.ErrInvalidInput
	}
	if q.RequestID != "" {
		if id, e := uuid.Parse(q.RequestID); e != nil {
			return q, apperrors.ErrInvalidInput
		} else {
			q.RequestID = id.String()
		}
	}
	var from, to time.Time
	var e error
	if q.From != "" {
		from, e = time.Parse(time.RFC3339Nano, q.From)
		if e != nil {
			return q, apperrors.ErrInvalidInput
		}
	}
	if q.To != "" {
		to, e = time.Parse(time.RFC3339Nano, q.To)
		if e != nil {
			return q, apperrors.ErrInvalidInput
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return q, apperrors.ErrInvalidInput
	}
	if q.Before != "" {
		if _, e := DecodeCursor(q.Before); e != nil {
			return q, e
		}
	}
	return q, nil
}
