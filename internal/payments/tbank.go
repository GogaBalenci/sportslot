package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// TBank — интернет-эквайринг Т-Кассы, API v2: https://developer.tbank.ru/eacq/api.
// Запросы подписываются Token: SHA-256 от значений корневых полей и пароля
// терминала, отсортированных по имени поля.
type TBank struct {
	BaseURL string
	HTTP    *http.Client
}

type tbankResponse struct {
	Success    bool        `json:"Success"`
	ErrorCode  string      `json:"ErrorCode"`
	Message    string      `json:"Message"`
	Details    string      `json:"Details"`
	Status     string      `json:"Status"`
	PaymentID  json.Number `json:"PaymentId"`
	Amount     json.Number `json:"Amount"`
	PaymentURL string      `json:"PaymentURL"`
}

func (t *TBank) Create(ctx context.Context, c Credentials, r CreateRequest) (*Created, error) {
	params := map[string]interface{}{
		"TerminalKey": c.Account,
		"Amount":      r.Amount,
		"OrderId":     r.OrderID,
		"Description": truncate(r.Description, 140),
		"SuccessURL":  r.ReturnURL,
		"FailURL":     r.ReturnURL,
	}
	if r.NotificationURL != "" {
		params["NotificationURL"] = r.NotificationURL
	}
	var resp tbankResponse
	if err := t.call(ctx, c, "Init", params, &resp); err != nil {
		return nil, err
	}
	if resp.PaymentURL == "" || resp.PaymentID == "" {
		return nil, fmt.Errorf("%w: empty PaymentId or PaymentURL", ErrRejected)
	}
	return &Created{ProviderID: resp.PaymentID.String(), ConfirmationURL: resp.PaymentURL}, nil
}

func (t *TBank) Status(ctx context.Context, c Credentials, providerID string) (*State, error) {
	var resp tbankResponse
	params := map[string]interface{}{"TerminalKey": c.Account, "PaymentId": providerID}
	if err := t.call(ctx, c, "GetState", params, &resp); err != nil {
		return nil, err
	}
	amount, _ := resp.Amount.Int64()
	return &State{Status: TBankStatus(resp.Status), Amount: int(amount)}, nil
}

// Check у Т-Кассы нет отдельного метода проверки ключей: данные проверятся
// при первом платеже, а ошибка придёт в ответе Init.
func (t *TBank) Check(ctx context.Context, c Credentials) error {
	if c.Account == "" || c.Secret == "" {
		return fmt.Errorf("%w: terminal key and password are required", ErrRejected)
	}
	return nil
}

// TBankStatus приводит статус Т-Кассы к общему виду. Платёж в одну стадию
// считается оплаченным в статусе CONFIRMED.
func TBankStatus(s string) string {
	switch s {
	case "CONFIRMED":
		return StatusSucceeded
	case "REJECTED", "CANCELED", "DEADLINE_EXPIRED", "REVERSED", "REFUNDED", "PARTIAL_REFUNDED", "AUTH_FAIL":
		return StatusCanceled
	}
	return StatusPending
}

func (t *TBank) call(ctx context.Context, c Credentials, method string, params map[string]interface{}, out *tbankResponse) error {
	params["Token"] = TBankToken(params, c.Secret)
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(t.BaseURL, "/")+"/v2/"+method, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("tbank: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%w: tbank http %d", ErrRejected, resp.StatusCode)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("tbank: decode: %w", err)
	}
	if !out.Success || (out.ErrorCode != "" && out.ErrorCode != "0") {
		return fmt.Errorf("%w: tbank %s %s %s", ErrRejected, out.ErrorCode, out.Message, out.Details)
	}
	return nil
}

// TBankToken считает подпись: корневые скалярные поля (кроме Token) плюс
// Password, сортировка по имени, склейка значений, SHA-256 в hex.
func TBankToken(params map[string]interface{}, password string) string {
	values := map[string]string{"Password": password}
	for k, v := range params {
		if k == "Token" {
			continue
		}
		if s, ok := scalar(v); ok {
			values[k] = s
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(values[k])
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func scalar(v interface{}) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case json.Number:
		return x.String(), true
	case bool:
		if x {
			return "true", true
		}
		return "false", true
	case int:
		return fmt.Sprint(x), true
	case int64:
		return fmt.Sprint(x), true
	case float64:
		return fmt.Sprint(x), true
	}
	return "", false // вложенные объекты в подпись не входят
}

// TBankNotification — уведомление Т-Кассы о статусе платежа.
type TBankNotification struct {
	TerminalKey string
	OrderID     string
	PaymentID   string
	Status      string
	Amount      int
}

// ParseTBankNotification проверяет подпись уведомления паролем терминала.
func ParseTBankNotification(raw []byte, password string) (*TBankNotification, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var params map[string]interface{}
	if err := dec.Decode(&params); err != nil {
		return nil, err
	}
	token, _ := params["Token"].(string)
	expected := TBankToken(params, password)
	if token == "" || subtle.ConstantTimeCompare([]byte(strings.ToLower(token)), []byte(expected)) != 1 {
		return nil, ErrBadSignature
	}
	n := &TBankNotification{}
	n.TerminalKey, _ = scalar(params["TerminalKey"])
	n.OrderID, _ = scalar(params["OrderId"])
	n.PaymentID, _ = scalar(params["PaymentId"])
	n.Status, _ = scalar(params["Status"])
	if a, ok := params["Amount"].(json.Number); ok {
		v, _ := a.Int64()
		n.Amount = int(v)
	}
	return n, nil
}

// PeekTBankOrder достаёт OrderId до проверки подписи, чтобы найти магазин.
func PeekTBankOrder(raw []byte) string {
	var p struct {
		OrderID string `json:"OrderId"`
	}
	_ = json.Unmarshal(raw, &p)
	return p.OrderID
}

// PeekYooKassaPayment достаёт id платежа из уведомления ЮKassa. Само
// уведомление не подписано, поэтому статус потом перепроверяется запросом к API.
func PeekYooKassaPayment(raw []byte) string {
	var n struct {
		Object struct {
			ID string `json:"id"`
		} `json:"object"`
	}
	_ = json.Unmarshal(raw, &n)
	return n.Object.ID
}
