package maxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL — актуальный домен MAX Bot API (см. dev.max.ru/docs-api).
const DefaultBaseURL = "https://platform-api2.max.ru"

// Client — HTTP-клиент MAX Bot API.
// Авторизация: заголовок `Authorization: <token>` (без префикса Bearer,
// передача токена через query-параметры больше не поддерживается).
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client

	botMu   sync.Mutex
	botInfo *BotInfo
}

func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Enabled сообщает, задан ли токен бота. Без токена исходящие вызовы
// пропускаются (локальный запуск без MAX).
func (c *Client) Enabled() bool { return c.token != "" }

// MessageButton — кнопка inline-клавиатуры в терминах продукта.
// Тип кнопки MAX выбирается автоматически:
//   - OpenApp=true -> open_app (открывает мини-приложение бота внутри MAX);
//   - URL != ""    -> link;
//   - иначе        -> callback (нажатие приходит как update message_callback).
type MessageButton struct {
	Text    string
	Payload string
	URL     string
	OpenApp bool
}

// BotInfo — ответ GET /me (нужен для кнопки open_app).
type BotInfo struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type apiButton struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Payload   string `json:"payload,omitempty"`
	URL       string `json:"url,omitempty"`
	WebApp    string `json:"web_app,omitempty"`
	ContactID int64  `json:"contact_id,omitempty"`
}

type inlineKeyboardPayload struct {
	Buttons [][]apiButton `json:"buttons"`
}

type attachment struct {
	Type    string                `json:"type"`
	Payload inlineKeyboardPayload `json:"payload"`
}

type newMessageBody struct {
	Text        string       `json:"text"`
	Attachments []attachment `json:"attachments,omitempty"`
}

// ErrOpenAppUnavailable — кнопку open_app построить нельзя (нет данных бота).
var ErrOpenAppUnavailable = errors.New("open_app button unavailable")

// SendMessage отправляет пользователю сообщение (POST /messages?user_id=...).
// Если в клавиатуре есть кнопка open_app и MAX её отклонил, сообщение
// повторно отправляется без неё — основной сценарий в чате не должен ломаться.
func (c *Client) SendMessage(ctx context.Context, maxUserID, text string, buttons []MessageButton) error {
	if !c.Enabled() {
		log.Printf("maxclient: MAX_BOT_API_TOKEN is empty, skip message to %s", maxUserID)
		return nil
	}

	body, hasOpenApp := c.buildMessage(ctx, text, buttons, true)
	err := c.postMessage(ctx, maxUserID, body)
	if err != nil && hasOpenApp {
		log.Printf("maxclient: send with open_app failed (%v), retry without it", err)
		body, _ = c.buildMessage(ctx, text, buttons, false)
		err = c.postMessage(ctx, maxUserID, body)
	}
	return err
}

func (c *Client) buildMessage(ctx context.Context, text string, buttons []MessageButton, allowOpenApp bool) (newMessageBody, bool) {
	msg := newMessageBody{Text: text}
	if len(buttons) == 0 {
		return msg, false
	}

	rows := make([][]apiButton, 0, len(buttons))
	hasOpenApp := false
	for _, b := range buttons {
		switch {
		case b.OpenApp:
			if !allowOpenApp {
				continue
			}
			info, err := c.Bot(ctx)
			if err != nil || info.Username == "" {
				log.Printf("maxclient: skip open_app button: %v", err)
				continue
			}
			rows = append(rows, []apiButton{{
				Type:      "open_app",
				Text:      b.Text,
				WebApp:    info.Username,
				ContactID: info.UserID,
				Payload:   b.Payload,
			}})
			hasOpenApp = true
		case b.URL != "":
			rows = append(rows, []apiButton{{Type: "link", Text: b.Text, URL: b.URL}})
		default:
			rows = append(rows, []apiButton{{Type: "callback", Text: b.Text, Payload: b.Payload}})
		}
	}
	if len(rows) > 0 {
		msg.Attachments = []attachment{{
			Type:    "inline_keyboard",
			Payload: inlineKeyboardPayload{Buttons: rows},
		}}
	}
	return msg, hasOpenApp
}

func (c *Client) postMessage(ctx context.Context, maxUserID string, body newMessageBody) error {
	q := url.Values{}
	q.Set("user_id", maxUserID)
	return c.do(ctx, http.MethodPost, "/messages?"+q.Encode(), body, nil)
}

// AnswerCallback подтверждает нажатие callback-кнопки (POST /answers),
// чтобы у пользователя не «висел» индикатор загрузки на кнопке.
func (c *Client) AnswerCallback(ctx context.Context, callbackID, notification string) error {
	if !c.Enabled() || callbackID == "" {
		return nil
	}
	q := url.Values{}
	q.Set("callback_id", callbackID)
	return c.do(ctx, http.MethodPost, "/answers?"+q.Encode(), map[string]string{"notification": notification}, nil)
}

// Bot возвращает (и кэширует) информацию о боте из GET /me.
func (c *Client) Bot(ctx context.Context) (*BotInfo, error) {
	c.botMu.Lock()
	defer c.botMu.Unlock()
	if c.botInfo != nil {
		return c.botInfo, nil
	}
	if !c.Enabled() {
		return nil, ErrOpenAppUnavailable
	}
	var info BotInfo
	if err := c.do(ctx, http.MethodGet, "/me", nil, &info); err != nil {
		return nil, err
	}
	c.botInfo = &info
	return c.botInfo, nil
}

// SetCommands задаёт список команд бота (PATCH /me/commands).
func (c *Client) SetCommands(ctx context.Context, commands map[string]string) error {
	if !c.Enabled() {
		return nil
	}
	type command struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	list := make([]command, 0, len(commands))
	for name, desc := range commands {
		list = append(list, command{Name: name, Description: desc})
	}
	return c.do(ctx, http.MethodPatch, "/me/commands", map[string]interface{}{"commands": list}, nil)
}

// Subscribe регистрирует webhook (POST /subscriptions).
func (c *Client) Subscribe(ctx context.Context, webhookURL, secret string, updateTypes []string) error {
	if !c.Enabled() {
		return nil
	}
	body := map[string]interface{}{
		"url":          webhookURL,
		"update_types": updateTypes,
	}
	if secret != "" {
		body["secret"] = secret
	}
	var res struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := c.do(ctx, http.MethodPost, "/subscriptions", body, &res); err != nil {
		return err
	}
	if !res.Success {
		return fmt.Errorf("subscribe rejected: %s", res.Message)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, in, out interface{}) error {
	var reader io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("max bot api request %s %s: %w", method, strings.SplitN(path, "?", 2)[0], err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("max bot api error: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) SendReminder(ctx context.Context, maxUserID, venueName string, startAt time.Time) error {
	text := fmt.Sprintf(
		"Напоминание: через час у вас тренировка в \"%s\", начало в %s. Не опаздывайте!",
		venueName, startAt.Format("15:04 02.01.2006"),
	)
	return c.SendMessage(ctx, maxUserID, text, nil)
}
