-- Отзывы, онлайн-оплата, абонементы, запись с другом, время ответа студии,
-- утренняя сводка и служебный статус мониторинга.

-- Настройки студии, которые она меняет в кабинете.
CREATE TABLE IF NOT EXISTS studio_settings (
    venue_id            UUID PRIMARY KEY REFERENCES venues(id) ON DELETE CASCADE,
    show_reviews        BOOLEAN NOT NULL DEFAULT false,
    daily_digest        BOOLEAN NOT NULL DEFAULT true,
    payment_provider    VARCHAR(16) NOT NULL DEFAULT ''
                            CHECK (payment_provider IN ('', 'yookassa', 'tbank')),
    -- shopId ЮKassa или TerminalKey Т-Кассы.
    payment_account     VARCHAR(128) NOT NULL DEFAULT '',
    -- Секретный ключ или пароль терминала, зашифрован AES-256-GCM ключом сервера.
    payment_secret      BYTEA,
    payment_secret_hint VARCHAR(8) NOT NULL DEFAULT '',
    payments_enabled    BOOLEAN NOT NULL DEFAULT false,
    trial_online        BOOLEAN NOT NULL DEFAULT false,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Абонементы, которые продаёт студия.
CREATE TABLE IF NOT EXISTS passes (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id    UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    title       VARCHAR(120) NOT NULL,
    visits      SMALLINT NOT NULL CHECK (visits BETWEEN 1 AND 100),
    valid_days  SMALLINT NOT NULL CHECK (valid_days BETWEEN 1 AND 365),
    price       INTEGER NOT NULL CHECK (price BETWEEN 1 AND 1000000),
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_passes_venue ON passes(venue_id);

-- Купленные абонементы. requested — ждёт оплаты онлайн или подтверждения студии.
CREATE TABLE IF NOT EXISTS user_passes (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pass_id      UUID REFERENCES passes(id) ON DELETE SET NULL,
    venue_id     UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id),
    title        VARCHAR(120) NOT NULL,
    visits_total SMALLINT NOT NULL,
    visits_left  SMALLINT NOT NULL CHECK (visits_left >= 0),
    valid_days   SMALLINT NOT NULL,
    price        INTEGER NOT NULL,
    status       VARCHAR(16) NOT NULL DEFAULT 'requested'
                     CHECK (status IN ('requested', 'active', 'used', 'expired', 'cancelled')),
    paid_by      VARCHAR(16) NOT NULL DEFAULT '' CHECK (paid_by IN ('', 'online', 'studio')),
    valid_until  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_user_passes_user ON user_passes(user_id, venue_id, status);
CREATE INDEX IF NOT EXISTS idx_user_passes_venue ON user_passes(venue_id, status);

ALTER TABLE bookings
    ADD COLUMN IF NOT EXISTS review          TEXT CHECK (char_length(review) <= 500),
    ADD COLUMN IF NOT EXISTS reviewed_at     TIMESTAMPTZ,
    -- Место для друга: пока друг не открыл ссылку, запись принадлежит пригласившему.
    ADD COLUMN IF NOT EXISTS guest_token     CHAR(32),
    ADD COLUMN IF NOT EXISTS host_booking_id UUID REFERENCES bookings(id),
    ADD COLUMN IF NOT EXISTS user_pass_id    UUID REFERENCES user_passes(id),
    ADD COLUMN IF NOT EXISTS paid_at         TIMESTAMPTZ;

-- У пользователя может быть своя запись и место для друга на одно занятие.
DROP INDEX IF EXISTS uq_bookings_active_user_slot;
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_active_user_slot
    ON bookings(user_id, slot_id) WHERE status = 'confirmed' AND guest_token IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_bookings_guest_token
    ON bookings(guest_token) WHERE guest_token IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bookings_host ON bookings(host_booking_id) WHERE host_booking_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS payments (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    venue_id            UUID NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id),
    provider            VARCHAR(16) NOT NULL CHECK (provider IN ('yookassa', 'tbank')),
    kind                VARCHAR(16) NOT NULL CHECK (kind IN ('trial', 'pass')),
    booking_id          UUID REFERENCES bookings(id),
    user_pass_id        UUID REFERENCES user_passes(id),
    amount              INTEGER NOT NULL CHECK (amount > 0), -- в копейках
    description         VARCHAR(128) NOT NULL DEFAULT '',
    status              VARCHAR(16) NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'succeeded', 'canceled')),
    provider_payment_id VARCHAR(64),
    confirmation_url    TEXT NOT NULL DEFAULT '',
    checked_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at             TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_payments_provider ON payments(provider, provider_payment_id)
    WHERE provider_payment_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_payments_venue ON payments(venue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_payments_pending ON payments(created_at) WHERE status = 'pending';

-- Напоминания о сообщениях без ответа: студии через час, клиенту через три.
ALTER TABLE studio_messages
    ADD COLUMN IF NOT EXISTS studio_nudged_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS client_notified_at TIMESTAMPTZ;

-- Утренняя сводка уходит администратору один раз в день.
CREATE TABLE IF NOT EXISTS digest_log (
    max_user_id VARCHAR(128) NOT NULL,
    day         DATE NOT NULL,
    PRIMARY KEY (max_user_id, day)
);

-- Служебные отметки мониторинга (последний бэкап и т. п.) для команды /status.
CREATE TABLE IF NOT EXISTS ops_status (
    key        VARCHAR(64) PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
