#!/usr/bin/env sh
# Управление webhook-подпиской бота MAX. Читает токен и секрет из .env.
#   ./deploy/max-webhook.sh me           — проверить токен (GET /me)
#   ./deploy/max-webhook.sh list         — текущие подписки
#   ./deploy/max-webhook.sh subscribe    — подписать https://$DOMAIN/bot/webhook
#   ./deploy/max-webhook.sh unsubscribe  — отписать (включит long polling)
set -eu
cd "$(dirname "$0")/.."
[ -f .env ] || { echo ".env not found" >&2; exit 1; }
set -a; . ./.env; set +a

API="${MAX_BOT_API_BASE_URL:-https://platform-api2.max.ru}"
: "${MAX_BOT_API_TOKEN:?MAX_BOT_API_TOKEN is empty in .env}"
HOOK="https://${DOMAIN:?DOMAIN is empty in .env}/bot/webhook"

case "${1:-}" in
  me)
    curl -sS "$API/me" -H "Authorization: $MAX_BOT_API_TOKEN"; echo ;;
  list)
    curl -sS "$API/subscriptions" -H "Authorization: $MAX_BOT_API_TOKEN"; echo ;;
  subscribe)
    curl -sS -X POST "$API/subscriptions" \
      -H "Authorization: $MAX_BOT_API_TOKEN" -H "Content-Type: application/json" \
      -d "{\"url\":\"$HOOK\",\"update_types\":[\"message_created\",\"message_callback\",\"bot_started\"],\"secret\":\"$MAX_WEBHOOK_SECRET\"}"
    echo ;;
  unsubscribe)
    curl -sS -X DELETE "$API/subscriptions?url=$HOOK" -H "Authorization: $MAX_BOT_API_TOKEN"; echo ;;
  *)
    echo "usage: $0 {me|list|subscribe|unsubscribe}" >&2; exit 2 ;;
esac
