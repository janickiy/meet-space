package websocket

import "context"

// DisconnectUser cancels only this user's local streams. Replicas retain their
// existing five-second database authorization check.
func (h *UserHandler) DisconnectUser(userID string) {
	h.mu.Lock()
	var stops []context.CancelFunc
	for conn, id := range h.clientUsers {
		if id == userID {
			if stop := h.clients[conn]; stop != nil {
				stops = append(stops, stop)
			}
		}
	}
	h.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}
