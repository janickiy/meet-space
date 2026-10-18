package personal

// PeerPresence exposes verified online state within an authorized direct conversation. It
// contains no names, email addresses, session identifiers or connection counts.
type PeerPresence struct {
	ConversationID string `json:"conversationId"`
	PeerID         string `json:"peerId"`
	Online         bool   `json:"online"`
}
