package liveproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/janickiy/meet-space/internal/domain/media"
	"io"
	"net/http"
	"strings"
	"time"
)

// Registry resolves only the current room owner's address from the trusted internal registry.
type Registry interface {
	// GetOwner loads the current distributed owner and fencing version of the media room.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//   - argument 2: Conference identifier limiting the operation scope.
	//
	// Returns:
	//   - result 1: The resulting route.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	GetOwner(context.Context, string) (media.Route, error)
}

// Tap opens a separate HTTP stream without proxying media through the API or RabbitMQ.
type Tap struct {
	Registry Registry
	Secret   string
	Client   *http.Client
}

// NewTap constructs the tap with the dependencies and limits required for live speech providers.
//
// Parameters:
//   - registry: Dependency or metrics registry used to locate or register the component.
//   - secret: Private key material or raw secret; it must not be logged or exposed.
//
// Returns:
//   - result 1: The constructed or loaded tap.
func NewTap(registry Registry, secret string) *Tap {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 5 * time.Second
	transport.MaxConnsPerHost = 32
	return &Tap{registry, secret, &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Open opens the scoped media, recording or session resource required by this component.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - cid: Conference identifier limiting the operation scope.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The resulting read closer.
//   - result 2: nil on success; otherwise the validation, access, transport or persistence
//     failure.
func (t *Tap) Open(ctx context.Context, cid, id string) (io.ReadCloser, error) {
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	route, err := t.Registry.GetOwner(op, cid)
	cancel()
	if err != nil || config.ValidateMediaEndpoint(route.Endpoint) != nil {
		return nil, media.ErrUnavailable
	}
	command := media.EgressRequest{Purpose: "captions", RequestID: uuid.NewString(), ConferenceID: cid, RecordingID: id, Route: route, SegmentDurationSec: 5}
	raw, _ := json.Marshal(command)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(raw))
	if err != nil {
		return nil, media.ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+t.Secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", command.RequestID)
	response, err := t.Client.Do(request)
	if err != nil {
		return nil, media.ErrUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, media.ErrUnavailable
	}
	return response.Body, nil
}
