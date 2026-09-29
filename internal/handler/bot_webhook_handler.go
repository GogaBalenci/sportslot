package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"sportslot/internal/bot"
)

// BotWebhook принимает обновления MAX (POST /bot/webhook).
type BotWebhook struct {
	bot    *bot.Bot
	secret string
}

type maxUser struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Name      string `json:"name"`
}

func (u maxUser) id() string { return strconv.FormatInt(u.UserID, 10) }

func (u maxUser) fullName() string {
	if u.FirstName != "" || u.LastName != "" {
		return u.FirstName + " " + u.LastName
	}
	return u.Name
}

type rawAttachment struct {
	Type      string   `json:"type"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Payload   *struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	} `json:"payload"`
}

type update struct {
	UpdateType string `json:"update_type"`
	Message    *struct {
		Sender maxUser `json:"sender"`
		Body   struct {
			Text        string          `json:"text"`
			Attachments []rawAttachment `json:"attachments"`
		} `json:"body"`
	} `json:"message"`
	Callback *struct {
		CallbackID string  `json:"callback_id"`
		Payload    string  `json:"payload"`
		User       maxUser `json:"user"`
	} `json:"callback"`
	User *maxUser `json:"user"`
}

// incoming переводит Update MAX в событие сценария бота.
func (u update) incoming() (bot.Incoming, bool) {
	switch u.UpdateType {
	case "bot_started":
		if u.User == nil {
			return bot.Incoming{}, false
		}
		return bot.Incoming{MaxUserID: u.User.id(), Name: u.User.fullName(), Started: true}, true
	case "message_callback":
		if u.Callback == nil {
			return bot.Incoming{}, false
		}
		return bot.Incoming{MaxUserID: u.Callback.User.id(), Name: u.Callback.User.fullName(),
			Payload: u.Callback.Payload, CallbackID: u.Callback.CallbackID}, true
	case "message_created":
		if u.Message == nil || u.Message.Sender.UserID == 0 {
			return bot.Incoming{}, false
		}
		in := bot.Incoming{MaxUserID: u.Message.Sender.id(), Name: u.Message.Sender.fullName(), Text: u.Message.Body.Text}
		for _, a := range u.Message.Body.Attachments {
			if a.Type != "location" {
				continue
			}
			lat, lon := a.Latitude, a.Longitude
			if (lat == nil || lon == nil) && a.Payload != nil {
				lat, lon = a.Payload.Latitude, a.Payload.Longitude
			}
			if lat != nil && lon != nil {
				in.HasLocation, in.Lat, in.Lon = true, *lat, *lon
			}
		}
		return in, true
	}
	return bot.Incoming{}, false
}

func (h *BotWebhook) Handle(w http.ResponseWriter, r *http.Request) {
	if h.secret != "" {
		got := r.Header.Get("X-Max-Bot-Api-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(h.secret)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid_secret", "Неверный секрет webhook")
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Не удалось прочитать тело запроса")
		return
	}
	var upd update
	if err := json.Unmarshal(body, &upd); err != nil {
		log.Printf("webhook: bad update: %v", err)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	in, ok := upd.incoming()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	// MAX ждёт быстрый ответ, а сценарий может отправить несколько сообщений —
	// обрабатываем событие в фоне со своим таймаутом.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.bot.Handle(ctx, in)
	}()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
