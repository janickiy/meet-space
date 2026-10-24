// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package personal

import "time"

// Peer содержит общедоступную информацию о собеседниках без указания электронной почты, ролей
// или данных о сеансе.
type Peer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}
type Conversation struct {
	ID                    string     `json:"id"`
	Type                  string     `json:"type"`
	Peer                  *Peer      `json:"peer,omitempty"`
	Name                  string     `json:"name,omitempty"`
	Description           string     `json:"description"`
	CreatedBy             *string    `json:"createdBy,omitempty"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	MemberCount           int64      `json:"memberCount,omitempty"`
	MyRole                Role       `json:"myRole,omitempty"`
	AvatarVersion         *string    `json:"avatarVersion"`
	LastSender            *Peer      `json:"lastSender"`
	CreatedAt             time.Time  `json:"createdAt"`
	LastMessageAt         *time.Time `json:"lastMessageAt"`
	LastMessageID         *string    `json:"lastMessageId"`
	Preview               string     `json:"preview"`
	UnreadCount           int64      `json:"unreadCount"`
	NotificationsEnabled  bool       `json:"notificationsEnabled"`
	HistoryClearedThrough int64      `json:"historyClearedThrough"`
	ActivityAt            time.Time  `json:"-"`
}
type Page struct {
	Items       []Conversation `json:"items"`
	NextCursor  string         `json:"nextCursor,omitempty"`
	UnreadCount int64          `json:"unreadCount"`
}
