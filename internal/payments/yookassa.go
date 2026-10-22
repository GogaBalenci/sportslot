package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// YooKassa — API v3: https://yookassa.ru/developers/api.
// Авторизация — HTTP Basic: shopId и секретный ключ магазина.
type YooKassa struct {
	BaseURL string
	HTTP    *http.Client
}

type yooAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type yooPayment struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Amount       yooAmount `json:"amount"`
	Confirmation struct {
		Type string `json:"type"`
		URL  string `json:"confirmation_url"`
	} `json:"confirmation"`
}

type yooError struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

func (y *YooKassa) Create(ctx context.Context, c Credentials, r CreateRequest) (*Created, error) {
	body := map[string]interface{}{
		"amount":       yooAmount{Value: rubles(r.Amount), Currency: "RUB"},
		"capture":      true,
		"confirmation": map[string]string{"type": "redirect", "return_url": r.ReturnURL},
		"description":  truncate(r.Description, 128),
		"metadata":     map[string]string{"order_id": r.OrderID},
	}
	var p yooPayment
	// Ключ идемпотентности — наш id платежа: повтор запроса не создаст второй платёж.
	if err := y.do(ctx, c, http.MethodPost, "/v3/payments", r.OrderID, body, &p); err != nil {
		return nil, err
	}
	if p.ID == "" || p.Confirmation.URL == "" {
		return nil, fmt.Errorf("%w: empty payment id or confirmation url", ErrRejected)
	}
	return &Created{ProviderID: p.ID, ConfirmationURL: p.Confirmation.URL}, nil
}

func (y *YooKassa) Status(ctx context.Context, c Credentials, providerID string) (*State, error) {
	var p yooPayment
	if err := y.do(ctx, c, http.MethodGet, "/v3/payments/"+url.PathEscape(providerID), "", nil, &p); err != nil {
		return nil, err
	}
	st := &State{Amount: kopecks(p.Amount.Value)}
	switch p.Status {
	case "succeeded":
		st.Status = StatusSucceeded
	case "canceled":
		st.Status = StatusCanceled
	default: // pending, waiting_for_capture
		st.Status = StatusPending
	}
	return st, nil
}

// Check запрашивает настройки магазина (GET /v3/me): неверный ключ даст 401.
func (y *YooKassa) Check(ctx context.Context, c Credentials) error {
	var me map[string]interface{}
	return y.do(ctx, c, http.MethodGet, "/v3/me", "", nil, &me)
}

func (y *YooKassa) do(ctx context.Context, c Credentials, method, path, idempotence string, in, out interface{}) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(y.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.Account, c.Secret)
	req.Header.Set("Content-Type", "application/json")
	if idempotence != "" {
		req.Header.Set("Idempotence-Key", idempotence)
	}
	resp, err := y.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("yookassa: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e yooError
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("%w: yookassa %d %s %s", ErrRejected, resp.StatusCode, e.Code, e.Description)
	}
	return json.Unmarshal(raw, out)
}

func rubles(kop int) string { return fmt.Sprintf("%d.%02d", kop/100, kop%100) }

func kopecks(value string) int {
	whole, frac, _ := strings.Cut(value, ".")
	r, _ := strconv.Atoi(whole)
	frac = (frac + "00")[:2]
	k, _ := strconv.Atoi(frac)
	return r*100 + k
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
