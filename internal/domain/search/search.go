// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Package search defines keyword search over authorized conference titles.
package search

import "time"

// Query limits conference-title search to the caller's visible meetings.
type Query struct {
	Mode, Source, Query, ConferenceID, ParticipantID, Membership string
	From, To                                                     *time.Time
	Limit, Offset                                                int
}

type Result struct {
	Type            string  `json:"type"`
	ConferenceID    string  `json:"conferenceId"`
	ConferenceTitle string  `json:"conferenceTitle"`
	Snippet         string  `json:"snippet"`
	Rank            float64 `json:"rank"`
}

type Page struct {
	EffectiveMode string   `json:"effectiveMode"`
	Items         []Result `json:"items"`
	Total         int64    `json:"total"`
	Limit         int      `json:"limit"`
	Offset        int      `json:"offset"`
}
