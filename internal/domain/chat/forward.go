package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

const MaxForwardMessages = 30
const MaxForwardAttachments = 20
const MaxForwardBytes int64 = 100 << 20

type ForwardRequest struct {
	SourceType      string   `json:"sourceType"`
	SourceID        string   `json:"sourceId"`
	MessageIDs      []string `json:"messageIds"`
	ClientRequestID string   `json:"clientRequestId"`
}

// ForwardPlan contains only server-loaded source data or a completed replay.
type ForwardPlan struct {
	Messages []Message
	Existing []Message
}

func NormalizeForward(request ForwardRequest) (ForwardRequest, string, error) {
	if request.SourceType != "conversation" && request.SourceType != "conference" {
		return request, "", apperrors.New(apperrors.ErrInvalidInput, "invalid forwarding source type")
	}
	var err error
	request.SourceID, err = UUID(request.SourceID)
	if err != nil {
		return request, "", err
	}
	request.ClientRequestID, err = UUID(request.ClientRequestID)
	if err != nil {
		return request, "", err
	}
	if len(request.MessageIDs) == 0 || len(request.MessageIDs) > MaxForwardMessages {
		return request, "", apperrors.New(apperrors.ErrInvalidInput, "select between 1 and 30 messages")
	}
	ids := make([]string, len(request.MessageIDs))
	seen := make(map[string]bool, len(ids))
	for i, raw := range request.MessageIDs {
		id, err := UUID(raw)
		if err != nil {
			return request, "", err
		}
		if seen[id] {
			return request, "", apperrors.New(apperrors.ErrInvalidInput, "duplicate forwarded message")
		}
		seen[id] = true
		ids[i] = id
	}
	sort.Strings(ids)
	request.MessageIDs = ids
	encoded, _ := json.Marshal(struct {
		Kind       string
		SourceType string
		SourceID   string
		MessageIDs []string
	}{"forward", request.SourceType, request.SourceID, request.MessageIDs})
	sum := sha256.Sum256(encoded)
	return request, hex.EncodeToString(sum[:]), nil
}

func ForwardMessageRequestID(batchID string, position int) string {
	return uuid.NewSHA1(uuid.MustParse(batchID), []byte("meetspace-forward-message:"+strconv.Itoa(position))).String()
}

func ForwardAttachmentRequestID(batchID, attachmentID string) string {
	return uuid.NewSHA1(uuid.MustParse(batchID), []byte("meetspace-forward-attachment:"+attachmentID)).String()
}
