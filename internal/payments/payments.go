// Package payments — приём оплаты через магазин студии: ЮKassa и Т-Касса.
// У каждой студии свой магазин, поэтому данные доступа передаются в каждый вызов.
package payments

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	ProviderYooKassa = "yookassa"
	ProviderTBank    = "tbank"
)

// Статусы, к которым приводятся ответы платёжных систем.
const (
	StatusPending   = "pending"
	StatusSucceeded = "succeeded"
	StatusCanceled  = "canceled"
)

var (
	ErrUnknownProvider = errors.New("unknown payment provider")
	ErrRejected        = errors.New("payment provider rejected the request")
	ErrBadSignature    = errors.New("notification signature mismatch")
)

// Credentials — магазин студии: shopId и секретный ключ ЮKassa или
// TerminalKey и пароль терминала Т-Кассы.
type Credentials struct {
	Provider string
	Account  string
	Secret   string
}

type CreateRequest struct {
	OrderID         string // наш id платежа
	Amount          int    // копейки
	Description     string
	ReturnURL       string
	NotificationURL string // для Т-Кассы передаётся в каждом платеже
}

type Created struct {
	ProviderID      string
	ConfirmationURL string
}

type State struct {
	Status string
	Amount int // копейки
}

// Provider — общий интерфейс платёжных систем.
type Provider interface {
	Create(ctx context.Context, c Credentials, r CreateRequest) (*Created, error)
	Status(ctx context.Context, c Credentials, providerID string) (*State, error)
	// Check проверяет, что данные магазина подходят (если система это позволяет).
	Check(ctx context.Context, c Credentials) error
}

// Gateway выбирает реализацию по имени платёжной системы.
type Gateway struct {
	providers map[string]Provider
}

func NewGateway(yooBaseURL, tbankBaseURL string) *Gateway {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	return &Gateway{providers: map[string]Provider{
		ProviderYooKassa: &YooKassa{BaseURL: yooBaseURL, HTTP: httpClient},
		ProviderTBank:    &TBank{BaseURL: tbankBaseURL, HTTP: httpClient},
	}}
}

func (g *Gateway) Get(name string) (Provider, error) {
	if p, ok := g.providers[name]; ok {
		return p, nil
	}
	return nil, ErrUnknownProvider
}

func ValidProvider(name string) bool {
	return name == ProviderYooKassa || name == ProviderTBank
}
