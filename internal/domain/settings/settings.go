package settings

import (
	"context"
	"encoding/json"

	"github.com/janickiy/meet-space/internal/domain/admin"
)

const DefaultRecordingRetentionDays = 7

// Values is safe to expose to runtime consumers. It never contains credentials.
type Values struct {
	Revision                  int64  `json:"revision"`
	AttachmentMaxBytes        int64  `json:"attachmentMaxBytes"`
	MediaMaxPeers             int    `json:"mediaMaxPeers"`
	MediaMaxRooms             int    `json:"mediaMaxRooms"`
	RecordingRetentionDays    int    `json:"recordingRetentionDays"`
	SMTPEnabled               bool   `json:"smtpEnabled"`
	SMTPHost                  string `json:"smtpHost"`
	SMTPPort                  int    `json:"smtpPort"`
	SMTPUsername              string `json:"smtpUsername"`
	SMTPFrom                  string `json:"smtpFrom"`
	SMTPTLSMode               string `json:"smtpTLSMode"`
	SMTPPasswordConfigured    bool   `json:"smtpPasswordConfigured"`
	EmailVerificationRequired bool   `json:"emailVerificationRequired"`
}

type Reader interface {
	Snapshot(context.Context) (Values, error)
}

// SMTPConfig is restricted to server-side delivery; the password cannot be JSON encoded.
type SMTPConfig struct {
	Enabled                       bool
	Host, Username, From, TLSMode string
	Port                          int
	Password                      string `json:"-"`
}

type State struct {
	Revision               int64           `json:"-"`
	Overrides              json.RawMessage `json:"-"`
	SMTPPasswordCiphertext string          `json:"-"`
}

type SaveInput struct {
	ExpectedRevision       int64
	Overrides              json.RawMessage
	SMTPPasswordCiphertext string
	Fields                 []string
}

type Repository interface {
	Load(context.Context) (State, error)
	Save(context.Context, SaveInput, admin.AuditInput) (State, error)
}

type Patch struct {
	Revision int64                      `json:"revision"`
	Changes  map[string]json.RawMessage `json:"changes"`
}
