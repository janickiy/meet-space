package websocket

import (
	"context"
	"errors"
	ws "github.com/gorilla/websocket"
	"time"
)

var errUserAuthenticationRevoked = errors.New("user authentication revoked")

// An explicit close lets the browser clear its session immediately instead of
// treating an administrative block as a temporary network interruption.
func closeUserAuthentication(conn *ws.Conn, timeout time.Duration) {
	_ = conn.WriteControl(ws.CloseMessage, ws.FormatCloseMessage(4401, "authentication_revoked"), time.Now().Add(timeout))
}

// DisconnectUser cancels only this user's local streams. Replicas retain their
// existing five-second database authorization check.
func (h *UserHandler) DisconnectUser(userID string) {
	h.mu.Lock()
	var stops []context.CancelCauseFunc
	for conn, id := range h.clientUsers {
		if id == userID {
			if stop := h.clients[conn]; stop != nil {
				stops = append(stops, stop)
			}
		}
	}
	h.mu.Unlock()
	for _, stop := range stops {
		stop(errUserAuthenticationRevoked)
	}
}
