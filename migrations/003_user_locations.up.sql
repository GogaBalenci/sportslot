-- Последняя точка, которую пользователь отправил боту (геолокация или адрес).
-- Мини-приложение использует её, когда геолокация в клиенте MAX недоступна.
CREATE TABLE IF NOT EXISTS user_locations (
    max_user_id VARCHAR(128) PRIMARY KEY,
    lat         DOUBLE PRECISION NOT NULL,
    lon         DOUBLE PRECISION NOT NULL,
    address     VARCHAR(500) NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
