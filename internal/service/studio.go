package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/geocode"
	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
	"sportslot/internal/seed"
)

var (
	ErrNoStudio      = errors.New("user is not a studio admin")
	ErrDemoReadOnly  = errors.New("demo studio is read-only")
	ErrNotPlatform   = errors.New("platform admin only")
	ErrInvalidInvite = errors.New("invite is invalid or expired")
)

const inviteTTL = 7 * 24 * time.Hour

// StudioPresenter — сообщения, связанные с кабинетом студии (реализация в пакете bot).
type StudioPresenter interface {
	StudioEvent(ctx context.Context, adminMaxID, kind string, v *model.BookingView) error
	StudioMessage(ctx context.Context, adminMaxID string, venue *model.Venue, client *model.User, text string, messageID int64) error
	StudioReply(ctx context.Context, clientMaxID string, venue *model.Venue, text string) error
	SlotCancelled(ctx context.Context, v *model.BookingView) error
	StudioPassRequest(ctx context.Context, adminMaxID string, up *model.UserPass) error
	StudioPayment(ctx context.Context, adminMaxID string, p *model.Payment, venue *model.Venue) error
	StudioWaiting(ctx context.Context, adminMaxID string, venue *model.Venue, clientName string, since time.Time, messageID int64) error
	ClientWaiting(ctx context.Context, clientMaxID string, venue *model.Venue) error
	StudioDigest(ctx context.Context, adminMaxID string, day time.Time, studios []DigestStudio) error
}

// Access — к каким студиям есть доступ у того, кто открыл кабинет.
type Access struct {
	MaxUserID string
	Name      string
	VenueIDs  []string
	Platform  bool // владелец платформы: все студии и выдача приглашений
	Demo      bool // вход по общему коду: только демо-студии
}

func (a Access) Can(venueID string) bool {
	for _, id := range a.VenueIDs {
		if id == venueID {
			return true
		}
	}
	return false
}

type StudioService struct {
	store     *postgres.Store
	partner   *PartnerService
	geocoder  *geocode.Geocoder
	presenter StudioPresenter
	platform  map[string]bool
	inviteURL func(token string) string
	now       func() time.Time
}

func NewStudioService(store *postgres.Store, partner *PartnerService, platformIDs []string) *StudioService {
	ids := map[string]bool{}
	for _, id := range platformIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = true
		}
	}
	return &StudioService{store: store, partner: partner, platform: ids, now: time.Now,
		inviteURL: func(token string) string { return "studio_" + token }}
}

func (s *StudioService) SetPresenter(p StudioPresenter)           { s.presenter = p }
func (s *StudioService) SetGeocoder(g *geocode.Geocoder)          { s.geocoder = g }
func (s *StudioService) SetInviteURL(f func(token string) string) { s.inviteURL = f }

func (s *StudioService) IsPlatform(maxUserID string) bool { return s.platform[maxUserID] }

// ResolveUser — доступ пользователя MAX: студии по приглашениям, у владельца платформы — все.
func (s *StudioService) ResolveUser(ctx context.Context, maxUserID, name string) (Access, error) {
	a := Access{MaxUserID: maxUserID, Name: name, Platform: s.platform[maxUserID]}
	var err error
	if a.Platform {
		a.VenueIDs, err = s.store.VenueIDsBySource(ctx, model.VenueSourcePartner, model.VenueSourceDemo)
	} else {
		a.VenueIDs, err = s.store.StudioVenueIDs(ctx, maxUserID)
	}
	return a, err
}

// ResolveDemo — доступ по общему коду: только демо-студии.
func (s *StudioService) ResolveDemo(ctx context.Context) (Access, error) {
	ids, err := s.store.VenueIDsBySource(ctx, model.VenueSourceDemo)
	return Access{MaxUserID: "demo", Name: "Демо-доступ", VenueIDs: ids, Demo: true}, err
}

type StudioSummary struct {
	Venue    model.Venue `json:"venue"`
	Today    int         `json:"today_bookings"`
	Upcoming int         `json:"upcoming_bookings"`
	Unread   int         `json:"unread_messages"`
}

func (s *StudioService) Studios(ctx context.Context, a Access) ([]StudioSummary, error) {
	now := s.now().In(catalog.Moscow)
	dayEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, catalog.Moscow).AddDate(0, 0, 1)
	list := []StudioSummary{}
	for _, id := range a.VenueIDs {
		v, err := s.store.GetVenue(ctx, id)
		if err != nil {
			continue
		}
		views, err := s.store.VenueBookings(ctx, []string{id}, now, now.Add(8*24*time.Hour))
		if err != nil {
			return nil, err
		}
		sum := StudioSummary{Venue: *v}
		for _, b := range views {
			if b.Status == model.BookingStatusConfirmed {
				sum.Upcoming++
				if b.Slot.StartAt.Before(dayEnd) {
					sum.Today++
				}
			}
		}
		sum.Unread, _ = s.store.UnreadMessages(ctx, []string{id})
		list = append(list, sum)
	}
	return list, nil
}

func (s *StudioService) venue(ctx context.Context, a Access, venueID string) (*model.Venue, error) {
	if !a.Can(venueID) {
		return nil, ErrForbidden
	}
	return s.store.GetVenue(ctx, venueID)
}

// Bookings — записи в студии от начала сегодняшнего дня на неделю вперёд.
func (s *StudioService) Bookings(ctx context.Context, a Access, venueID string) ([]model.BookingView, error) {
	if _, err := s.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	now := s.now().In(catalog.Moscow)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, catalog.Moscow)
	views, err := s.store.VenueBookings(ctx, []string{venueID}, dayStart, dayStart.AddDate(0, 0, 8))
	if views == nil {
		views = []model.BookingView{}
	}
	return views, err
}

// ClientCard — запись и всё, что студии нужно знать о клиенте.
type ClientCard struct {
	Booking   model.BookingView   `json:"booking"`
	MaxUserID string              `json:"max_user_id"`
	History   []model.BookingView `json:"history"`
	Visits    int                 `json:"visits"`
	NoShows   int                 `json:"no_shows"`
	Messages  []model.Message     `json:"messages"`
}

func (s *StudioService) Booking(ctx context.Context, a Access, bookingID string) (*ClientCard, error) {
	view, err := s.store.GetBookingView(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if !a.Can(view.Venue.ID) {
		return nil, ErrForbidden
	}
	return s.clientCard(ctx, *view)
}

func (s *StudioService) clientCard(ctx context.Context, view model.BookingView) (*ClientCard, error) {
	history, err := s.store.ClientBookings(ctx, view.Venue.ID, view.UserID)
	if err != nil {
		return nil, err
	}
	card := &ClientCard{Booking: view, MaxUserID: view.MaxUserID, History: history}
	for _, h := range history {
		switch h.Status {
		case model.BookingStatusAttended:
			card.Visits++
		case model.BookingStatusNoShow:
			card.NoShows++
		}
	}
	card.Messages, err = s.store.ThreadMessages(ctx, view.Venue.ID, view.UserID)
	return card, err
}

// TrainingResult — ответ на учебный пропуск.
type TrainingResult struct {
	Training bool   `json:"training"`
	Message  string `json:"message"`
}

func (s *StudioService) CheckIn(ctx context.Context, a Access, raw string) (*model.BookingView, error) {
	return s.partner.CheckInVenues(ctx, raw, a.VenueIDs)
}

func (s *StudioService) Remind(ctx context.Context, a Access, bookingID string) error {
	view, err := s.store.GetBookingView(ctx, bookingID)
	if err != nil {
		return err
	}
	if !a.Can(view.Venue.ID) {
		return ErrForbidden
	}
	if view.Status != model.BookingStatusConfirmed {
		return ErrNotActive
	}
	if s.partner.presenter == nil {
		return nil
	}
	return s.partner.presenter.Reminder24h(ctx, view)
}

// --- Расписание ---

func (s *StudioService) Templates(ctx context.Context, a Access, venueID string) ([]model.Template, error) {
	if _, err := s.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	return s.store.ListTemplates(ctx, []string{venueID})
}

func validTemplate(t *model.Template) bool {
	t.Title = strings.TrimSpace(t.Title)
	if t.Level == "" {
		t.Level = "beginner"
	}
	_, sportOK := catalog.SportByID(t.SportType)
	return sportOK && t.Title != "" && len([]rune(t.Title)) <= 120 &&
		t.Weekday >= 0 && t.Weekday <= 6 && t.StartMinute >= 0 && t.StartMinute < 24*60 &&
		t.DurationMin >= 15 && t.DurationMin <= 300 && t.Quota >= 1 && t.Quota <= 100
}

// SaveTemplate сохраняет шаблон и перестраивает будущие занятия по нему.
// Занятия, на которые уже записались, не меняются — kept говорит, сколько их.
func (s *StudioService) SaveTemplate(ctx context.Context, a Access, t *model.Template) (kept int, err error) {
	if _, err := s.venue(ctx, a, t.VenueID); err != nil {
		return 0, err
	}
	if !validTemplate(t) {
		return 0, ErrInvalidRequest
	}
	if t.ID != "" {
		old, err := s.store.GetTemplate(ctx, t.ID)
		if err != nil {
			return 0, err
		}
		if old.VenueID != t.VenueID {
			return 0, ErrForbidden
		}
	}
	if err := s.store.SaveTemplate(ctx, t); err != nil {
		return 0, err
	}
	kept, err = s.store.DropEmptyTemplateSlots(ctx, t.ID, s.now())
	if err != nil {
		return 0, err
	}
	_, err = seed.ExpandSchedules(ctx, s.store, s.now(), t.VenueID)
	s.event(ctx, a, "template_saved", map[string]interface{}{"venue_id": t.VenueID})
	return kept, err
}

func (s *StudioService) DeleteTemplate(ctx context.Context, a Access, venueID, templateID string) (int, error) {
	t, err := s.store.GetTemplate(ctx, templateID)
	if err != nil {
		return 0, err
	}
	if t.VenueID != venueID || !a.Can(venueID) {
		return 0, ErrForbidden
	}
	kept, err := s.store.DropEmptyTemplateSlots(ctx, templateID, s.now())
	if err != nil {
		return 0, err
	}
	return kept, s.store.DeleteTemplate(ctx, templateID)
}

func (s *StudioService) Slots(ctx context.Context, a Access, venueID string) ([]model.StudioSlot, error) {
	if _, err := s.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	now := s.now()
	return s.store.StudioSlots(ctx, venueID, now.Add(-3*time.Hour), now.Add(catalog.SearchHorizon))
}

func (s *StudioService) slot(ctx context.Context, a Access, venueID, slotID string) (*model.Slot, error) {
	sl, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		return nil, err
	}
	if sl.VenueID != venueID || !a.Can(venueID) {
		return nil, ErrForbidden
	}
	return sl, nil
}

func (s *StudioService) CloseSlot(ctx context.Context, a Access, venueID, slotID string, closed bool) error {
	if _, err := s.slot(ctx, a, venueID, slotID); err != nil {
		return err
	}
	return s.store.SetSlotClosed(ctx, slotID, closed)
}

// CancelSlot отменяет занятие и сообщает каждому записавшемуся.
func (s *StudioService) CancelSlot(ctx context.Context, a Access, venueID, slotID string) (int, error) {
	if _, err := s.slot(ctx, a, venueID, slotID); err != nil {
		return 0, err
	}
	ids, err := s.store.CancelSlot(ctx, slotID)
	if err != nil {
		return 0, err
	}
	s.event(ctx, a, "slot_cancelled", map[string]interface{}{"slot_id": slotID, "bookings": len(ids)})
	if s.presenter != nil && len(ids) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			for _, id := range ids {
				view, err := s.store.GetBookingView(ctx, id)
				if err != nil || view.IsGuest {
					// Непринятое место для друга принадлежит пригласившему — ему уже пишем.
					continue
				}
				if err := s.presenter.SlotCancelled(ctx, view); err != nil {
					log.Printf("slot cancelled message %s: %v", id, err)
				}
			}
		}()
	}
	return len(ids), nil
}

func (s *StudioService) Stats(ctx context.Context, a Access, venueID string) (*StatsFull, error) {
	if _, err := s.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	now := s.now()
	since := now.AddDate(0, 0, -30)
	base, err := s.store.StudioStats(ctx, venueID, since, now)
	if err != nil {
		return nil, err
	}
	full := &StatsFull{StudioStats: base}
	if full.Response, err = s.store.ResponseStats(ctx, venueID, since); err != nil {
		return nil, err
	}
	full.Paid, err = s.store.PaymentTotals(ctx, []string{venueID}, since, now)
	return full, err
}

type VenueProfile struct {
	Phone       string `json:"phone"`
	Website     string `json:"website"`
	Description string `json:"description"`
	WhatToBring string `json:"what_to_bring"`
	TrialPrice  *int   `json:"trial_price"`
}

func (s *StudioService) UpdateProfile(ctx context.Context, a Access, venueID string, p VenueProfile) (*model.Venue, error) {
	v, err := s.venue(ctx, a, venueID)
	if err != nil {
		return nil, err
	}
	if v.IsDemo() && a.Demo {
		// Демо-студии по общему коду только просматриваются: их профиль общий для всех проверяющих.
		return nil, ErrDemoReadOnly
	}
	if p.TrialPrice != nil && (*p.TrialPrice < 0 || *p.TrialPrice > 100000) {
		return nil, ErrInvalidRequest
	}
	if p.Website != "" && !strings.HasPrefix(p.Website, "http") {
		p.Website = "https://" + p.Website
	}
	if err := s.store.UpdateVenueProfile(ctx, venueID, strings.TrimSpace(p.Phone), strings.TrimSpace(p.Website),
		strings.TrimSpace(p.Description), strings.TrimSpace(p.WhatToBring), p.TrialPrice); err != nil {
		return nil, err
	}
	return s.store.GetVenue(ctx, venueID)
}

// --- Переписка ---

func (s *StudioService) Threads(ctx context.Context, a Access, venueID string) ([]model.Thread, error) {
	if _, err := s.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	return s.store.Threads(ctx, venueID)
}

type ThreadView struct {
	User     model.User      `json:"user"`
	Messages []model.Message `json:"messages"`
}

func (s *StudioService) Thread(ctx context.Context, a Access, venueID, userID string) (*ThreadView, error) {
	if _, err := s.venue(ctx, a, venueID); err != nil {
		return nil, err
	}
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.store.ThreadMessages(ctx, venueID, userID)
	if err != nil {
		return nil, err
	}
	if !a.Demo {
		_ = s.store.MarkThreadRead(ctx, venueID, userID)
	}
	return &ThreadView{User: *user, Messages: msgs}, nil
}

// ThreadByMessage — переписка по id сообщения (из кнопки в уведомлении бота).
func (s *StudioService) ThreadByMessage(ctx context.Context, a Access, messageID int64) (*model.Message, error) {
	m, err := s.store.GetMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if !a.Can(m.VenueID) {
		return nil, ErrForbidden
	}
	return m, nil
}

func cleanText(text string) (string, bool) {
	text = strings.TrimSpace(text)
	n := len([]rune(text))
	return text, n >= 1 && n <= 2000
}

func (s *StudioService) Reply(ctx context.Context, a Access, venueID, userID, text string) (*model.Message, error) {
	v, err := s.venue(ctx, a, venueID)
	if err != nil {
		return nil, err
	}
	text, ok := cleanText(text)
	if !ok {
		return nil, ErrInvalidRequest
	}
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	m := &model.Message{VenueID: venueID, UserID: userID, Author: "studio", Text: text}
	if err := s.store.AddMessage(ctx, m); err != nil {
		return nil, err
	}
	_ = s.store.MarkThreadRead(ctx, venueID, userID)
	if s.presenter != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := s.presenter.StudioReply(ctx, user.MaxUserID, v, text); err != nil {
				log.Printf("studio reply message: %v", err)
			}
		}()
	}
	return m, nil
}

// SendClientMessage — сообщение клиента студии; администраторы получают уведомление в боте.
func (s *StudioService) SendClientMessage(ctx context.Context, maxUserID, name, venueID, text string) (*model.Message, error) {
	v, err := s.store.GetVenue(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v.BookingMode != model.BookingModeInstant {
		return nil, ErrExternalVenue
	}
	text, ok := cleanText(text)
	if !ok {
		return nil, ErrInvalidRequest
	}
	user, err := s.store.EnsureUser(ctx, maxUserID, name)
	if err != nil {
		return nil, err
	}
	m := &model.Message{VenueID: venueID, UserID: user.ID, Author: "client", Text: text}
	if err := s.store.AddMessage(ctx, m); err != nil {
		return nil, err
	}
	_ = s.store.LogEvent(ctx, maxUserID, "studio_message", map[string]interface{}{"venue_id": venueID})
	s.notifyAdmins(venueID, func(ctx context.Context, admin string) error {
		return s.presenter.StudioMessage(ctx, admin, v, user, text, m.ID)
	})
	return m, nil
}

func (s *StudioService) ClientThread(ctx context.Context, maxUserID, venueID string) ([]model.Message, error) {
	user, err := s.store.UserByMaxID(ctx, maxUserID)
	if errors.Is(err, ErrNotFound) {
		return []model.Message{}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.store.ThreadMessages(ctx, venueID, user.ID)
}

// NotifyBooking сообщает администраторам студии о событии с записью
// (booked, cancelled, rescheduled, going, promoted).
func (s *StudioService) NotifyBooking(kind string, view *model.BookingView) {
	s.notifyAdmins(view.Venue.ID, func(ctx context.Context, admin string) error {
		return s.presenter.StudioEvent(ctx, admin, kind, view)
	})
}

func (s *StudioService) notifyAdmins(venueID string, send func(ctx context.Context, adminMaxID string) error) {
	if s.presenter == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admins, err := s.store.StudioAdmins(ctx, venueID)
		if err != nil {
			log.Printf("studio admins %s: %v", venueID, err)
			return
		}
		for _, admin := range admins {
			if err := send(ctx, admin.MaxUserID); err != nil {
				log.Printf("studio notification to %s: %v", admin.MaxUserID, err)
			}
		}
	}()
}

// --- Приглашения и платформа ---

func (s *StudioService) AcceptInvite(ctx context.Context, maxUserID, name, token string) (*model.Venue, error) {
	token = strings.TrimSpace(strings.TrimPrefix(token, "studio_"))
	if len(token) != 32 {
		return nil, ErrInvalidInvite
	}
	venueID, err := s.store.AcceptInvite(ctx, token, maxUserID, name)
	if errors.Is(err, postgres.ErrInviteInvalid) {
		return nil, ErrInvalidInvite
	}
	if err != nil {
		return nil, err
	}
	_ = s.store.LogEvent(ctx, maxUserID, "studio_joined", map[string]interface{}{"venue_id": venueID})
	return s.store.GetVenue(ctx, venueID)
}

type PlatformStudio struct {
	Venue   model.Venue         `json:"venue"`
	Admins  []model.StudioAdmin `json:"admins"`
	Invites []model.Invite      `json:"invites"`
}

func (s *StudioService) PlatformStudios(ctx context.Context, a Access) ([]PlatformStudio, error) {
	if !a.Platform {
		return nil, ErrNotPlatform
	}
	list := []PlatformStudio{}
	for _, id := range a.VenueIDs {
		v, err := s.store.GetVenue(ctx, id)
		if err != nil {
			continue
		}
		admins, err := s.store.StudioAdmins(ctx, id)
		if err != nil {
			return nil, err
		}
		invites, err := s.store.ActiveInvites(ctx, id)
		if err != nil {
			return nil, err
		}
		list = append(list, PlatformStudio{Venue: *v, Admins: admins, Invites: invites})
	}
	// Реальные студии — выше демо.
	sort.SliceStable(list, func(i, j int) bool { return !list[i].Venue.IsDemo() && list[j].Venue.IsDemo() })
	return list, nil
}

func (s *StudioService) SearchCatalog(ctx context.Context, a Access, query string) ([]model.Venue, error) {
	if !a.Platform {
		return nil, ErrNotPlatform
	}
	query = strings.TrimSpace(query)
	if len([]rune(query)) < 2 {
		return []model.Venue{}, nil
	}
	return s.store.SearchCatalog(ctx, query, 20)
}

// Connect подключает зал из каталога к онлайн-записи.
func (s *StudioService) Connect(ctx context.Context, a Access, venueID string) (*model.Venue, error) {
	if !a.Platform {
		return nil, ErrNotPlatform
	}
	if err := s.store.ConnectVenue(ctx, venueID); err != nil {
		return nil, err
	}
	s.event(ctx, a, "studio_connected", map[string]interface{}{"venue_id": venueID})
	return s.store.GetVenue(ctx, venueID)
}

type NewStudio struct {
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Sports  []string `json:"sports"`
	Phone   string   `json:"phone"`
}

// CreateStudio добавляет студию, которой нет в каталоге: координаты — по адресу.
func (s *StudioService) CreateStudio(ctx context.Context, a Access, n NewStudio) (*model.Venue, error) {
	if !a.Platform {
		return nil, ErrNotPlatform
	}
	n.Name, n.Address = strings.TrimSpace(n.Name), strings.TrimSpace(n.Address)
	var sports []string
	for _, sp := range n.Sports {
		if _, ok := catalog.SportByID(sp); ok {
			sports = append(sports, sp)
		}
	}
	if n.Name == "" || n.Address == "" || len(sports) == 0 {
		return nil, ErrInvalidRequest
	}
	place, err := s.geocoder.Find(ctx, n.Address)
	if err != nil {
		return nil, err
	}
	v := &model.Venue{
		ID: newUUID(), Name: n.Name, SportType: sports[0], Sports: sports, Level: "beginner",
		Address: place.Address, District: catalog.NearestDistrict(place.Lat, place.Lon).ID, Lat: place.Lat, Lon: place.Lon,
		Source: model.VenueSourcePartner, BookingMode: model.BookingModeInstant, Phone: strings.TrimSpace(n.Phone),
	}
	if err := s.store.UpsertVenue(ctx, v); err != nil {
		return nil, err
	}
	s.event(ctx, a, "studio_created", map[string]interface{}{"venue_id": v.ID})
	return s.store.GetVenue(ctx, v.ID)
}

type InviteLink struct {
	Token     string    `json:"token"`
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *StudioService) CreateInvite(ctx context.Context, a Access, venueID string) (*InviteLink, error) {
	if !a.Platform {
		return nil, ErrNotPlatform
	}
	v, err := s.store.GetVenue(ctx, venueID)
	if err != nil {
		return nil, err
	}
	if v.BookingMode != model.BookingModeInstant {
		return nil, ErrExternalVenue
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(buf)
	expires := s.now().Add(inviteTTL)
	if err := s.store.CreateInvite(ctx, token, venueID, a.MaxUserID, expires); err != nil {
		return nil, err
	}
	return &InviteLink{Token: token, Link: s.inviteURL(token), ExpiresAt: expires}, nil
}

func (s *StudioService) RemoveAdmin(ctx context.Context, a Access, venueID, maxUserID string) error {
	if !a.Platform {
		return ErrNotPlatform
	}
	return s.store.RemoveStudioAdmin(ctx, venueID, maxUserID)
}

func (s *StudioService) event(ctx context.Context, a Access, kind string, payload map[string]interface{}) {
	s.logEvent(ctx, a.MaxUserID, kind, payload)
}

func (s *StudioService) logEvent(ctx context.Context, maxUserID, kind string, payload map[string]interface{}) {
	if err := s.store.LogEvent(ctx, maxUserID, kind, payload); err != nil {
		log.Printf("event %s: %v", kind, err)
	}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
