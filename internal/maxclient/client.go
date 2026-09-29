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

// Client — HTTP-клиент MAX Bot API. Авторизация заголовком `Authorization: <token>`.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client

	// miniAppURL задан, если open_app недоступен (мини-приложение не привязано
	// к боту): тогда кнопки open_app отправляются обычными ссылками.
	miniAppURL string

	botMu   sync.Mutex
	botInfo *BotInfo
}

func NewClient(baseURL, token string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// UseLinksForMiniApp включает замену open_app на ссылки вида <url>?start=<payload>.
func (c *Client) UseLinksForMiniApp(miniAppURL string) {
	c.miniAppURL = strings.TrimRight(miniAppURL, "/") + "/"
}

func (c *Client) Enabled() bool { return c.token != "" }

func (c *Client) Token() string { return c.token }

type ButtonType string

const (
	ButtonCallback ButtonType = "callback"
	ButtonLink     ButtonType = "link"
	ButtonOpenApp  ButtonType = "open_app"
	ButtonGeo      ButtonType = "request_geo_location"
)

type Button struct {
	Type    ButtonType
	Text    string
	Payload string
	URL     string
}

func Callback(text, payload string) Button {
	return Button{Type: ButtonCallback, Text: text, Payload: payload}
}
func Link(text, url string) Button { return Button{Type: ButtonLink, Text: text, URL: url} }
func OpenApp(text, payload string) Button {
	return Button{Type: ButtonOpenApp, Text: text, Payload: payload}
}
func RequestGeo(text string) Button { return Button{Type: ButtonGeo, Text: text} }

// Keyboard — ряды кнопок inline-клавиатуры.
type Keyboard [][]Button

type BotInfo struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type apiButton struct {
	Type      ButtonType `json:"type"`
	Text      string     `json:"text"`
	Payload   string     `json:"payload,omitempty"`
	URL       string     `json:"url,omitempty"`
	WebApp    string     `json:"web_app,omitempty"`
	ContactID int64      `json:"contact_id,omitempty"`
}

type attachment struct {
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
	Latitude  float64     `json:"latitude,omitempty"`
	Longitude float64     `json:"longitude,omitempty"`
}

type newMessageBody struct {
	Text        string       `json:"text,omitempty"`
	Attachments []attachment `json:"attachments,omitempty"`
}

// SendMessage отправляет сообщение пользователю (POST /messages?user_id=...).
// Если MAX отклонил клавиатуру с open_app, сообщение уходит без этих кнопок:
// основной сценарий в чате не должен зависеть от мини-приложения.
func (c *Client) SendMessage(ctx context.Context, maxUserID, text string, kb Keyboard) error {
	if !c.Enabled() {
		log.Printf("maxclient: token is empty, message to %s skipped", maxUserID)
		return nil
	}
	body, hasOpenApp := c.buildMessage(ctx, text, kb, true)
	err := c.postMessage(ctx, maxUserID, body)
	if err != nil && hasOpenApp {
		log.Printf("maxclient: message with open_app rejected (%v), retry without it", err)
		body, _ = c.buildMessage(ctx, text, kb, false)
		err = c.postMessage(ctx, maxUserID, body)
	}
	return err
}

// SendLocation отправляет точку на карте отдельным сообщением.
func (c *Client) SendLocation(ctx context.Context, maxUserID string, lat, lon float64) error {
	if !c.Enabled() {
		return nil
	}
	return c.postMessage(ctx, maxUserID, newMessageBody{
		Attachments: []attachment{{Type: "location", Latitude: lat, Longitude: lon}},
	})
}

func (c *Client) buildMessage(ctx context.Context, text string, kb Keyboard, allowOpenApp bool) (newMessageBody, bool) {
	msg := newMessageBody{Text: text}
	hasOpenApp := false
	var rows [][]apiButton
	for _, row := range kb {
		var out []apiButton
		for _, b := range row {
			switch b.Type {
			case ButtonOpenApp:
				if c.miniAppURL != "" {
					out = append(out, apiButton{Type: ButtonLink, Text: b.Text, URL: c.miniAppURL + "?start=" + url.QueryEscape(b.Payload)})
					continue
				}
				if !allowOpenApp {
					continue
				}
				info, err := c.Bot(ctx)
				if err != nil || info.Username == "" {
					continue
				}
				out = append(out, apiButton{Type: ButtonOpenApp, Text: b.Text, Payload: b.Payload,
					WebApp: info.Username, ContactID: info.UserID})
				hasOpenApp = true
			case ButtonLink:
				out = append(out, apiButton{Type: ButtonLink, Text: b.Text, URL: b.URL})
			case ButtonGeo:
				out = append(out, apiButton{Type: ButtonGeo, Text: b.Text})
			default:
				out = append(out, apiButton{Type: ButtonCallback, Text: b.Text, Payload: b.Payload})
			}
		}
		if len(out) > 0 {
			rows = append(rows, out)
		}
	}
	if len(rows) > 0 {
		msg.Attachments = []attachment{{
			Type:    "inline_keyboard",
			Payload: map[string]interface{}{"buttons": rows},
		}}
	}
	return msg, hasOpenApp
}

func (c *Client) postMessage(ctx context.Context, maxUserID string, body newMessageBody) error {
	q := url.Values{}
	q.Set("user_id", maxUserID)
	return c.do(ctx, http.MethodPost, "/messages?"+q.Encode(), body, nil)
}

// AnswerCallback подтверждает нажатие callback-кнопки (POST /answers).
func (c *Client) AnswerCallback(ctx context.Context, callbackID, notification string) error {
	if !c.Enabled() || callbackID == "" {
		return nil
	}
	q := url.Values{}
	q.Set("callback_id", callbackID)
	return c.do(ctx, http.MethodPost, "/answers?"+q.Encode(), map[string]string{"notification": notification}, nil)
}

var errNoToken = errors.New("bot token is empty")

// Bot возвращает (и кэширует) информацию о боте из GET /me.
func (c *Client) Bot(ctx context.Context) (*BotInfo, error) {
	c.botMu.Lock()
	defer c.botMu.Unlock()
	if c.botInfo != nil {
		return c.botInfo, nil
	}
	if !c.Enabled() {
		return nil, errNoToken
	}
	var info BotInfo
	if err := c.do(ctx, http.MethodGet, "/me", nil, &info); err != nil {
		return nil, err
	}
	c.botInfo = &info
	return c.botInfo, nil
}

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SetCommands задаёт меню команд бота (PATCH /me/commands).
func (c *Client) SetCommands(ctx context.Context, commands []Command) error {
	if !c.Enabled() {
		return nil
	}
	return c.do(ctx, http.MethodPatch, "/me/commands", map[string]interface{}{"commands": commands}, nil)
}

// Subscribe регистрирует webhook (POST /subscriptions).
func (c *Client) Subscribe(ctx context.Context, webhookURL, secret string, updateTypes []string) error {
	if !c.Enabled() {
		return nil
	}
	body := map[string]interface{}{"url": webhookURL, "update_types": updateTypes}
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
		return fmt.Errorf("max bot api %s %s: %w", method, strings.SplitN(path, "?", 2)[0], err)
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
