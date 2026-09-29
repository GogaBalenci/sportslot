package handler

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"sportslot/internal/maxclient"
	"sportslot/internal/service"
)

type BotWebhookHandler struct {
	dialog     *service.DialogService
	maxClient  *maxclient.Client
	secret     string
	useOpenApp bool
}

func NewBotWebhookHandler(dialog *service.DialogService, maxClient *maxclient.Client, secret string, useOpenApp bool) *BotWebhookHandler {
	return &BotWebhookHandler{dialog: dialog, maxClient: maxClient, secret: secret, useOpenApp: useOpenApp}
}

// flexID принимает идентификатор MAX и как число (так приходит из MAX Bot API),
// и как строку (удобно для ручных curl-проверок).
type flexID string

func (f *flexID) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexID(n.String())
	return nil
}

type maxUser struct {
	UserID    flexID `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`
	IsBot     bool   `json:"is_bot"`
}

func (u maxUser) displayName() string {
	if n := strings.TrimSpace(u.FirstName + " " + u.LastName); n != "" {
		return n
	}
	return u.Name
}

// incomingUpdate — подмножество объекта Update MAX Bot API
// (https://dev.max.ru/docs-api/objects/Update), нужное для сценария бота.
type incomingUpdate struct {
	UpdateType string `json:"update_type"`
	Message    *struct {
		Sender maxUser `json:"sender"`
		Body   struct {
			Text string `json:"text"`
		} `json:"body"`
	} `json:"message"`
	Callback *struct {
		CallbackID string  `json:"callback_id"`
		Payload    string  `json:"payload"`
		User       maxUser `json:"user"`
	} `json:"callback"`
	// bot_started: пользователь нажал «Начать» / перешёл по ссылке на бота.
	User    *maxUser `json:"user"`
	Payload string   `json:"payload"`
}

func (h *BotWebhookHandler) authorized(r *http.Request) bool {
	got := r.Header.Get("X-Max-Bot-Api-Secret")
	if got == "" {
		// Обратная совместимость с локальными curl-сценариями из README.
		got = r.Header.Get("X-Webhook-Secret")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.secret)) == 1
}

// HandleWebhook принимает события MAX. На любое корректно авторизованное
// событие отвечаем 200: иначе MAX повторяет доставку, а после 8 часов
// ошибок автоматически отписывает бота от webhook.
func (h *BotWebhookHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		writeError(w, http.StatusUnauthorized, "invalid_webhook_secret", "Недействительный секрет webhook")
		return
	}

	var update incomingUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		log.Printf("bot webhook: bad body: %v", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	var (
		user       maxUser
		text       string
		callbackID string
	)
	switch update.UpdateType {
	case "message_created":
		if update.Message == nil {
			break
		}
		user = update.Message.Sender
		text = update.Message.Body.Text
	case "message_callback":
		if update.Callback == nil {
			break
		}
		user = update.Callback.User
		text = update.Callback.Payload
		callbackID = update.Callback.CallbackID
	case "bot_started":
		if update.User != nil {
			user = *update.User
		}
		text = "/start"
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	maxUserID := string(user.UserID)
	if maxUserID == "" || user.IsBot {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	// Ответ MAX не должен зависеть от тайм-аута входящего запроса.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if callbackID != "" {
		if err := h.maxClient.AnswerCallback(ctx, callbackID, "Принято"); err != nil {
			log.Printf("bot webhook: answer callback error: %v", err)
		}
	}

	resp, err := h.dialog.HandleMessage(ctx, maxUserID, user.displayName(), text)
	if err != nil {
		log.Printf("bot webhook: dialog handle error: %v", err)
		resp = &service.DialogResponse{Text: maxclient.MsgInternalError}
	}

	buttons := resp.Buttons
	if resp.MiniAppLink != "" {
		if h.useOpenApp {
			// Мини-приложение привязано к боту в кабинете MAX: открываем внутри
			// мессенджера. Если MAX отклонит кнопку, сообщение уйдёт без неё.
			buttons = append(buttons, maxclient.MessageButton{
				Text:    "Сравнить в приложении",
				OpenApp: true,
				Payload: resp.MiniAppPayload,
			})
		} else {
			// Без доступа к кабинету бота: обычная ссылка с sport и user в query.
			buttons = append(buttons, maxclient.MessageButton{
				Text: "Сравнить в приложении",
				URL:  resp.MiniAppLink,
			})
		}
	}

	if err := h.maxClient.SendMessage(ctx, maxUserID, resp.Text, buttons); err != nil {
		log.Printf("bot webhook: send message error: %v", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
