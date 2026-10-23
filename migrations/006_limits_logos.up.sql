-- Логотипы студий, сохранение правок профиля демо-студий и индексы для лимитов.

ALTER TABLE venues
    ADD COLUMN IF NOT EXISTS logo_version   VARCHAR(16) NOT NULL DEFAULT '',
    -- Профиль менялся в кабинете: перезапуск не возвращает текст из кода.
    ADD COLUMN IF NOT EXISTS profile_edited BOOLEAN NOT NULL DEFAULT false;
UPDATE venues SET profile_edited = true WHERE source = 'partner';

CREATE TABLE IF NOT EXISTS venue_logos (
    venue_id     UUID PRIMARY KEY REFERENCES venues(id) ON DELETE CASCADE,
    content_type VARCHAR(32) NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg')),
    data         BYTEA NOT NULL CHECK (octet_length(data) <= 1048576),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bookings_user_created ON bookings(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_messages_client ON studio_messages(user_id, created_at) WHERE author = 'client';
CREATE INDEX IF NOT EXISTS idx_user_passes_created ON user_passes(user_id, created_at);
