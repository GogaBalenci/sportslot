package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"sportslot/internal/model"
	"sportslot/internal/repository"
)

// --- Шаблоны расписания ---

const templateColumns = `id, venue_id, weekday, start_minute, duration_min, title, sport_type, level, quota, updated_at`

func scanTemplate(row pgx.Row) (*model.Template, error) {
	var t model.Template
	if err := row.Scan(&t.ID, &t.VenueID, &t.Weekday, &t.StartMinute, &t.DurationMin, &t.Title, &t.SportType,
		&t.Level, &t.Quota, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListTemplates(ctx context.Context, venueIDs []string) ([]model.Template, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+templateColumns+` FROM schedule_templates
		WHERE venue_id = ANY($1::uuid[]) ORDER BY venue_id, (weekday + 6) % 7, start_minute, title`, venueIDs)
	if err != nil {
		return nil, wrap("list templates", err)
	}
	defer rows.Close()
	list := []model.Template{}
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, wrap("scan template", err)
		}
		list = append(list, *t)
	}
	return list, wrap("list templates", rows.Err())
}

func (s *Store) GetTemplate(ctx context.Context, id string) (*model.Template, error) {
	t, err := scanTemplate(s.pool.QueryRow(ctx, `SELECT `+templateColumns+` FROM schedule_templates WHERE id = $1`, id))
	return t, wrap("get template", notFound(err))
}

func (s *Store) CountTemplates(ctx context.Context, venueID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM schedule_templates WHERE venue_id = $1`, venueID).Scan(&n)
	return n, wrap("count templates", err)
}

// SaveTemplate создаёт шаблон (пустой ID — новый) или обновляет существующий.
func (s *Store) SaveTemplate(ctx context.Context, t *model.Template) error {
	if t.ID == "" {
		return wrap("create template", s.pool.QueryRow(ctx, `
			INSERT INTO schedule_templates (venue_id, weekday, start_minute, duration_min, title, sport_type, level, quota)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, updated_at`,
			t.VenueID, t.Weekday, t.StartMinute, t.DurationMin, t.Title, t.SportType, t.Level, t.Quota).Scan(&t.ID, &t.UpdatedAt))
	}
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO schedule_templates (id, venue_id, weekday, start_minute, duration_min, title, sport_type, level, quota)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (id) DO UPDATE SET weekday = EXCLUDED.weekday, start_minute = EXCLUDED.start_minute,
			duration_min = EXCLUDED.duration_min, title = EXCLUDED.title, sport_type = EXCLUDED.sport_type,
			level = EXCLUDED.level, quota = EXCLUDED.quota, updated_at = now()
		WHERE schedule_templates.venue_id = EXCLUDED.venue_id`,
		t.ID, t.VenueID, t.Weekday, t.StartMinute, t.DurationMin, t.Title, t.SportType, t.Level, t.Quota)
	if err != nil {
		return wrap("save template", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTemplate(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM schedule_templates WHERE id = $1`, id)
	return wrap("delete template", err)
}

// TemplateWithSource — шаблон и тип площадки (для демо-студий занятые места моделируются).
type TemplateWithSource struct {
	model.Template
	Source model.VenueSource
}

func (s *Store) ActiveTemplates(ctx context.Context, venueID string) ([]TemplateWithSource, error) {
	rows, err := s.pool.Query(ctx, `SELECT t.id, t.venue_id, t.weekday, t.start_minute, t.duration_min, t.title,
			t.sport_type, t.level, t.quota, t.updated_at, v.source
		FROM schedule_templates t JOIN venues v ON v.id = t.venue_id
		WHERE v.booking_mode = 'instant' AND ($1 = '' OR t.venue_id::text = $1)`, venueID)
	if err != nil {
		return nil, wrap("active templates", err)
	}
	defer rows.Close()
	var list []TemplateWithSource
	for rows.Next() {
		var t TemplateWithSource
		if err := rows.Scan(&t.ID, &t.VenueID, &t.Weekday, &t.StartMinute, &t.DurationMin, &t.Title, &t.SportType,
			&t.Level, &t.Quota, &t.UpdatedAt, &t.Source); err != nil {
			return nil, wrap("scan template", err)
		}
		list = append(list, t)
	}
	return list, wrap("active templates", rows.Err())
}

// InsertTemplateSlot добавляет занятие из шаблона; уже существующее занятие
// (например, созданное до появления шаблонов) только привязывается к шаблону.
func (s *Store) InsertTemplateSlot(ctx context.Context, sl *model.Slot) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO slots (id, venue_id, title, sport_type, level, start_at, end_at, quota_total, quota_booked, template_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET template_id = EXCLUDED.template_id WHERE slots.template_id IS NULL`,
		sl.ID, sl.VenueID, sl.Title, sl.SportType, sl.Level, sl.StartAt, sl.EndAt, sl.QuotaTotal, sl.QuotaBooked, sl.TemplateID)
	return tag.RowsAffected() > 0, wrap("insert template slot", err)
}

// DropEmptyTemplateSlots удаляет будущие занятия шаблона без броней и очереди —
// перед тем как развернуть изменённый шаблон заново. Занятия с бронями (даже
// отменёнными) остаются ради истории. Возвращает, сколько активных занятий
// оставлено, потому что на них уже кто-то записан.
func (s *Store) DropEmptyTemplateSlots(ctx context.Context, templateID string, now time.Time) (int, error) {
	if _, err := s.pool.Exec(ctx, `DELETE FROM slots s WHERE s.template_id = $1 AND s.start_at > $2
		AND NOT EXISTS (SELECT 1 FROM bookings b WHERE b.slot_id = s.id)
		AND NOT EXISTS (SELECT 1 FROM waitlist w WHERE w.slot_id = s.id)`, templateID, now); err != nil {
		return 0, wrap("drop template slots", err)
	}
	var kept int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM slots WHERE template_id = $1 AND start_at > $2 AND cancelled_at IS NULL`,
		templateID, now).Scan(&kept)
	return kept, wrap("count kept slots", err)
}

// --- Занятия в кабинете ---

func (s *Store) StudioSlots(ctx context.Context, venueID string, from, to time.Time) ([]model.StudioSlot, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+slotColumns+`,
			(SELECT count(*) FROM bookings b WHERE b.slot_id = s.id AND b.status IN ('confirmed', 'attended'))
		FROM slots s WHERE s.venue_id = $1 AND s.start_at >= $2 AND s.start_at < $3
		ORDER BY s.start_at, s.title`, venueID, from, to)
	if err != nil {
		return nil, wrap("studio slots", err)
	}
	defer rows.Close()
	list := []model.StudioSlot{}
	for rows.Next() {
		var ss model.StudioSlot
		sl := &ss.Slot
		if err := rows.Scan(&sl.ID, &sl.VenueID, &sl.Title, &sl.SportType, &sl.Level, &sl.StartAt, &sl.EndAt,
			&sl.QuotaTotal, &sl.QuotaBooked, &sl.TemplateID, &sl.Closed, &sl.Cancelled, &ss.Booked); err != nil {
			return nil, wrap("scan studio slot", err)
		}
		list = append(list, ss)
	}
	return list, wrap("studio slots", rows.Err())
}

func (s *Store) SetSlotClosed(ctx context.Context, slotID string, closed bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE slots SET booking_closed = $2 WHERE id = $1 AND cancelled_at IS NULL`, slotID, closed)
	if err != nil {
		return wrap("close slot", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotActive
	}
	return nil
}

// CancelSlot отменяет занятие студией: запись на него закрывается, все активные
// брони отменяются с причиной studio, очередь закрывается. Возвращает id
// отменённых броней — их владельцев нужно уведомить.
func (s *Store) CancelSlot(ctx context.Context, slotID string) ([]string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var cancelled bool
	if err := tx.QueryRow(ctx, `SELECT cancelled_at IS NOT NULL FROM slots WHERE id = $1 FOR UPDATE`, slotID).Scan(&cancelled); err != nil {
		return nil, wrap("lock slot", notFound(err))
	}
	if cancelled {
		return nil, repository.ErrNotActive
	}
	rows, err := tx.Query(ctx, `UPDATE bookings SET status = 'cancelled', cancelled_at = now(), cancel_reason = 'studio'
		WHERE slot_id = $1 AND status = 'confirmed' RETURNING id`, slotID)
	if err != nil {
		return nil, wrap("cancel bookings", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, wrap("scan booking", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := refundPassVisit(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE slots SET cancelled_at = now(), booking_closed = true,
		quota_booked = GREATEST(quota_booked - $2, 0) WHERE id = $1`, slotID, len(ids)); err != nil {
		return nil, wrap("cancel slot", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE waitlist SET status = 'cancelled' WHERE slot_id = $1 AND status = 'waiting'`, slotID); err != nil {
		return nil, wrap("close waitlist", err)
	}
	return ids, wrap("commit", tx.Commit(ctx))
}

// --- Администраторы и приглашения ---

func (s *Store) StudioVenueIDs(ctx context.Context, maxUserID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT venue_id FROM studio_admins WHERE max_user_id = $1 ORDER BY created_at`, maxUserID)
	if err != nil {
		return nil, wrap("studio venues", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) VenueIDsBySource(ctx context.Context, sources ...model.VenueSource) ([]string, error) {
	names := make([]string, len(sources))
	for i, src := range sources {
		names[i] = string(src)
	}
	rows, err := s.pool.Query(ctx, `SELECT id FROM venues WHERE source = ANY($1) ORDER BY name`, names)
	if err != nil {
		return nil, wrap("venues by source", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) StudioAdmins(ctx context.Context, venueID string) ([]model.StudioAdmin, error) {
	rows, err := s.pool.Query(ctx, `SELECT venue_id, max_user_id, name, created_at FROM studio_admins
		WHERE venue_id = $1 ORDER BY created_at`, venueID)
	if err != nil {
		return nil, wrap("studio admins", err)
	}
	defer rows.Close()
	list := []model.StudioAdmin{}
	for rows.Next() {
		var a model.StudioAdmin
		if err := rows.Scan(&a.VenueID, &a.MaxUserID, &a.Name, &a.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

func (s *Store) RemoveStudioAdmin(ctx context.Context, venueID, maxUserID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM studio_admins WHERE venue_id = $1 AND max_user_id = $2`, venueID, maxUserID)
	return wrap("remove admin", err)
}

func (s *Store) CreateInvite(ctx context.Context, token, venueID, createdBy string, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO studio_invites (token, venue_id, created_by, expires_at) VALUES ($1,$2,$3,$4)`,
		token, venueID, createdBy, expires)
	return wrap("create invite", err)
}

var ErrInviteInvalid = errors.New("invite is invalid or expired")

// AcceptInvite привязывает пользователя к студии. Приглашение одноразовое;
// повторное открытие тем же пользователем просто возвращает студию.
func (s *Store) AcceptInvite(ctx context.Context, token, maxUserID, name string) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", wrap("begin", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var venueID string
	var usedBy *string
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT venue_id, used_by, expires_at FROM studio_invites WHERE token = $1 FOR UPDATE`, token).
		Scan(&venueID, &usedBy, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInviteInvalid
	}
	if err != nil {
		return "", wrap("load invite", err)
	}
	if usedBy != nil && *usedBy != maxUserID {
		return "", ErrInviteInvalid
	}
	if usedBy == nil && time.Now().After(expires) {
		return "", ErrInviteInvalid
	}
	if _, err := tx.Exec(ctx, `UPDATE studio_invites SET used_by = $2, used_at = COALESCE(used_at, now()) WHERE token = $1`,
		token, maxUserID); err != nil {
		return "", wrap("use invite", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO studio_admins (venue_id, max_user_id, name) VALUES ($1,$2,$3)
		ON CONFLICT (venue_id, max_user_id) DO UPDATE SET name = EXCLUDED.name`, venueID, maxUserID, name); err != nil {
		return "", wrap("add admin", err)
	}
	return venueID, wrap("commit", tx.Commit(ctx))
}

func (s *Store) ActiveInvites(ctx context.Context, venueID string) ([]model.Invite, error) {
	rows, err := s.pool.Query(ctx, `SELECT token, venue_id, expires_at, COALESCE(used_by, ''), used_at FROM studio_invites
		WHERE venue_id = $1 AND (used_by IS NOT NULL OR expires_at > now()) ORDER BY created_at DESC LIMIT 20`, venueID)
	if err != nil {
		return nil, wrap("invites", err)
	}
	defer rows.Close()
	list := []model.Invite{}
	for rows.Next() {
		var i model.Invite
		if err := rows.Scan(&i.Token, &i.VenueID, &i.ExpiresAt, &i.UsedBy, &i.UsedAt); err != nil {
			return nil, err
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

// --- Площадки для платформы ---

func (s *Store) SearchCatalog(ctx context.Context, query string, limit int) ([]model.Venue, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+venueColumns+` FROM venues
		WHERE source = 'osm' AND (name ILIKE '%' || $1 || '%' OR address ILIKE '%' || $1 || '%')
		ORDER BY name LIMIT $2`, query, limit)
	if err != nil {
		return nil, wrap("search catalog", err)
	}
	defer rows.Close()
	list := []model.Venue{}
	for rows.Next() {
		v, err := scanVenue(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *v)
	}
	return list, rows.Err()
}

// ConnectVenue переводит зал из каталога в студию-партнёра с онлайн-записью.
func (s *Store) ConnectVenue(ctx context.Context, venueID string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE venues SET source = 'partner', booking_mode = 'instant', level = 'beginner'
		WHERE id = $1 AND source = 'osm'`, venueID)
	if err != nil {
		return wrap("connect venue", err)
	}
	if tag.RowsAffected() == 0 {
		return repository.ErrNotActive
	}
	return nil
}

// UpdateVenueProfile — то, что студия может поправить сама.
func (s *Store) UpdateVenueProfile(ctx context.Context, venueID, phone, website, description, whatToBring string, trialPrice *int) error {
	_, err := s.pool.Exec(ctx, `UPDATE venues SET phone = $2, website = $3, description = $4, what_to_bring = $5, trial_price = $6
		WHERE id = $1`, venueID, phone, website, description, whatToBring, trialPrice)
	return wrap("update venue profile", err)
}

// --- Сообщения ---

func (s *Store) AddMessage(ctx context.Context, m *model.Message) error {
	return wrap("add message", s.pool.QueryRow(ctx, `INSERT INTO studio_messages (venue_id, user_id, author, text)
		VALUES ($1,$2,$3,$4) RETURNING id, created_at`, m.VenueID, m.UserID, m.Author, m.Text).Scan(&m.ID, &m.CreatedAt))
}

func (s *Store) GetMessage(ctx context.Context, id int64) (*model.Message, error) {
	var m model.Message
	err := s.pool.QueryRow(ctx, `SELECT id, venue_id, user_id, author, text, created_at FROM studio_messages WHERE id = $1`, id).
		Scan(&m.ID, &m.VenueID, &m.UserID, &m.Author, &m.Text, &m.CreatedAt)
	return &m, wrap("get message", notFound(err))
}

func (s *Store) ThreadMessages(ctx context.Context, venueID, userID string) ([]model.Message, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, venue_id, user_id, author, text, created_at FROM studio_messages
		WHERE venue_id = $1 AND user_id = $2 ORDER BY created_at, id`, venueID, userID)
	if err != nil {
		return nil, wrap("thread", err)
	}
	defer rows.Close()
	list := []model.Message{}
	for rows.Next() {
		var m model.Message
		if err := rows.Scan(&m.ID, &m.VenueID, &m.UserID, &m.Author, &m.Text, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// MarkThreadRead отмечает сообщения клиента прочитанными студией.
func (s *Store) MarkThreadRead(ctx context.Context, venueID, userID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE studio_messages SET read_at = now()
		WHERE venue_id = $1 AND user_id = $2 AND author = 'client' AND read_at IS NULL`, venueID, userID)
	return wrap("mark read", err)
}

func (s *Store) Threads(ctx context.Context, venueID string) ([]model.Thread, error) {
	rows, err := s.pool.Query(ctx, `SELECT m.user_id, COALESCE(u.name, ''), u.max_user_id, last.text, last.created_at, last.author,
			(SELECT count(*) FROM studio_messages x WHERE x.venue_id = m.venue_id AND x.user_id = m.user_id
				AND x.author = 'client' AND x.read_at IS NULL)
		FROM (SELECT DISTINCT venue_id, user_id FROM studio_messages WHERE venue_id = $1) m
		JOIN users u ON u.id = m.user_id
		JOIN LATERAL (SELECT text, created_at, author FROM studio_messages y
			WHERE y.venue_id = m.venue_id AND y.user_id = m.user_id ORDER BY created_at DESC, id DESC LIMIT 1) last ON true
		ORDER BY last.created_at DESC`, venueID)
	if err != nil {
		return nil, wrap("threads", err)
	}
	defer rows.Close()
	list := []model.Thread{}
	for rows.Next() {
		var t model.Thread
		if err := rows.Scan(&t.UserID, &t.UserName, &t.MaxUserID, &t.LastText, &t.LastAt, &t.LastBy, &t.Unread); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) UnreadMessages(ctx context.Context, venueIDs []string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM studio_messages WHERE venue_id = ANY($1::uuid[])
		AND author = 'client' AND read_at IS NULL`, venueIDs).Scan(&n)
	return n, wrap("unread", err)
}

// --- Клиенты и статистика ---

// ClientBookings — история клиента в студии (для карточки клиента).
func (s *Store) ClientBookings(ctx context.Context, venueID, userID string) ([]model.BookingView, error) {
	return s.queryViews(ctx, `WHERE v.id = $1 AND b.user_id = $2 ORDER BY s.start_at DESC`, venueID, userID)
}

func (s *Store) StudioStats(ctx context.Context, venueID string, since, now time.Time) (*model.StudioStats, error) {
	st := &model.StudioStats{Since: since}
	err := s.pool.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE NOT (b.status = 'cancelled' AND b.cancel_reason = 'studio')),
			count(*) FILTER (WHERE b.status = 'attended'),
			count(*) FILTER (WHERE b.status = 'no_show'),
			count(*) FILTER (WHERE b.status = 'cancelled' AND b.cancel_reason <> 'studio'),
			count(*) FILTER (WHERE b.status = 'confirmed' AND s.start_at > $3),
			count(DISTINCT b.user_id),
			COALESCE(avg(b.rating), 0), count(b.rating)
		FROM bookings b JOIN slots s ON s.id = b.slot_id
		WHERE s.venue_id = $1 AND s.start_at >= $2`, venueID, since, now).
		Scan(&st.Bookings, &st.Attended, &st.NoShow, &st.CancelledUser, &st.Upcoming, &st.Clients, &st.RatingAvg, &st.Ratings)
	if err != nil {
		return nil, wrap("studio stats", err)
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM (
			SELECT b.user_id FROM bookings b JOIN slots s ON s.id = b.slot_id
			WHERE s.venue_id = $1 AND b.status IN ('attended', 'confirmed')
			GROUP BY b.user_id HAVING count(*) >= 2) r`, venueID).Scan(&st.ReturningUsers)
	return st, wrap("returning clients", err)
}

// UserByMaxID — внутренний пользователь по id MAX.
func (s *Store) UserByMaxID(ctx context.Context, maxUserID string) (*model.User, error) {
	var u model.User
	err := s.pool.QueryRow(ctx, `SELECT id, max_user_id, COALESCE(name, ''), created_at FROM users WHERE max_user_id = $1`, maxUserID).
		Scan(&u.ID, &u.MaxUserID, &u.Name, &u.CreatedAt)
	return &u, wrap("user by max id", notFound(err))
}
