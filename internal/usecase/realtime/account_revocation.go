package realtime

// DisconnectUser closes this API instance's conference sockets using their
// normal lifecycle, including media cleanup by the disconnect observers. Other
// API instances detect the revoked durable session on the next heartbeat.
func (h *Hub) DisconnectUser(userID string) {
	for _, entry := range h.entries("") {
		if entry.session.UserID == userID {
			entry.socket.Stop("authentication_revoked")
		}
	}
}
