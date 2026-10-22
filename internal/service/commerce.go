package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"sportslot/internal/model"
	"sportslot/internal/payments"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/secret"
)

// CommerceService — абонементы и онлайн-оплата через магазин студии.
type CommerceService struct {
	store    *postgres.Store
	gateway  *payments.Gateway
	box      *secret.Box
	studio   *StudioService
	client   ClientPresenter
	deepLink func(param string) string
	baseURL  string // публичный адрес API для уведомлений платёжных систем
	now      func() time.Time
}

func NewCommerceService(store *postgres.Store, gateway *payments.Gateway, box *secret.Box, studio *StudioService) *CommerceService {
	return &CommerceService{store: store, gateway: gateway, box: box, studio: studio, now: time.Now,
		deepLink: func(param string) string { return param }}
}

func (c *CommerceService) SetClientPresenter(p ClientPresenter)    { c.client = p }
func (c *CommerceService) SetDeepLink(f func(param string) string) { c.deepLink = f }
func (c *CommerceService) SetPublicURL(base string)                { c.baseURL = strings.TrimRight(base, "/") }

// --- Настройки студии ---

// SettingsView — настройки для кабинета: ключ магазина не отдаётся, только его окончание.
type SettingsView struct {
	*model.StudioSettings
	SecretSaved     bool   `json:"secret_saved"`
	EncryptionReady bool   `json:"encryption_ready"`
	YooKassaWebhook string `json:"yookassa_webhook"`
	TBankWebhook    string `json:"tbank_webhook"`
}

func (c *CommerceService) view(st *model.StudioSettings) *SettingsView {
	return &SettingsView{StudioSettings: st, SecretSaved: len(st.PaymentSecret) > 0, EncryptionReady: c.box.Ready(),
		YooKassaWebhook: c.baseURL + "/api/v1/payments/yookassa/" + st.VenueID,
		TBankWebhook:    c.baseURL + "/api/v1/payments/tbank/" + st.VenueID}
}

func (c *CommerceService) Settings(ctx context.Context, a Access, venueID string) (*SettingsView, error) {
	if _, err := c.studio.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	st, err := c.store.GetSettings(ctx, venueID)
	if err != nil {
		return nil, err
	}
	return c.view(st), nil
}

// SettingsInput — изменения настроек; пустые поля не трогаются.
type SettingsInput struct {
	ShowReviews     *bool   `json:"show_reviews"`
	DailyDigest     *bool   `json:"daily_digest"`
	Provider        *string `json:"payment_provider"`
	Account         *string `json:"payment_account"`
	Secret          *string `json:"payment_secret"`
	PaymentsEnabled *bool   `json:"payments_enabled"`
	TrialOnline     *bool   `json:"trial_online"`
}

func (c *CommerceService) SaveSettings(ctx context.Context, a Access, venueID string, in SettingsInput) (*SettingsView, error) {
	v, err := c.studio.venue(ctx, a, venueID)
	if err != nil {
		return nil, err
	}
	if a.Demo && v.IsDemo() {
		return nil, ErrDemoReadOnly
	}
	st, err := c.store.GetSettings(ctx, venueID)
	if err != nil {
		return nil, err
	}
	setBool := func(dst *bool, src *bool) {
		if src != nil {
			*dst = *src
		}
	}
	setBool(&st.ShowReviews, in.ShowReviews)
	setBool(&st.DailyDigest, in.DailyDigest)
	setBool(&st.TrialOnline, in.TrialOnline)
	if in.Provider != nil {
		p := strings.TrimSpace(*in.Provider)
		if p != "" && !payments.ValidProvider(p) {
			return nil, ErrInvalidRequest
		}
		if p != st.PaymentProvider {
			// Другая платёжная система — старый ключ к ней не подходит.
			st.PaymentSecret, st.PaymentSecretHint = nil, ""
		}
		st.PaymentProvider = p
	}
	if in.Account != nil {
		st.PaymentAccount = strings.TrimSpace(*in.Account)
		if utf8.RuneCountInString(st.PaymentAccount) > 128 {
			return nil, ErrInvalidRequest
		}
	}
	if in.Secret != nil && strings.TrimSpace(*in.Secret) != "" {
		plain := strings.TrimSpace(*in.Secret)
		if len(plain) > 256 {
			return nil, ErrInvalidRequest
		}
		sealed, err := c.box.Seal(plain)
		if errors.Is(err, secret.ErrNoKey) {
			return nil, ErrNoSecretKey
		}
		if err != nil {
			return nil, err
		}
		st.PaymentSecret, st.PaymentSecretHint = sealed, hint(plain)
	}
	setBool(&st.PaymentsEnabled, in.PaymentsEnabled)
	if st.PaymentsEnabled && (st.PaymentProvider == "" || st.PaymentAccount == "" || len(st.PaymentSecret) == 0) {
		return nil, ErrInvalidRequest
	}
	if err := c.store.SaveSettings(ctx, st); err != nil {
		return nil, err
	}
	s := c.studio
	s.event(ctx, a, "studio_settings", map[string]interface{}{"venue_id": venueID, "payments": st.PaymentsEnabled,
		"provider": st.PaymentProvider, "reviews": st.ShowReviews})
	return c.view(st), nil
}

func hint(secret string) string {
	// Окончание показываем только у длинных ключей, чтобы не раскрыть короткий пароль.
	r := []rune(secret)
	if len(r) < 12 {
		return "••••"
	}
	return "••" + string(r[len(r)-4:])
}

// CheckPayments проверяет ключи магазина запросом к платёжной системе.
func (c *CommerceService) CheckPayments(ctx context.Context, a Access, venueID string) error {
	if _, err := c.studio.venue(ctx, a, venueID); err != nil {
		return err
	}
	st, err := c.store.GetSettings(ctx, venueID)
	if err != nil {
		return err
	}
	creds, provider, err := c.credentials(st)
	if err != nil {
		return err
	}
	if err := provider.Check(ctx, creds); err != nil {
		log.Printf("payments check %s: %v", venueID, err)
		return ErrPaymentFailed
	}
	return nil
}

func (c *CommerceService) credentials(st *model.StudioSettings) (payments.Credentials, payments.Provider, error) {
	if st.PaymentProvider == "" || st.PaymentAccount == "" || len(st.PaymentSecret) == 0 {
		return payments.Credentials{}, nil, ErrPaymentsOff
	}
	plain, err := c.box.Open(st.PaymentSecret)
	if errors.Is(err, secret.ErrNoKey) {
		return payments.Credentials{}, nil, ErrNoSecretKey
	}
	if err != nil {
		return payments.Credentials{}, nil, fmt.Errorf("payment secret of %s: %w", st.VenueID, err)
	}
	provider, err := c.gateway.Get(st.PaymentProvider)
	if err != nil {
		return payments.Credentials{}, nil, err
	}
	return payments.Credentials{Provider: st.PaymentProvider, Account: st.PaymentAccount, Secret: plain}, provider, nil
}

// --- Абонементы в кабинете ---

// StudioCommerce — абонементы студии, заявки клиентов и оплаты.
type StudioCommerce struct {
	Passes     []model.Pass        `json:"passes"`
	UserPasses []model.UserPass    `json:"user_passes"`
	Payments   []model.Payment     `json:"payments"`
	Month      model.PaymentTotals `json:"month"`
}

func (c *CommerceService) StudioCommerce(ctx context.Context, a Access, venueID string) (*StudioCommerce, error) {
	if _, err := c.studio.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	out := &StudioCommerce{}
	var err error
	if out.Passes, err = c.store.ListPasses(ctx, venueID, false); err != nil {
		return nil, err
	}
	if out.UserPasses, err = c.store.VenueUserPasses(ctx, venueID); err != nil {
		return nil, err
	}
	if out.Payments, err = c.store.VenuePayments(ctx, venueID, 30); err != nil {
		return nil, err
	}
	now := c.now()
	out.Month, err = c.store.PaymentTotals(ctx, []string{venueID}, now.AddDate(0, 0, -30), now)
	return out, err
}

func (c *CommerceService) SavePass(ctx context.Context, a Access, venueID string, p model.Pass) (*model.Pass, error) {
	if _, err := c.studio.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	p.Title = strings.TrimSpace(p.Title)
	n := utf8.RuneCountInString(p.Title)
	if n < 2 || n > 120 || p.Visits < 1 || p.Visits > 100 || p.ValidDays < 1 || p.ValidDays > 365 || p.Price < 1 || p.Price > 1_000_000 {
		return nil, ErrInvalidRequest
	}
	p.VenueID = venueID
	saved, err := c.store.SavePass(ctx, &p)
	if err == nil {
		c.studio.event(ctx, a, "pass_saved", map[string]interface{}{"pass_id": saved.ID, "price": saved.Price})
	}
	return saved, err
}

func (c *CommerceService) studioUserPass(ctx context.Context, a Access, venueID, id string) (*model.UserPass, error) {
	if _, err := c.studio.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	up, err := c.store.GetUserPass(ctx, id)
	if err != nil {
		return nil, err
	}
	if up.VenueID != venueID {
		return nil, ErrNotFound
	}
	return up, nil
}

// ConfirmPass — студия получила оплату на месте и включает абонемент.
func (c *CommerceService) ConfirmPass(ctx context.Context, a Access, venueID, id string) (*model.UserPass, error) {
	up, err := c.studioUserPass(ctx, a, venueID, id)
	if err != nil {
		return nil, err
	}
	changed, err := c.store.ActivateUserPass(ctx, id, "studio", c.now())
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, ErrNotActive
	}
	up, err = c.store.GetUserPass(ctx, id)
	if err == nil && c.client != nil {
		c.async(func(ctx context.Context) error { return c.client.PassActivated(ctx, up) })
	}
	return up, err
}

func (c *CommerceService) DeclinePass(ctx context.Context, a Access, venueID, id string) error {
	up, err := c.studioUserPass(ctx, a, venueID, id)
	if err != nil {
		return err
	}
	changed, err := c.store.CancelUserPass(ctx, id)
	if err != nil {
		return err
	}
	if !changed {
		return ErrNotActive
	}
	if c.client != nil {
		c.async(func(ctx context.Context) error { return c.client.PassDeclined(ctx, up) })
	}
	return nil
}

func (c *CommerceService) Reviews(ctx context.Context, a Access, venueID string) (*model.ReviewSummary, error) {
	if _, err := c.studio.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	return c.store.VenueReviews(ctx, venueID, 50, true)
}

// --- Клиент: абонементы и оплата ---

func (c *CommerceService) MyPasses(ctx context.Context, maxUserID, name string) ([]model.UserPass, error) {
	user, err := c.store.EnsureUser(ctx, maxUserID, name)
	if err != nil {
		return nil, err
	}
	return c.store.UserPassesOf(ctx, user.ID)
}

// PassOrder — заявка на абонемент; Payment есть, если студия принимает оплату онлайн.
type PassOrder struct {
	UserPass *model.UserPass `json:"user_pass"`
	Payment  *model.Payment  `json:"payment,omitempty"`
}

func (c *CommerceService) RequestPass(ctx context.Context, maxUserID, name, passID string) (*PassOrder, error) {
	pass, err := c.store.GetPass(ctx, passID)
	if err != nil {
		return nil, err
	}
	if !pass.Active {
		return nil, ErrNotFound
	}
	venue, err := c.store.GetVenue(ctx, pass.VenueID)
	if err != nil {
		return nil, err
	}
	if venue.BookingMode != model.BookingModeInstant {
		return nil, ErrExternalVenue
	}
	user, err := c.store.EnsureUser(ctx, maxUserID, name)
	if err != nil {
		return nil, err
	}
	up, err := c.store.RequestUserPass(ctx, pass, user.ID)
	if err != nil {
		return nil, err
	}
	order := &PassOrder{UserPass: up}
	settings, err := c.store.GetSettings(ctx, venue.ID)
	if err != nil {
		return nil, err
	}
	c.studio.logEvent(ctx, maxUserID, "pass_requested", map[string]interface{}{"pass_id": passID, "online": settings.PaymentsReady()})
	if settings.PaymentsReady() {
		order.Payment, err = c.pay(ctx, settings, venue, user.ID, model.PaymentKindPass, "", up.ID, pass.Price*100,
			fmt.Sprintf("Абонемент «%s», %s", pass.Title, venue.Name))
		return order, err
	}
	if c.studio.presenter != nil {
		c.studio.notifyAdmins(venue.ID, func(ctx context.Context, admin string) error {
			return c.studio.presenter.StudioPassRequest(ctx, admin, up)
		})
	}
	return order, nil
}

// PayPass — повторная попытка оплатить заявку на абонемент.
func (c *CommerceService) PayPass(ctx context.Context, maxUserID, userPassID string) (*model.Payment, error) {
	up, err := c.store.GetUserPass(ctx, userPassID)
	if err != nil {
		return nil, err
	}
	if up.MaxUserID != maxUserID {
		return nil, ErrForbidden
	}
	if up.Status != model.UserPassRequested {
		return nil, ErrNothingToPay
	}
	settings, venue, err := c.ready(ctx, up.VenueID)
	if err != nil {
		return nil, err
	}
	return c.pay(ctx, settings, venue, up.UserID, model.PaymentKindPass, "", up.ID, up.Price*100,
		fmt.Sprintf("Абонемент «%s», %s", up.Title, venue.Name))
}

func (c *CommerceService) CancelPassRequest(ctx context.Context, maxUserID, userPassID string) error {
	up, err := c.store.GetUserPass(ctx, userPassID)
	if err != nil {
		return err
	}
	if up.MaxUserID != maxUserID {
		return ErrForbidden
	}
	changed, err := c.store.CancelUserPass(ctx, userPassID)
	if err == nil && !changed {
		return ErrNotActive
	}
	return err
}

// PayTrial — оплата пробного занятия онлайн, если студия это включила.
func (c *CommerceService) PayTrial(ctx context.Context, maxUserID, bookingID string) (*model.Payment, error) {
	view, err := c.store.GetBookingView(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if view.MaxUserID != maxUserID {
		return nil, ErrForbidden
	}
	if view.Status != model.BookingStatusConfirmed {
		return nil, ErrNotActive
	}
	if view.PaidAt != nil || view.UserPassID != "" || view.Venue.TrialPrice == nil || *view.Venue.TrialPrice <= 0 {
		return nil, ErrNothingToPay
	}
	settings, venue, err := c.ready(ctx, view.Venue.ID)
	if err != nil {
		return nil, err
	}
	if !settings.TrialOnline {
		return nil, ErrPaymentsOff
	}
	return c.pay(ctx, settings, venue, view.UserID, model.PaymentKindTrial, view.ID, "", *view.Venue.TrialPrice*100,
		fmt.Sprintf("Пробное занятие «%s», %s", view.Slot.Title, venue.Name))
}

func (c *CommerceService) ready(ctx context.Context, venueID string) (*model.StudioSettings, *model.Venue, error) {
	settings, err := c.store.GetSettings(ctx, venueID)
	if err != nil {
		return nil, nil, err
	}
	if !settings.PaymentsReady() {
		return nil, nil, ErrPaymentsOff
	}
	venue, err := c.store.GetVenue(ctx, venueID)
	return settings, venue, err
}

// pay создаёт платёж у платёжной системы студии. Если недавно уже создан
// неоплаченный платёж за то же самое, возвращается его ссылка.
func (c *CommerceService) pay(ctx context.Context, settings *model.StudioSettings, venue *model.Venue,
	userID, kind, bookingID, userPassID string, amount int, description string) (*model.Payment, error) {
	if open, err := c.store.OpenPayment(ctx, kind, bookingID, userPassID); err == nil && open != nil {
		return open, nil
	}
	creds, provider, err := c.credentials(settings)
	if err != nil {
		return nil, err
	}
	p, err := c.store.CreatePayment(ctx, &model.Payment{VenueID: venue.ID, UserID: userID, Provider: settings.PaymentProvider,
		Kind: kind, BookingID: bookingID, UserPassID: userPassID, Amount: amount, Description: description})
	if err != nil {
		return nil, err
	}
	req := payments.CreateRequest{OrderID: p.ID, Amount: amount, Description: description, ReturnURL: c.deepLink("pay_" + p.ID)}
	if settings.PaymentProvider == payments.ProviderTBank && c.baseURL != "" {
		req.NotificationURL = c.baseURL + "/api/v1/payments/tbank/" + venue.ID
	}
	created, err := provider.Create(ctx, creds, req)
	if err != nil {
		log.Printf("payment %s create at %s: %v", p.ID, settings.PaymentProvider, err)
		_, _ = c.store.FinishPayment(ctx, p.ID, model.PaymentCanceled, c.now())
		return nil, ErrPaymentFailed
	}
	if err := c.store.SetPaymentProvider(ctx, p.ID, created.ProviderID, created.ConfirmationURL); err != nil {
		return nil, err
	}
	p.ProviderPaymentID, p.ConfirmationURL = created.ProviderID, created.ConfirmationURL
	c.studio.logEvent(ctx, p.MaxUserID, "payment_created", map[string]interface{}{"payment_id": p.ID, "kind": kind})
	return p, nil
}

// Payment — платёж клиента; неоплаченный перепроверяется у платёжной системы.
func (c *CommerceService) Payment(ctx context.Context, maxUserID, id string) (*model.Payment, error) {
	p, err := c.store.GetPayment(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.MaxUserID != maxUserID {
		return nil, ErrForbidden
	}
	if p.Status == model.PaymentPending && p.ProviderPaymentID != "" {
		if err := c.refresh(ctx, p); err != nil {
			log.Printf("payment %s refresh: %v", p.ID, err)
		}
		return c.store.GetPayment(ctx, id)
	}
	return p, nil
}

func (c *CommerceService) refresh(ctx context.Context, p *model.Payment) error {
	settings, err := c.store.GetSettings(ctx, p.VenueID)
	if err != nil {
		return err
	}
	if settings.PaymentProvider != p.Provider {
		return ErrPaymentsOff
	}
	creds, provider, err := c.credentials(settings)
	if err != nil {
		return err
	}
	state, err := provider.Status(ctx, creds, p.ProviderPaymentID)
	if err != nil {
		_ = c.store.TouchPayment(ctx, p.ID, c.now())
		return err
	}
	return c.apply(ctx, p, state)
}

// apply переносит статус платёжной системы в наш платёж. Побочные действия
// (отметка оплаты, включение абонемента, сообщения) выполняются один раз.
func (c *CommerceService) apply(ctx context.Context, p *model.Payment, state *payments.State) error {
	switch state.Status {
	case payments.StatusSucceeded:
		if state.Amount > 0 && state.Amount != p.Amount {
			return fmt.Errorf("payment %s: amount %d differs from %d", p.ID, state.Amount, p.Amount)
		}
		changed, err := c.store.FinishPayment(ctx, p.ID, model.PaymentSucceeded, c.now())
		if err != nil || !changed {
			return err
		}
		return c.paid(ctx, p)
	case payments.StatusCanceled:
		_, err := c.store.FinishPayment(ctx, p.ID, model.PaymentCanceled, c.now())
		return err
	}
	return c.store.TouchPayment(ctx, p.ID, c.now())
}

func (c *CommerceService) paid(ctx context.Context, p *model.Payment) error {
	now := c.now()
	paidAt := now
	p.Status, p.PaidAt = model.PaymentSucceeded, &paidAt
	var view *model.BookingView
	var up *model.UserPass
	switch p.Kind {
	case model.PaymentKindTrial:
		if err := c.store.MarkBookingPaid(ctx, p.BookingID, now); err != nil {
			return err
		}
		view, _ = c.store.GetBookingView(ctx, p.BookingID)
	case model.PaymentKindPass:
		if _, err := c.store.ActivateUserPass(ctx, p.UserPassID, "online", now); err != nil {
			return err
		}
		up, _ = c.store.GetUserPass(ctx, p.UserPassID)
	}
	c.studio.logEvent(ctx, p.MaxUserID, "payment_succeeded", map[string]interface{}{"payment_id": p.ID, "kind": p.Kind, "amount": p.Amount})
	if c.client != nil {
		c.async(func(ctx context.Context) error { return c.client.PaymentReceived(ctx, p, view, up) })
	}
	if c.studio.presenter != nil {
		if venue, err := c.store.GetVenue(ctx, p.VenueID); err == nil {
			c.studio.notifyAdmins(p.VenueID, func(ctx context.Context, admin string) error {
				return c.studio.presenter.StudioPayment(ctx, admin, p, venue)
			})
		}
	}
	return nil
}

// --- Уведомления платёжных систем ---

// YooKassaNotification — уведомление ЮKassa. Оно не подписано, поэтому статус
// платежа перепроверяется запросом к API ЮKassa с ключом магазина студии.
func (c *CommerceService) YooKassaNotification(ctx context.Context, venueID string, raw []byte) error {
	id := payments.PeekYooKassaPayment(raw)
	if id == "" {
		return ErrInvalidRequest
	}
	p, err := c.store.PaymentByProvider(ctx, payments.ProviderYooKassa, id)
	if err != nil {
		return err
	}
	if p.VenueID != venueID {
		return ErrNotFound
	}
	if p.Status != model.PaymentPending {
		return nil
	}
	return c.refresh(ctx, p)
}

// TBankNotification — уведомление Т-Кассы с подписью паролем терминала студии.
func (c *CommerceService) TBankNotification(ctx context.Context, venueID string, raw []byte) error {
	p, err := c.store.GetPayment(ctx, payments.PeekTBankOrder(raw))
	if err != nil {
		return err
	}
	if p.VenueID != venueID || p.Provider != payments.ProviderTBank {
		return ErrNotFound
	}
	settings, err := c.store.GetSettings(ctx, venueID)
	if err != nil {
		return err
	}
	creds, _, err := c.credentials(settings)
	if err != nil {
		return err
	}
	n, err := payments.ParseTBankNotification(raw, creds.Secret)
	if err != nil {
		return err
	}
	if n.TerminalKey != creds.Account || n.PaymentID != p.ProviderPaymentID {
		return payments.ErrBadSignature
	}
	if p.Status != model.PaymentPending {
		return nil
	}
	return c.apply(ctx, p, &payments.State{Status: payments.TBankStatus(n.Status), Amount: n.Amount})
}

// Tick — фоновые задачи: перепроверка неоплаченных платежей, сроки абонементов.
func (c *CommerceService) Tick(ctx context.Context, now time.Time) {
	if err := c.store.ExpirePasses(ctx, now); err != nil {
		log.Printf("commerce: expire passes: %v", err)
	}
	pending, err := c.store.PendingPayments(ctx, now)
	if err != nil {
		log.Printf("commerce: pending payments: %v", err)
	}
	for i := range pending {
		if err := c.refresh(ctx, &pending[i]); err != nil {
			log.Printf("commerce: refresh payment %s: %v", pending[i].ID, err)
		}
	}
	if err := c.store.ExpirePayments(ctx, now); err != nil {
		log.Printf("commerce: expire payments: %v", err)
	}
}

func (c *CommerceService) async(send func(ctx context.Context) error) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := send(ctx); err != nil {
			log.Printf("commerce message: %v", err)
		}
	}()
}
