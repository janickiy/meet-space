// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package postgres

import (
	domain "github.com/janickiy/meet-space/internal/domain/content"
	"gorm.io/gorm"
)

// reconcileLiveTranscript сверяет окончательные субтитры в реальном времени с текущей
// расшифровкой.
//
// @Parameters:
//   - tx: Текущая транзакция в базе данных.
//   - t: Test дескриптор, используемый для утверждений, очистки и сообщений об ошибках.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func reconcileLiveTranscript(tx *gorm.DB, t domain.Transcript) error {
	if err := tx.Exec(`WITH links AS (
 SELECT s.id,min(l.participant_id::text)::uuid AS participant_id FROM transcript_segments s
 JOIN record r ON r.uuid=? AND r.media_started_at IS NOT NULL
 JOIN live_transcription_sessions ls ON ls.conference_id=r.platform_conference_id
 JOIN live_caption_segments l ON l.session_id=ls.id
 WHERE s.transcript_id=? AND lower(regexp_replace(s.text,'[[:space:]]+',' ','g'))=lower(regexp_replace(l.text,'[[:space:]]+',' ','g'))
 AND abs(EXTRACT(EPOCH FROM((r.media_started_at+s.start_ms*interval '1 millisecond')-(ls.origin+l.start_ms*interval '1 millisecond'))))<=2
 GROUP BY s.id HAVING count(DISTINCT l.participant_id)=1
 ) UPDATE transcript_segments s SET speaker_id=links.participant_id,speaker_label=p.display_name
 FROM links JOIN conference_participants p ON p.id=links.participant_id WHERE s.id=links.id`, t.RecordingID, t.ID).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE live_transcription_sessions SET canonical_recording_id=?,updated_at=clock_timestamp()
 WHERE conference_id=? AND (canonical_recording_id IS NULL OR canonical_recording_id=? OR
 (SELECT created_at FROM record WHERE uuid=canonical_recording_id)<=(SELECT created_at FROM record WHERE uuid=?))`, t.RecordingID, t.ConferenceID, t.RecordingID, t.RecordingID).Error
}
