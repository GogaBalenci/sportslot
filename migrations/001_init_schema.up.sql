CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    max_user_id     VARCHAR(128) UNIQUE NOT NULL,
    name            VARCHAR(255),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS venues (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    sport_type      VARCHAR(64) NOT NULL,
    level           VARCHAR(32) NOT NULL,
    address         VARCHAR(500) NOT NULL,
    lat             DOUBLE PRECISION NOT NULL,
    lon             DOUBLE PRECISION NOT NULL,
    source_ref      VARCHAR(255) DEFAULT 'test-data',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- slots: конкретные тренировки на конкретную дату/время (start_at/end_at),
-- квота привязана к конкретному инстансу тренировки, не к шаблону weekday
CREATE TABLE IF NOT EXISTS slots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id        UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    start_at        TIMESTAMPTZ NOT NULL,
    end_at          TIMESTAMPTZ NOT NULL,
    quota_total     SMALLINT NOT NULL CHECK (quota_total > 0),
    quota_booked    SMALLINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT quota_not_exceeded CHECK (quota_booked <= quota_total),
    CONSTRAINT end_after_start CHECK (end_at > start_at)
);

CREATE TABLE IF NOT EXISTS bookings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    slot_id         UUID NOT NULL REFERENCES slots(id),
    status          VARCHAR(16) NOT NULL DEFAULT 'confirmed'
                        CHECK (status IN ('confirmed', 'cancelled')),
    source_channel  VARCHAR(16) NOT NULL CHECK (source_channel IN ('bot', 'miniapp')),
    reminder_sent   BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_slots_venue ON slots(venue_id);
CREATE INDEX IF NOT EXISTS idx_slots_start_at ON slots(start_at);
CREATE INDEX IF NOT EXISTS idx_bookings_user ON bookings(user_id);
CREATE INDEX IF NOT EXISTS idx_bookings_slot ON bookings(slot_id);
CREATE INDEX IF NOT EXISTS idx_venues_sport_level ON venues(sport_type, level);
