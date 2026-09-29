-- Пилот в Ростове-на-Дону: реальный каталог из OpenStreetMap, демо-партнёры
-- с онлайн-записью, QR-отметка посещения, лист ожидания и состояние диалогов.

-- Московские тестовые данные MVP больше не нужны: каталог и расписание
-- загружаются заново при старте API.
DELETE FROM bookings;
DELETE FROM slots;
DELETE FROM venues;
DELETE FROM users WHERE max_user_id LIKE 'max-test-%';

ALTER TABLE venues
    ADD COLUMN IF NOT EXISTS external_id    VARCHAR(64),
    ADD COLUMN IF NOT EXISTS sports         TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS district       VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source         VARCHAR(16) NOT NULL DEFAULT 'demo_partner'
        CHECK (source IN ('osm', 'demo_partner')),
    ADD COLUMN IF NOT EXISTS source_url     VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS booking_mode   VARCHAR(16) NOT NULL DEFAULT 'external'
        CHECK (booking_mode IN ('instant', 'external')),
    ADD COLUMN IF NOT EXISTS phone          VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS website        VARCHAR(500) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS opening_hours  VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS what_to_bring  TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trial_price    INTEGER,
    ADD COLUMN IF NOT EXISTS verified_at    TIMESTAMPTZ;

ALTER TABLE venues ALTER COLUMN address TYPE VARCHAR(500);
CREATE UNIQUE INDEX IF NOT EXISTS uq_venues_external_id ON venues(external_id) WHERE external_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_venues_sports ON venues USING GIN (sports);

ALTER TABLE slots
    ADD COLUMN IF NOT EXISTS title       VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sport_type  VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS level       VARCHAR(32) NOT NULL DEFAULT 'beginner';

ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_status_check;
ALTER TABLE bookings ADD CONSTRAINT bookings_status_check
    CHECK (status IN ('confirmed', 'cancelled', 'attended', 'no_show'));

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS checkin_code      CHAR(6),
    ADD COLUMN IF NOT EXISTS attended_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reminder_24h_sent BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS reminder_2h_sent  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS feedback_asked    BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS rating            SMALLINT CHECK (rating BETWEEN 1 AND 5);

-- Одна активная запись пользователя на занятие: повторное нажатие не создаёт дубль.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_active_user_slot
    ON bookings(user_id, slot_id) WHERE status = 'confirmed';
-- Код входа уникален среди активных записей.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_active_checkin
    ON bookings(checkin_code) WHERE status = 'confirmed';

CREATE TABLE IF NOT EXISTS waitlist (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    slot_id     UUID NOT NULL REFERENCES slots(id) ON DELETE CASCADE,
    status      VARCHAR(16) NOT NULL DEFAULT 'waiting'
                    CHECK (status IN ('waiting', 'booked', 'cancelled')),
    booking_id  UUID REFERENCES bookings(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_waitlist_waiting
    ON waitlist(user_id, slot_id) WHERE status = 'waiting';
CREATE INDEX IF NOT EXISTS idx_waitlist_slot ON waitlist(slot_id, created_at);

CREATE TABLE IF NOT EXISTS dialog_states (
    max_user_id  VARCHAR(128) PRIMARY KEY,
    state        JSONB NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS partner_sessions (
    token       CHAR(64) PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);

-- События воронки для метрик пилота (без персональных данных, кроме id MAX).
CREATE TABLE IF NOT EXISTS events (
    id          BIGSERIAL PRIMARY KEY,
    max_user_id VARCHAR(128) NOT NULL DEFAULT '',
    kind        VARCHAR(32) NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_events_kind_time ON events(kind, created_at);
