package handler

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"

	"sportslot/internal/maxclient"
	"sportslot/internal/service"
)

type BotWebhookHandler struct {
	dialog    *service.DialogService
	maxClient *maxclient.Client
	secret    string
}

func NewBotWebhookHandler(dialog *service.DialogService, maxClient *maxclient.Client, secret string) *BotWebhookHandler {
	return &BotWebhookHandler{dialog: dialog, maxClient: maxClient, secret: secret}
}

// incomingUpdate - упрощённая структура входящего обновления от MAX Bot API,
// покрывающая обычные текстовые сообщения и нажатия на кнопки (callback).
// Перед продакшен-использованием формат должен быть сверен с актуальной
// документацией MAX Bot API.
type incomingUpdate struct {
	UpdateType string `json:"update_type"`
	Message    struct {
		Sender struct {
			UserID string `json:"user_id"`
			Name   string `json:"name"`
		} `json:"sender"`
		Body struct {
			Text string `json:"text"`
		} `json:"body"`
	} `json:"message"`
	Callback struct {
		Payload string `json:"payload"`
		User    struct {
			UserID string `json:"user_id"`
			Name   string `json:"name"`
		} `json:"user"`
	} `json:"callback"`
}

func (h *BotWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Webhook-Secret")), []byte(h.secret)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid_webhook_secret", "Недействительный секрет webhook")
		return
	}

	var update incomingUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело webhook-запроса")
		return
	}

	var maxUserID, userName, text string
	switch update.UpdateType {
	case "message_callback":
		maxUserID = update.Callback.User.UserID
		userName = update.Callback.User.Name
		text = update.Callback.Payload
	default:
		maxUserID = update.Message.Sender.UserID
		userName = update.Message.Sender.Name
		text = update.Message.Body.Text
	}

	if maxUserID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "Не удалось определить пользователя из webhook")
		return
	}

	resp, err := h.dialog.HandleMessage(r.Context(), maxUserID, userName, text)
	if err != nil {
		log.Printf("bot webhook: dialog handle error: %v", err)
		writeError(w, http.StatusInternalServerError, "dialog_failed", "Ошибка обработки диалога")
		return
	}

	buttons := resp.Buttons
	if resp.MiniAppLink != "" {
		buttons = append(buttons, maxclient.MessageButton{
			Text: "Сравнить в приложении",
			URL:  resp.MiniAppLink,
		})
	}

	if err := h.maxClient.SendMessage(r.Context(), maxUserID, resp.Text, buttons); err != nil {
		log.Printf("bot webhook: send message error: %v", err)
		// Не возвращаем 500 наружу MAX, чтобы платформа не повторяла доставку
		// webhook бесконечно - ошибка отправки уже залогирована.
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
