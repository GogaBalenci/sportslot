-- Кабинеты студий: администраторы по приглашениям, шаблоны расписания,
-- управление занятиями и переписка с клиентами.

-- partner — реальная студия, подключённая к онлайн-записи.
ALTER TABLE venues DROP CONSTRAINT IF EXISTS venues_source_check;
ALTER TABLE venues ADD CONSTRAINT venues_source_check
    CHECK (source IN ('osm', 'demo_partner', 'partner'));

CREATE TABLE IF NOT EXISTS schedule_templates (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id      UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    weekday       SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_minute  SMALLINT NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    duration_min  SMALLINT NOT NULL CHECK (duration_min BETWEEN 15 AND 300),
    title         VARCHAR(255) NOT NULL,
    sport_type    VARCHAR(32) NOT NULL,
    level         VARCHAR(32) NOT NULL DEFAULT 'beginner',
    quota         SMALLINT NOT NULL CHECK (quota BETWEEN 1 AND 100),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_templates_venue ON schedule_templates(venue_id, weekday, start_minute);

ALTER TABLE slots
    ADD COLUMN IF NOT EXISTS template_id    UUID REFERENCES schedule_templates(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS booking_closed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS cancelled_at   TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_slots_template ON slots(template_id, start_at);

-- user — отменил клиент, studio — студия отменила занятие.
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS cancel_reason VARCHAR(16) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS studio_admins (
    venue_id    UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    max_user_id VARCHAR(128) NOT NULL,
    name        VARCHAR(255) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (venue_id, max_user_id)
);
CREATE INDEX IF NOT EXISTS idx_studio_admins_user ON studio_admins(max_user_id);

CREATE TABLE IF NOT EXISTS studio_invites (
    token       CHAR(32) PRIMARY KEY,
    venue_id    UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    created_by  VARCHAR(128) NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    used_by     VARCHAR(128),
    used_at     TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS studio_messages (
    id         BIGSERIAL PRIMARY KEY,
    venue_id   UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id),
    author     VARCHAR(8) NOT NULL CHECK (author IN ('client', 'studio')),
    text       TEXT NOT NULL CHECK (char_length(text) BETWEEN 1 AND 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON studio_messages(venue_id, user_id, created_at);
