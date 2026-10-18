package realtime

// AllowedReaction checks whether a reaction belongs to the fixed set supported by the protocol.
//
// Parameters:
//   - emoji: One of the protocol-supported temporary reactions.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func AllowedReaction(emoji string) bool {
	return emoji == "👍" || emoji == "👏" || emoji == "❤️" || emoji == "😂"
}

// LowPriorityEvent identifies events that may use the bounded low-priority realtime queue.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func LowPriorityEvent(kind string) bool {
	switch kind {
	case "caption.partial", "caption.final", "caption.status", "reaction.created", "chat.message.created", "chat.message.updated", "chat.message.deleted", "chat.read.updated":
		return true
	default:
		return false
	}
}
