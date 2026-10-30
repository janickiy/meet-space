// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package records

// Command содержит данные команд, используемые для метаданных записи.
type Command struct {
	Type               string `json:"type"`
	RecordID           string `json:"recordId"`
	Reason             string `json:"reason,omitempty"`
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
}

// RecordDetails объединяет запись с сопутствующими файлами, сегментами и событиями для чтения
// приложениями.
type RecordDetails struct {
	Record   Record
	Files    []RecordFile
	Segments []RecordSegment
	Events   []RecordEvent
}

// RecordCard содержит авторизованное представление записи с разрешенными ссылками и
// сопутствующими артефактами.
type RecordCard struct {
	Record
	Files    []RecordFileView    `json:"files"`
	Segments []RecordSegmentView `json:"segments"`
	Events   []RecordEvent       `json:"events,omitempty"`
}

// RecordFileView добавляет временную авторизованную ссылку для чтения к метаданным сохраненного
// файла.
type RecordFileView struct {
	RecordFile
	URL string `json:"url,omitempty"`
}

// RecordSegmentView добавляет приватный сегмент и временную ссылку для чтения к метаданным
// сегмента.
type RecordSegmentView struct {
	RecordSegment
	Bucket string `json:"bucket,omitempty"`
	URL    string `json:"url,omitempty"`
}
