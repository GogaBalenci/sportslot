package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"sportslot/internal/bot"
)

// BotWebhook принимает обновления MAX (POST /bot/webhook).
type BotWebhook struct {
	bot    *bot.Bot
	secret string

	// События одного пользователя обрабатываются по очереди, в порядке
	// поступления: при быстрых нажатиях кнопок сценарий не путается.
	mu     sync.Mutex
	queues map[string]chan bot.Incoming

	// Защита от флуда: не больше 5 событий за 3 секунды от одного человека,
	// одинаковые нажатия и команды подряд (за 2 секунды) считаются одним.
	rate    *limiter
	repeats *limiter
	warns   *limiter
	answers *limiter
}

func (h *BotWebhook) allow(in bot.Incoming) (ok, warn bool) {
	if h.rate == nil {
		return true, false
	}
	now := time.Now()
	same := in.MaxUserID + "|" + in.Payload + "|" + in.Text
	if in.Started {
		same = in.MaxUserID + "|start"
	}
	if in.HasLocation {
		same = ""
	}
	if same != "" && !h.repeats.allow(same, now) {
		// Двойное нажатие: молча отбрасываем, но кнопку закрываем.
		return false, false
	}
	if !h.rate.allow(in.MaxUserID, now) {
		return false, h.warns.allow(in.MaxUserID, now)
	}
	return true, false
}

const userQueueSize = 32

func (h *BotWebhook) dispatch(in bot.Incoming) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.queues == nil {
		h.queues = map[string]chan bot.Incoming{}
	}
	if ok, warn := h.allow(in); !ok {
		if in.CallbackID != "" && !h.answers.allow(in.MaxUserID, time.Now()) {
			return
		}
		if in.CallbackID == "" && !warn {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			h.bot.Throttled(ctx, in, warn)
		}()
		return
	}
	q, running := h.queues[in.MaxUserID]
	if !running {
		q = make(chan bot.Incoming, userQueueSize)
		h.queues[in.MaxUserID] = q
		go h.drain(in.MaxUserID, q)
	}
	select {
	case q <- in:
	default:
		log.Printf("webhook: queue of %s is full, event dropped", in.MaxUserID)
	}
}

func (h *BotWebhook) drain(userID string, q chan bot.Incoming) {
	for {
		h.mu.Lock()
		select {
		case in := <-q:
			h.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			h.bot.Handle(ctx, in)
			cancel()
		default:
			delete(h.queues, userID)
			h.mu.Unlock()
			return
		}
	}
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
	h.dispatch(in)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
