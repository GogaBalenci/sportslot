package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
	"sportslot/internal/repository"
)

// --- Настройки студии ---

const settingsColumns = `venue_id, show_reviews, daily_digest, payment_provider, payment_account,
	payment_secret, payment_secret_hint, payments_enabled, trial_online, updated_at`

func scanSettings(row pgx.Row) (*model.StudioSettings, error) {
	var st model.StudioSettings
	err := row.Scan(&st.VenueID, &st.ShowReviews, &st.DailyDigest, &st.PaymentProvider, &st.PaymentAccount,
		&st.PaymentSecret, &st.PaymentSecretHint, &st.PaymentsEnabled, &st.TrialOnline, &st.UpdatedAt)
	return &st, err
}

// GetSettings — настройки студии; если студия их не меняла, значения по умолчанию.
func (s *Store) GetSettings(ctx context.Context, venueID string) (*model.StudioSettings, error) {
	st, err := scanSettings(s.pool.QueryRow(ctx, `SELECT `+settingsColumns+` FROM studio_settings WHERE venue_id = $1`, venueID))
	if err == pgx.ErrNoRows {
		return &model.StudioSettings{VenueID: venueID, DailyDigest: true}, nil
	}
	return st, wrap("get settings", err)
}

func (s *Store) SaveSettings(ctx context.Context, st *model.StudioSettings) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO studio_settings (`+settingsColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (venue_id) DO UPDATE SET show_reviews = EXCLUDED.show_reviews, daily_digest = EXCLUDED.daily_digest,
			payment_provider = EXCLUDED.payment_provider, payment_account = EXCLUDED.payment_account,
			payment_secret = EXCLUDED.payment_secret, payment_secret_hint = EXCLUDED.payment_secret_hint,
			payments_enabled = EXCLUDED.payments_enabled, trial_online = EXCLUDED.trial_online, updated_at = now()`,
		st.VenueID, st.ShowReviews, st.DailyDigest, st.PaymentProvider, st.PaymentAccount,
		st.PaymentSecret, st.PaymentSecretHint, st.PaymentsEnabled, st.TrialOnline)
	return wrap("save settings", err)
}

// --- Отзывы ---

// SetReview сохраняет текст отзыва к посещению, которое клиент уже оценил.
func (s *Store) SetReview(ctx context.Context, bookingID, text string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE bookings SET review = $2, reviewed_at = now()
		WHERE id = $1 AND status = 'attended' AND rating IS NOT NULL`, bookingID, text)
	if err != nil {
		return wrap("set review", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotActive
	}
	return nil
}

// VenueReviews — средняя оценка и последние отзывы. fullNames — для кабинета
// студии; в карточке для клиентов показывается только имя.
func (s *Store) VenueReviews(ctx context.Context, venueID string, limit int, fullNames bool) (*model.ReviewSummary, error) {
	sum := &model.ReviewSummary{Items: []model.Review{}}
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(avg(b.rating), 0)::float8, count(*) FROM bookings b
		JOIN slots sl ON sl.id = b.slot_id WHERE sl.venue_id = $1 AND b.rating IS NOT NULL`, venueID).Scan(&sum.Average, &sum.Count)
	if err != nil {
		return nil, wrap("review summary", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT b.rating, COALESCE(b.review, ''), COALESCE(u.name, ''), COALESCE(b.reviewed_at, b.attended_at, b.created_at)
		FROM bookings b JOIN slots sl ON sl.id = b.slot_id JOIN users u ON u.id = b.user_id
		WHERE sl.venue_id = $1 AND b.rating IS NOT NULL
		ORDER BY (b.review IS NOT NULL AND b.review <> '') DESC, COALESCE(b.reviewed_at, b.attended_at) DESC LIMIT $2`, venueID, limit)
	if err != nil {
		return nil, wrap("reviews", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r model.Review
		var rating int16
		if err := rows.Scan(&rating, &r.Text, &r.Author, &r.Date); err != nil {
			return nil, wrap("scan review", err)
		}
		r.Rating = int(rating)
		if !fullNames {
			r.Author = firstName(r.Author)
		}
		sum.Items = append(sum.Items, r)
	}
	return sum, wrap("reviews", rows.Err())
}

func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return "Клиент"
}

// PublicRatings — средние оценки студий, которые включили показ отзывов.
func (s *Store) PublicRatings(ctx context.Context, venueIDs []string) (map[string]model.ReviewSummary, error) {
	out := map[string]model.ReviewSummary{}
	if len(venueIDs) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT sl.venue_id, avg(b.rating)::float8, count(*) FROM bookings b
		JOIN slots sl ON sl.id = b.slot_id JOIN studio_settings ss ON ss.venue_id = sl.venue_id AND ss.show_reviews
		WHERE sl.venue_id = ANY($1::uuid[]) AND b.rating IS NOT NULL GROUP BY sl.venue_id`, venueIDs)
	if err != nil {
		return nil, wrap("public ratings", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var r model.ReviewSummary
		if err := rows.Scan(&id, &r.Average, &r.Count); err != nil {
			return nil, wrap("scan rating", err)
		}
		out[id] = r
	}
	return out, wrap("public ratings", rows.Err())
}

// --- Абонементы студии ---

const passColumns = `id, venue_id, title, visits, valid_days, price, active, created_at`

func scanPass(row pgx.Row) (*model.Pass, error) {
	var p model.Pass
	var visits, days int16
	err := row.Scan(&p.ID, &p.VenueID, &p.Title, &visits, &days, &p.Price, &p.Active, &p.CreatedAt)
	p.Visits, p.ValidDays = int(visits), int(days)
	return &p, err
}

func (s *Store) ListPasses(ctx context.Context, venueID string, onlyActive bool) ([]model.Pass, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+passColumns+` FROM passes WHERE venue_id = $1 AND (active OR NOT $2)
		ORDER BY active DESC, price`, venueID, onlyActive)
	if err != nil {
		return nil, wrap("list passes", err)
	}
	defer rows.Close()
	list := []model.Pass{}
	for rows.Next() {
		p, err := scanPass(rows)
		if err != nil {
			return nil, wrap("scan pass", err)
		}
		list = append(list, *p)
	}
	return list, wrap("list passes", rows.Err())
}

func (s *Store) GetPass(ctx context.Context, id string) (*model.Pass, error) {
	p, err := scanPass(s.pool.QueryRow(ctx, `SELECT `+passColumns+` FROM passes WHERE id = $1`, id))
	return p, wrap("get pass", notFound(err))
}

// SavePass создаёт абонемент или меняет его в пределах студии.
func (s *Store) SavePass(ctx context.Context, p *model.Pass) (*model.Pass, error) {
	if p.ID == "" {
		saved, err := scanPass(s.pool.QueryRow(ctx, `INSERT INTO passes (venue_id, title, visits, valid_days, price, active)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+passColumns, p.VenueID, p.Title, p.Visits, p.ValidDays, p.Price, p.Active))
		return saved, wrap("create pass", err)
	}
	saved, err := scanPass(s.pool.QueryRow(ctx, `UPDATE passes SET title = $3, visits = $4, valid_days = $5, price = $6,
		active = $7, updated_at = now() WHERE id = $1 AND venue_id = $2 RETURNING `+passColumns,
		p.ID, p.VenueID, p.Title, p.Visits, p.ValidDays, p.Price, p.Active))
	return saved, wrap("update pass", notFound(err))
}

// --- Абонементы клиентов ---

const userPassColumns = `up.id, COALESCE(up.pass_id::text, ''), up.venue_id, up.user_id, up.title, up.visits_total,
	up.visits_left, up.valid_days, up.price, up.status, up.paid_by, up.valid_until, up.created_at, up.activated_at,
	v.name, COALESCE(u.name, ''), u.max_user_id`

const userPassFrom = ` FROM user_passes up JOIN venues v ON v.id = up.venue_id JOIN users u ON u.id = up.user_id`

func scanUserPass(row pgx.Row) (*model.UserPass, error) {
	var p model.UserPass
	var total, left, days int16
	err := row.Scan(&p.ID, &p.PassID, &p.VenueID, &p.UserID, &p.Title, &total, &left, &days, &p.Price, &p.Status,
		&p.PaidBy, &p.ValidUntil, &p.CreatedAt, &p.ActivatedAt, &p.VenueName, &p.UserName, &p.MaxUserID)
	p.VisitsTotal, p.VisitsLeft, p.ValidDays = int(total), int(left), int(days)
	return &p, err
}

func (s *Store) queryUserPasses(ctx context.Context, where string, args ...interface{}) ([]model.UserPass, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userPassColumns+userPassFrom+` `+where, args...)
	if err != nil {
		return nil, wrap("user passes", err)
	}
	defer rows.Close()
	list := []model.UserPass{}
	for rows.Next() {
		p, err := scanUserPass(rows)
		if err != nil {
			return nil, wrap("scan user pass", err)
		}
		list = append(list, *p)
	}
	return list, wrap("user passes", rows.Err())
}

func (s *Store) GetUserPass(ctx context.Context, id string) (*model.UserPass, error) {
	p, err := scanUserPass(s.pool.QueryRow(ctx, `SELECT `+userPassColumns+userPassFrom+` WHERE up.id = $1`, id))
	return p, wrap("get user pass", notFound(err))
}

// RequestUserPass — клиент выбрал абонемент: заявка ждёт оплаты или подтверждения студии.
// Повторная заявка на тот же абонемент возвращает уже открытую.
func (s *Store) RequestUserPass(ctx context.Context, pass *model.Pass, userID string) (*model.UserPass, bool, error) {
	var id string
	created := false
	err := s.pool.QueryRow(ctx, `SELECT id FROM user_passes WHERE user_id = $1 AND pass_id = $2 AND status = 'requested'`,
		userID, pass.ID).Scan(&id)
	if err == pgx.ErrNoRows {
		created = true
		err = s.pool.QueryRow(ctx, `INSERT INTO user_passes (pass_id, venue_id, user_id, title, visits_total, visits_left, valid_days, price)
			VALUES ($1, $2, $3, $4, $5, $5, $6, $7) RETURNING id`,
			pass.ID, pass.VenueID, userID, pass.Title, pass.Visits, pass.ValidDays, pass.Price).Scan(&id)
	}
	if err != nil {
		return nil, false, wrap("request pass", err)
	}
	up, err := s.GetUserPass(ctx, id)
	return up, created, err
}

// ActivateUserPass включает абонемент после оплаты. changed=false — он уже был активирован.
func (s *Store) ActivateUserPass(ctx context.Context, id, paidBy string, now time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE user_passes SET status = 'active', paid_by = $2, activated_at = $3,
		visits_left = visits_total, valid_until = $3::timestamptz + make_interval(days => valid_days)
		WHERE id = $1 AND status = 'requested'`, id, paidBy, now)
	return tag.RowsAffected() > 0, wrap("activate pass", err)
}

func (s *Store) CancelUserPass(ctx context.Context, id string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE user_passes SET status = 'cancelled' WHERE id = $1 AND status = 'requested'`, id)
	return tag.RowsAffected() > 0, wrap("cancel pass", err)
}

func (s *Store) UserPassesOf(ctx context.Context, userID string) ([]model.UserPass, error) {
	return s.queryUserPasses(ctx, `WHERE up.user_id = $1 AND up.status IN ('requested', 'active')
		ORDER BY up.status = 'active' DESC, up.created_at DESC`, userID)
}

func (s *Store) VenueUserPasses(ctx context.Context, venueID string) ([]model.UserPass, error) {
	return s.queryUserPasses(ctx, `WHERE up.venue_id = $1 AND (up.status IN ('requested', 'active')
		OR up.created_at > now() - interval '30 days') ORDER BY up.status = 'requested' DESC, up.created_at DESC LIMIT 200`, venueID)
}

// ActiveUserPass — действующий абонемент клиента в студии с остатком визитов.
func (s *Store) ActiveUserPass(ctx context.Context, userID, venueID string) (*model.UserPass, error) {
	p, err := scanUserPass(s.pool.QueryRow(ctx, `SELECT `+userPassColumns+userPassFrom+`
		WHERE up.user_id = $1 AND up.venue_id = $2 AND up.status = 'active' AND up.visits_left > 0 AND up.valid_until > now()
		ORDER BY up.valid_until LIMIT 1`, userID, venueID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, wrap("active pass", err)
}

func (s *Store) ExpirePasses(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE user_passes SET status = 'expired' WHERE status = 'active' AND valid_until < $1`, now)
	return wrap("expire passes", err)
}

// usePassVisit списывает визит из действующего абонемента студии.
func usePassVisit(ctx context.Context, tx pgx.Tx, b *model.Booking, venueID string, startAt time.Time) error {
	var passID string
	err := tx.QueryRow(ctx, `SELECT id FROM user_passes WHERE user_id = $1 AND venue_id = $2 AND status = 'active'
		AND visits_left > 0 AND valid_until >= $3 ORDER BY valid_until LIMIT 1 FOR UPDATE`, b.UserID, venueID, startAt).Scan(&passID)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return wrap("find pass", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE user_passes SET visits_left = visits_left - 1,
		status = CASE WHEN visits_left - 1 = 0 THEN 'used' ELSE status END WHERE id = $1`, passID); err != nil {
		return wrap("use pass visit", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bookings SET user_pass_id = $2 WHERE id = $1`, b.ID, passID); err != nil {
		return wrap("link pass", err)
	}
	b.UserPassID = passID
	return nil
}

// refundPassVisit возвращает визит, если отменённая запись была по абонементу.
func refundPassVisit(ctx context.Context, tx pgx.Tx, bookingID string) error {
	_, err := tx.Exec(ctx, `UPDATE user_passes up SET visits_left = LEAST(up.visits_left + 1, up.visits_total),
		status = CASE WHEN up.status = 'used' AND up.valid_until > now() THEN 'active' ELSE up.status END
		FROM bookings b WHERE b.id = $1 AND b.user_pass_id = up.id AND up.status IN ('active', 'used')`, bookingID)
	return wrap("refund pass visit", err)
}

// --- Место для друга ---

func guestToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AddGuest бронирует второе место на то же занятие для друга. Если место уже
// есть (принятое или нет), возвращает его.
func (s *Store) AddGuest(ctx context.Context, hostBookingID string) (*model.Booking, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	host, err := scanBooking(tx.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings b WHERE b.id = $1 FOR UPDATE`, hostBookingID))
	if err != nil {
		return nil, wrap("lock host booking", notFound(err))
	}
	if host.Status != model.BookingStatusConfirmed || host.GuestToken != "" {
		return nil, repository.ErrNotActive
	}
	existing, err := scanBooking(tx.QueryRow(ctx, `SELECT `+bookingColumns+` FROM bookings b
		WHERE b.host_booking_id = $1 AND b.status = 'confirmed' LIMIT 1`, hostBookingID))
	if err == nil {
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, wrap("find guest", err)
	}
	token, err := guestToken()
	if err != nil {
		return nil, err
	}
	guest, err := bookInTx(ctx, tx, bookOpts{userID: host.UserID, slotID: host.SlotID, channel: host.SourceChannel,
		guestToken: token, hostBookingID: host.ID})
	if err != nil {
		return nil, err
	}
	return guest, wrap("commit", tx.Commit(ctx))
}

// GuestOf — место для друга, забронированное к записи пригласившего.
func (s *Store) GuestOf(ctx context.Context, hostBookingID string) (*model.BookingView, error) {
	views, err := s.queryViews(ctx, `WHERE b.host_booking_id = $1 AND b.status <> 'cancelled' ORDER BY b.created_at DESC LIMIT 1`, hostBookingID)
	if err != nil || len(views) == 0 {
		return nil, err
	}
	return &views[0], nil
}

// ClaimGuest передаёт место другу, который открыл ссылку-приглашение.
func (s *Store) ClaimGuest(ctx context.Context, token, userID string) (*model.Booking, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var id, hostUser string
	var startAt time.Time
	err = tx.QueryRow(ctx, `SELECT b.id, b.user_id, s.start_at FROM bookings b JOIN slots s ON s.id = b.slot_id
		WHERE b.guest_token = $1 AND b.status = 'confirmed' FOR UPDATE OF b`, token).Scan(&id, &hostUser, &startAt)
	if err != nil {
		return nil, wrap("find guest place", notFound(err))
	}
	if hostUser == userID {
		return nil, repository.ErrOwnInvite
	}
	if !startAt.After(time.Now()) {
		return nil, repository.ErrSlotStarted
	}
	b, err := scanBooking(tx.QueryRow(ctx, `UPDATE bookings b SET user_id = $2, guest_token = NULL
		WHERE b.id = $1 RETURNING `+bookingColumns, id, userID))
	if err != nil {
		if isUniqueViolation(err, "uq_bookings_active_user_slot") {
			return nil, repository.ErrAlreadyBooked
		}
		return nil, wrap("claim guest place", err)
	}
	return b, wrap("commit", tx.Commit(ctx))
}

// --- Платежи ---

const paymentColumns = `p.id, p.venue_id, p.user_id, p.provider, p.kind, COALESCE(p.booking_id::text, ''),
	COALESCE(p.user_pass_id::text, ''), p.amount, p.description, p.status, COALESCE(p.provider_payment_id, ''),
	p.confirmation_url, p.created_at, p.paid_at, COALESCE(u.name, ''), u.max_user_id`

const paymentFrom = ` FROM payments p JOIN users u ON u.id = p.user_id`

func scanPayment(row pgx.Row) (*model.Payment, error) {
	var p model.Payment
	err := row.Scan(&p.ID, &p.VenueID, &p.UserID, &p.Provider, &p.Kind, &p.BookingID, &p.UserPassID, &p.Amount,
		&p.Description, &p.Status, &p.ProviderPaymentID, &p.ConfirmationURL, &p.CreatedAt, &p.PaidAt, &p.UserName, &p.MaxUserID)
	return &p, err
}

func (s *Store) queryPayments(ctx context.Context, where string, args ...interface{}) ([]model.Payment, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+paymentColumns+paymentFrom+` `+where, args...)
	if err != nil {
		return nil, wrap("payments", err)
	}
	defer rows.Close()
	list := []model.Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, wrap("scan payment", err)
		}
		list = append(list, *p)
	}
	return list, wrap("payments", rows.Err())
}

func (s *Store) CreatePayment(ctx context.Context, p *model.Payment) (*model.Payment, error) {
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO payments (venue_id, user_id, provider, kind, booking_id, user_pass_id, amount, description)
		VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, NULLIF($6, '')::uuid, $7, $8) RETURNING id`,
		p.VenueID, p.UserID, p.Provider, p.Kind, p.BookingID, p.UserPassID, p.Amount, p.Description).Scan(&id)
	if err != nil {
		return nil, wrap("create payment", err)
	}
	return s.GetPayment(ctx, id)
}

func (s *Store) SetPaymentProvider(ctx context.Context, id, providerID, url string) error {
	_, err := s.pool.Exec(ctx, `UPDATE payments SET provider_payment_id = $2, confirmation_url = $3 WHERE id = $1`, id, providerID, url)
	return wrap("set provider payment", err)
}

func (s *Store) GetPayment(ctx context.Context, id string) (*model.Payment, error) {
	p, err := scanPayment(s.pool.QueryRow(ctx, `SELECT `+paymentColumns+paymentFrom+` WHERE p.id = $1`, id))
	return p, wrap("get payment", notFound(err))
}

func (s *Store) PaymentByProvider(ctx context.Context, provider, providerID string) (*model.Payment, error) {
	p, err := scanPayment(s.pool.QueryRow(ctx, `SELECT `+paymentColumns+paymentFrom+`
		WHERE p.provider = $1 AND p.provider_payment_id = $2`, provider, providerID))
	return p, wrap("payment by provider id", notFound(err))
}

// OpenPayment — неоплаченный платёж за то же самое, созданный недавно: его ссылку можно отдать снова.
func (s *Store) OpenPayment(ctx context.Context, kind, bookingID, userPassID string) (*model.Payment, error) {
	p, err := scanPayment(s.pool.QueryRow(ctx, `SELECT `+paymentColumns+paymentFrom+`
		WHERE p.kind = $1 AND p.status = 'pending' AND p.confirmation_url <> '' AND p.created_at > now() - interval '30 minutes'
		AND COALESCE(p.booking_id::text, '') = $2 AND COALESCE(p.user_pass_id::text, '') = $3
		ORDER BY p.created_at DESC LIMIT 1`, kind, bookingID, userPassID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, wrap("open payment", err)
}

// FinishPayment переводит платёж из pending в итоговый статус. changed=false —
// статус уже был выставлен раньше (повторное уведомление), побочные действия не нужны.
func (s *Store) FinishPayment(ctx context.Context, id, status string, at time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE payments SET status = $2::text,
		paid_at = CASE WHEN $2::text = 'succeeded' THEN $3::timestamptz ELSE NULL END, checked_at = $3
		WHERE id = $1 AND status = 'pending'`, id, status, at)
	return tag.RowsAffected() > 0, wrap("finish payment", err)
}

func (s *Store) TouchPayment(ctx context.Context, id string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE payments SET checked_at = $2 WHERE id = $1`, id, at)
	return wrap("touch payment", err)
}

// PendingPayments — неоплаченные платежи для перепроверки статуса у платёжной системы.
func (s *Store) PendingPayments(ctx context.Context, now time.Time) ([]model.Payment, error) {
	return s.queryPayments(ctx, `WHERE p.status = 'pending' AND p.provider_payment_id IS NOT NULL
		AND p.created_at > $1::timestamptz - interval '2 hours'
		AND (p.checked_at IS NULL OR p.checked_at < $1::timestamptz - interval '1 minute')
		ORDER BY p.created_at LIMIT 50`, now)
}

// ExpirePayments закрывает платежи, которые не оплатили за два часа.
func (s *Store) ExpirePayments(ctx context.Context, now time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE payments SET status = 'canceled' WHERE status = 'pending'
		AND created_at < $1::timestamptz - interval '2 hours'`, now)
	return wrap("expire payments", err)
}

func (s *Store) VenuePayments(ctx context.Context, venueID string, limit int) ([]model.Payment, error) {
	return s.queryPayments(ctx, `WHERE p.venue_id = $1 AND p.status = 'succeeded' ORDER BY p.paid_at DESC LIMIT $2`, venueID, limit)
}

func (s *Store) PaymentTotals(ctx context.Context, venueIDs []string, from, to time.Time) (model.PaymentTotals, error) {
	var t model.PaymentTotals
	err := s.pool.QueryRow(ctx, `SELECT count(*), COALESCE(sum(amount), 0) / 100 FROM payments
		WHERE venue_id = ANY($1::uuid[]) AND status = 'succeeded' AND paid_at >= $2 AND paid_at < $3`,
		venueIDs, from, to).Scan(&t.Count, &t.Amount)
	return t, wrap("payment totals", err)
}

func (s *Store) MarkBookingPaid(ctx context.Context, bookingID string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE bookings SET paid_at = $2 WHERE id = $1 AND paid_at IS NULL`, bookingID, at)
	return wrap("mark booking paid", err)
}

// --- Сообщения без ответа и время ответа студии ---

// Waiting — клиент написал студии и ждёт ответа с момента Since.
type Waiting struct {
	VenueID, UserID, MaxUserID, UserName string
	Since                                time.Time
	LastMessageID                        int64
	StudioNudged, ClientNotified         bool
}

// WaitingThreads — переписки, где после последнего ответа студии есть сообщения клиента.
func (s *Store) WaitingThreads(ctx context.Context, olderThan, newerThan time.Time, all bool) ([]Waiting, error) {
	rows, err := s.pool.Query(ctx, `WITH last_studio AS (
			SELECT venue_id, user_id, max(created_at) AS at FROM studio_messages WHERE author = 'studio' GROUP BY venue_id, user_id
		), waits AS (
			SELECT m.venue_id, m.user_id, min(m.created_at) AS since, max(m.id) AS last_id,
				bool_or(m.studio_nudged_at IS NOT NULL) AS nudged, bool_or(m.client_notified_at IS NOT NULL) AS notified
			FROM studio_messages m
			LEFT JOIN last_studio ls ON ls.venue_id = m.venue_id AND ls.user_id = m.user_id
			WHERE m.author = 'client' AND (ls.at IS NULL OR m.created_at > ls.at)
			GROUP BY m.venue_id, m.user_id
		)
		SELECT w.venue_id, w.user_id, u.max_user_id, COALESCE(u.name, ''), w.since, w.last_id, w.nudged, w.notified
		FROM waits w JOIN users u ON u.id = w.user_id
		WHERE w.since < $1 AND w.since > $2 AND ($3 OR NOT (w.nudged AND w.notified))`, olderThan, newerThan, all)
	if err != nil {
		return nil, wrap("waiting threads", err)
	}
	defer rows.Close()
	var list []Waiting
	for rows.Next() {
		var w Waiting
		if err := rows.Scan(&w.VenueID, &w.UserID, &w.MaxUserID, &w.UserName, &w.Since, &w.LastMessageID,
			&w.StudioNudged, &w.ClientNotified); err != nil {
			return nil, wrap("scan waiting", err)
		}
		list = append(list, w)
	}
	return list, wrap("waiting threads", rows.Err())
}

func (s *Store) MarkWaitingNudged(ctx context.Context, messageID int64, client bool) error {
	column := "studio_nudged_at"
	if client {
		column = "client_notified_at"
	}
	_, err := s.pool.Exec(ctx, `UPDATE studio_messages SET `+column+` = now() WHERE id = $1`, messageID)
	return wrap("mark nudged", err)
}

// ResponseStats — медиана времени первого ответа студии и доля ответов в течение часа.
func (s *Store) ResponseStats(ctx context.Context, venueID string, since time.Time) (model.ResponseStats, error) {
	var r model.ResponseStats
	err := s.pool.QueryRow(ctx, `WITH replies AS (
			SELECT rep.id, min(c.created_at) AS asked_at, rep.created_at AS replied_at
			FROM studio_messages c
			JOIN LATERAL (
				SELECT id, created_at FROM studio_messages r
				WHERE r.venue_id = c.venue_id AND r.user_id = c.user_id AND r.author = 'studio' AND r.created_at > c.created_at
				ORDER BY r.created_at LIMIT 1
			) rep ON true
			WHERE c.venue_id = $1 AND c.author = 'client' AND c.created_at >= $2
			GROUP BY rep.id, rep.created_at
		)
		SELECT count(*),
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY extract(epoch FROM replied_at - asked_at)), 0) / 60,
			count(*) FILTER (WHERE replied_at - asked_at <= interval '1 hour')
		FROM replies`, venueID, since).Scan(&r.Answered, &r.MedianMinutes, &r.WithinHour)
	if err != nil {
		return r, wrap("response stats", err)
	}
	waiting, err := s.WaitingThreads(ctx, time.Now(), since, true)
	if err == nil {
		for _, w := range waiting {
			if w.VenueID == venueID {
				r.Waiting++
			}
		}
	}
	return r, wrap("response stats", err)
}

// --- Утренняя сводка ---

// DigestTarget — администратор и студии, по которым ему положена сводка.
type DigestTarget struct {
	MaxUserID string
	VenueIDs  []string
}

func (s *Store) DigestTargets(ctx context.Context, day time.Time) ([]DigestTarget, error) {
	rows, err := s.pool.Query(ctx, `SELECT sa.max_user_id, array_agg(sa.venue_id::text ORDER BY sa.created_at)
		FROM studio_admins sa LEFT JOIN studio_settings ss ON ss.venue_id = sa.venue_id
		WHERE COALESCE(ss.daily_digest, true)
		  AND NOT EXISTS (SELECT 1 FROM digest_log d WHERE d.max_user_id = sa.max_user_id AND d.day = $1::date)
		GROUP BY sa.max_user_id`, day)
	if err != nil {
		return nil, wrap("digest targets", err)
	}
	defer rows.Close()
	var list []DigestTarget
	for rows.Next() {
		var t DigestTarget
		if err := rows.Scan(&t.MaxUserID, &t.VenueIDs); err != nil {
			return nil, wrap("scan digest target", err)
		}
		list = append(list, t)
	}
	return list, wrap("digest targets", rows.Err())
}

// MarkDigest отмечает отправку; false — сводка за этот день уже ушла.
func (s *Store) MarkDigest(ctx context.Context, maxUserID string, day time.Time) (bool, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO digest_log (max_user_id, day) VALUES ($1, $2::date) ON CONFLICT DO NOTHING`, maxUserID, day)
	return tag.RowsAffected() > 0, wrap("mark digest", err)
}

func (s *Store) RequestedPasses(ctx context.Context, venueIDs []string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM user_passes WHERE venue_id = ANY($1::uuid[]) AND status = 'requested'`,
		venueIDs).Scan(&n)
	return n, wrap("requested passes", err)
}

// --- Служебный статус ---

func (s *Store) SetOpsStatus(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO ops_status (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value)
	return wrap("set ops status", err)
}

type OpsValue struct {
	Value     string
	UpdatedAt time.Time
}

func (s *Store) OpsStatus(ctx context.Context) (map[string]OpsValue, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value, updated_at FROM ops_status`)
	if err != nil {
		return nil, wrap("ops status", err)
	}
	defer rows.Close()
	out := map[string]OpsValue{}
	for rows.Next() {
		var k string
		var v OpsValue
		if err := rows.Scan(&k, &v.Value, &v.UpdatedAt); err != nil {
			return nil, wrap("scan ops status", err)
		}
		out[k] = v
	}
	return out, wrap("ops status", rows.Err())
}

// SeriesSlots — то же занятие в следующие недели (по шаблону расписания).
func (s *Store) SeriesSlots(ctx context.Context, slot *model.Slot, count int) ([]model.Slot, error) {
	if slot.TemplateID == "" || count <= 0 {
		return nil, nil
	}
	return s.ListSlots(ctx, SlotQuery{VenueIDs: []string{slot.VenueID}, TemplateID: slot.TemplateID,
		From: slot.StartAt.Add(time.Hour), To: slot.StartAt.AddDate(0, 0, 7*count+1), Limit: count})
}

// --- Счётчики для лимитов ---

func (s *Store) count(ctx context.Context, op, query string, args ...interface{}) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, query, args...).Scan(&n)
	return n, wrap(op, err)
}

// ActiveBooking — своя активная запись пользователя на занятие, если есть.
func (s *Store) HasActiveBooking(ctx context.Context, userID, slotID string) (bool, error) {
	n, err := s.count(ctx, "has booking", `SELECT count(*) FROM bookings WHERE user_id = $1 AND slot_id = $2
		AND status = 'confirmed' AND guest_token IS NULL`, userID, slotID)
	return n > 0, err
}

// CountUpcomingBookings — свои активные записи на будущие занятия.
func (s *Store) CountUpcomingBookings(ctx context.Context, userID string, now time.Time) (int, error) {
	return s.count(ctx, "count upcoming", `SELECT count(*) FROM bookings b JOIN slots s ON s.id = b.slot_id
		WHERE b.user_id = $1 AND b.status = 'confirmed' AND b.guest_token IS NULL AND s.start_at > $2`, userID, now)
}

func (s *Store) CountBookingsSince(ctx context.Context, userID string, since time.Time) (int, error) {
	return s.count(ctx, "count recent bookings", `SELECT count(*) FROM bookings WHERE user_id = $1 AND created_at >= $2`, userID, since)
}

// CountOpenGuests — места для друзей, которые ещё никто не принял.
func (s *Store) CountOpenGuests(ctx context.Context, userID string, now time.Time) (int, error) {
	return s.count(ctx, "count guests", `SELECT count(*) FROM bookings b JOIN slots s ON s.id = b.slot_id
		WHERE b.user_id = $1 AND b.status = 'confirmed' AND b.guest_token IS NOT NULL AND s.start_at > $2`, userID, now)
}

// UnclaimedGuests — непринятые места для друзей на занятия, которые начнутся раньше deadline.
func (s *Store) UnclaimedGuests(ctx context.Context, deadline time.Time) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE b.status = 'confirmed' AND b.guest_token IS NOT NULL AND s.start_at <= $1
		ORDER BY s.start_at LIMIT 100`, deadline)
}

// OpenPassRequest — неоплаченная заявка пользователя на абонемент в студии.
func (s *Store) OpenPassRequest(ctx context.Context, userID, venueID string) (*model.UserPass, error) {
	p, err := scanUserPass(s.pool.QueryRow(ctx, `SELECT `+userPassColumns+userPassFrom+`
		WHERE up.user_id = $1 AND up.venue_id = $2 AND up.status = 'requested' ORDER BY up.created_at DESC LIMIT 1`, userID, venueID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, wrap("open pass request", err)
}

func (s *Store) CountOpenPassRequests(ctx context.Context, userID string) (int, error) {
	return s.count(ctx, "count pass requests", `SELECT count(*) FROM user_passes WHERE user_id = $1 AND status = 'requested'`, userID)
}

func (s *Store) CountPassRequestsSince(ctx context.Context, userID string, since time.Time) (int, error) {
	return s.count(ctx, "count recent pass requests", `SELECT count(*) FROM user_passes WHERE user_id = $1 AND created_at >= $2`, userID, since)
}

// CountClientMessages — сообщения клиента студиям (venueID пустой — всем студиям).
func (s *Store) CountClientMessages(ctx context.Context, userID, venueID string, since time.Time) (int, error) {
	return s.count(ctx, "count messages", `SELECT count(*) FROM studio_messages WHERE user_id = $1 AND author = 'client'
		AND created_at >= $2 AND ($3 = '' OR venue_id::text = $3)`, userID, since, venueID)
}
