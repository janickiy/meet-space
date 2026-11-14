package chat

import (
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeForwardStableSetAndValidation(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	r := ForwardRequest{SourceType: "conversation", SourceID: uuid.NewString(), ClientRequestID: uuid.NewString(), MessageIDs: []string{a, b}}
	_, first, err := NormalizeForward(r)
	if err != nil {
		t.Fatal(err)
	}
	r.MessageIDs = []string{b, a}
	_, second, err := NormalizeForward(r)
	if err != nil || first != second {
		t.Fatal("selection order changed idempotency fingerprint", err)
	}
	for _, ids := range [][]string{nil, {a, a}, {"bad"}, make([]string, MaxForwardMessages+1)} {
		r.MessageIDs = ids
		if _, _, err := NormalizeForward(r); err == nil {
			t.Fatalf("accepted invalid IDs: %v", ids)
		}
	}
	r.MessageIDs = []string{a}
	r.SourceType = "public-url"
	if _, _, err := NormalizeForward(r); err == nil {
		t.Fatal("accepted arbitrary source")
	}
	if ForwardMessageRequestID(r.ClientRequestID, 0) == ForwardMessageRequestID(r.ClientRequestID, 1) || ForwardAttachmentRequestID(r.ClientRequestID, a) == ForwardMessageRequestID(r.ClientRequestID, 0) {
		t.Fatal("idempotency namespaces collide")
	}
}
