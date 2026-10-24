-- Страницы с записями конференций и сводные отчёты по истории фильтруются по платформе FK.
-- Устаревшие conference_id и частичные индексы, привязанные к статусу, не подходят для этого
-- предиката. Индекс должен быть узким; проверка на сохранение данных выполняется во время
-- запроса.
CREATE INDEX IF NOT EXISTS idx_record_platform_page
    ON record(platform_conference_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL
      AND mode IN ('composite','audio_only','individual_tracks','screen_focus');
