package maxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type MessageButton struct {
	Text    string `json:"text"`
	Payload string `json:"payload,omitempty"`
	URL     string `json:"url,omitempty"`
}

type sendMessageRequest struct {
	UserID  string          `json:"user_id"`
	Text    string          `json:"text"`
	Buttons []MessageButton `json:"buttons,omitempty"`
}

// SendMessage отправляет текстовое сообщение с опциональными кнопками
// пользователю MAX. Формат запроса упрощён под MVP и должен быть сверен
// с актуальной документацией MAX Bot API перед продакшен-использованием.
func (c *Client) SendMessage(ctx context.Context, maxUserID, text string, buttons []MessageButton) error {
	payload := sendMessageRequest{
		UserID:  maxUserID,
		Text:    text,
		Buttons: buttons,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	url := fmt.Sprintf("%s/messages/sendText", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send message request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("max bot api error: status=%d body=%s", resp.StatusCode, string(respBody))
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
