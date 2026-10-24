-- Существующие ожидающие приглашения участники не должны оставаться в подвешенном состоянии
-- после перехода по прямой ссылке. Явно отклоненные или заблокированные членства, а также
-- закрытые или запланированные встречи не затрагиваются.
UPDATE conference_participants p
SET status = 'joined', admission_state = 'admitted',
    admission_decided_at = now(), admission_version = admission_version + 1,
    joined_at = now(), left_at = NULL, media_policy_version = media_policy_version + 1
FROM conferences c
WHERE p.conference_id = c.id AND c.status IN ('created', 'active')
  AND p.status = 'waiting' AND p.admission_state = 'waiting';
