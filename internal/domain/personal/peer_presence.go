package personal

// PeerPresence предоставляет проверенную информацию о том, кто сейчас в сети, в рамках
// авторизованной прямой беседы.  В ней нет имен, адресов электронной почты, идентификаторов
// сеансов или количества подключений.
type PeerPresence struct {
	ConversationID string `json:"conversationId"`
	PeerID         string `json:"peerId"`
	Online         bool   `json:"online"`
}
