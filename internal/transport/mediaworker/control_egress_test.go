package mediaworker

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/media"
)

func testRecordingEgressRequest() media.EgressRequest {
	return media.EgressRequest{
		RequestID: uuid.NewString(), ConferenceID: uuid.NewString(), RecordingID: uuid.NewString(),
		Route:              media.Route{WorkerID: "media-1", Endpoint: "http://media-worker:8091", LeaseID: uuid.NewString()},
		SegmentDurationSec: 5,
	}
}

func TestEgressRejectsRemovedPurpose(t *testing.T) {
	for _, purpose := range []string{"captions", "recording", ""} {
		t.Run(purpose, func(t *testing.T) {
			request := testRecordingEgressRequest()
			body, err := json.Marshal(struct {
				media.EgressRequest
				Purpose string `json:"purpose"`
			}{EgressRequest: request, Purpose: purpose})
			if err != nil {
				t.Fatal(err)
			}
			h := &Handler{cfg: config.MediaConfig{WorkerID: request.Route.WorkerID, WorkerInternalURL: request.Route.Endpoint}}
			r := httptest.NewRequest(http.MethodPost, "/internal/media/egress", bytes.NewReader(body))
			r.Header.Set("X-Request-ID", request.RequestID)
			w := httptest.NewRecorder()
			h.egress(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("removed purpose accepted: status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRecordingEgressRequestStillDecodes(t *testing.T) {
	want := testRecordingEgressRequest()
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got media.EgressRequest
	if err := strictJSON(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded request = %+v, want %+v", got, want)
	}
}
