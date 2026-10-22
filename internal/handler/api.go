package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"sportslot/internal/catalog"
	"sportslot/internal/geocode"
	"sportslot/internal/model"
	"sportslot/internal/quiz"
	"sportslot/internal/routing"
	"sportslot/internal/service"
)

type API struct {
	search   *service.SearchService
	bookings *service.BookingService
	partner  *service.PartnerService
	studio   *service.StudioService
	commerce *service.CommerceService
	auth     UserAuth
	location LocationListener
	geocoder *geocode.Geocoder
	logins   *limiter
	lookups  *limiter
	now      func() time.Time
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validID(id string) bool { return uuidPattern.MatchString(id) }

func decode(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(dst) == nil
}

// writeServiceError переводит ошибки бизнес-логики в HTTP-ответы.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Не найдено")
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Запись принадлежит другому пользователю")
	case errors.Is(err, service.ErrQuotaExceeded):
		writeError(w, http.StatusConflict, "quota_exceeded", "Свободных мест нет")
	case errors.Is(err, service.ErrAlreadyBooked):
		writeError(w, http.StatusConflict, "already_booked", "Вы уже записаны на это занятие")
	case errors.Is(err, service.ErrSlotStarted):
		writeError(w, http.StatusConflict, "slot_started", "Занятие уже началось")
	case errors.Is(err, service.ErrNotActive):
		writeError(w, http.StatusConflict, "not_active", "Запись уже неактивна")
	case errors.Is(err, service.ErrExternalVenue):
		writeError(w, http.StatusBadRequest, "external_venue", "В этом зале запись через сервис недоступна")
	case errors.Is(err, service.ErrBookingClosed):
		writeError(w, http.StatusConflict, "booking_closed", "Запись на это занятие закрыта студией")
	case errors.Is(err, service.ErrInvalidRequest):
		writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
	case errors.Is(err, service.ErrOwnInvite):
		writeError(w, http.StatusConflict, "own_invite", "Это ваша ссылка — отправьте её другу")
	case errors.Is(err, service.ErrPaymentsOff):
		writeError(w, http.StatusConflict, "payments_off", "Онлайн-оплата у этой студии не подключена")
	case errors.Is(err, service.ErrNothingToPay):
		writeError(w, http.StatusConflict, "nothing_to_pay", "Оплачивать нечего: уже оплачено или бесплатно")
	case errors.Is(err, service.ErrPaymentFailed):
		writeError(w, http.StatusBadGateway, "payment_failed", "Платёжная система не приняла запрос. Попробуйте позже или оплатите в студии")
	case errors.Is(err, service.ErrNoSecretKey):
		writeError(w, http.StatusServiceUnavailable, "payments_unconfigured", "Сервер пока не настроен для хранения ключей оплаты (PAYMENT_SECRET_KEY)")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка, попробуйте позже")
	}
}

func (a *API) Catalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sports":    catalog.Sports,
		"districts": catalog.Districts,
		"whens":     catalog.Whens,
		"quiz":      quiz.Questions,
		"data_sources": map[string]interface{}{
			"catalog":         "OpenStreetMap (ODbL)",
			"catalog_date":    a.search.CatalogDate(),
			"demo_partners":   a.search.DemoEnabled(),
			"partner_enabled": a.partner.Enabled(),
		},
	})
}

func (a *API) QuizRecommend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Answers map[string]string `json:"answers"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": quiz.Recommend(req.Answers, 3)})
}

func (a *API) Venues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := service.VenueSearch{
		Sport:       q.Get("sport"),
		When:        q.Get("when"),
		District:    q.Get("district"),
		InstantOnly: q.Get("instant_only") == "true",
	}
	if search.Sport != "" {
		if _, ok := catalog.SportByID(search.Sport); !ok {
			writeError(w, http.StatusBadRequest, "invalid_sport", "Неизвестный вид спорта")
			return
		}
	}
	if lat, lon, ok := parsePoint(q.Get("lat"), q.Get("lon")); ok {
		search.Lat, search.Lon, search.HasPoint = lat, lon, true
	}
	if raw := q.Get("mode"); raw != "" {
		mode, ok := routing.ParseMode(raw)
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_mode", "mode: walking, transit или driving")
			return
		}
		search.Mode = mode
	}
	if raw := q.Get("max_minutes"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 180 {
			writeError(w, http.StatusBadRequest, "invalid_max_minutes", "max_minutes: целое число от 0 до 180")
			return
		}
		search.MaxMinutes = n
	}
	cards, err := a.search.FindVenues(r.Context(), search, a.now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"venues": cards, "meta": a.meta()})
}

func (a *API) meta() map[string]interface{} {
	source := "osm"
	if a.search.DemoEnabled() {
		source = "osm+demo_partners"
	}
	return map[string]interface{}{"data_source": source, "catalog_date": a.search.CatalogDate(), "generated_at": a.now()}
}

func (a *API) Venue(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "venue_not_found", "Площадка не найдена")
		return
	}
	details, err := a.search.VenueDetails(r.Context(), id, a.now())
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeError(w, http.StatusNotFound, "venue_not_found", "Площадка не найдена")
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}

type legacySearchRequest struct {
	SportType string  `json:"sport_type"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	RadiusKM  float64 `json:"radius_km"`
	DateFrom  string  `json:"date_from"`
	DateTo    string  `json:"date_to"`
	TimeFrom  string  `json:"time_from"`
	TimeTo    string  `json:"time_to"`
	Level     string  `json:"level"`
}

// Search — поиск свободных занятий с онлайн-записью (контракт DATA-API).
func (a *API) Search(w http.ResponseWriter, r *http.Request) {
	var req legacySearchRequest
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	if _, ok := catalog.SportByID(req.SportType); !ok {
		writeError(w, http.StatusBadRequest, "invalid_sport_type", "Поле sport_type обязательно: swimming, yoga, boxing, fitness, dance, team")
		return
	}
	q := service.SlotSearch{Sport: req.SportType, Lat: req.Lat, Lon: req.Lon, RadiusKM: req.RadiusKM,
		TimeFrom: req.TimeFrom, TimeTo: req.TimeTo}
	var err error
	if req.DateFrom != "" {
		if q.DateFrom, err = time.ParseInLocation("2006-01-02", req.DateFrom, catalog.Moscow); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_from", "date_from в формате YYYY-MM-DD")
			return
		}
	}
	if req.DateTo != "" {
		if q.DateTo, err = time.ParseInLocation("2006-01-02", req.DateTo, catalog.Moscow); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_date_to", "date_to в формате YYYY-MM-DD")
			return
		}
		q.DateTo = q.DateTo.AddDate(0, 0, 1)
	}
	results, err := a.search.FindSlots(r.Context(), q, a.now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results, "meta": a.meta()})
}

func (a *API) Me(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	user, views, err := a.bookings.MyBookings(r.Context(), u.MaxUserID, u.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	resp := map[string]interface{}{
		"user":     map[string]string{"id": user.ID, "name": user.Name},
		"bookings": views,
	}
	// Последняя точка, которую пользователь отправил боту в чат: в мини-приложении
	// MAX на телефоне геолокация браузера доступна не всегда.
	if loc, err := a.bookings.Store().LastLocation(r.Context(), u.MaxUserID, 7*24*time.Hour); err == nil && loc != nil {
		resp["last_location"] = loc
	}
	writeJSON(w, http.StatusOK, resp)
}

// Geocode — координаты адреса в Ростове (геокодер 2ГИС на сервере, ключ не уходит в браузер).
func (a *API) Geocode(w http.ResponseWriter, r *http.Request) {
	if !a.lookups.allow(clientIP(r), a.now()) {
		writeError(w, http.StatusTooManyRequests, "too_many_requests", "Слишком много запросов, подождите минуту")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 3 || len([]rune(q)) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_query", "Параметр q: адрес от 3 до 200 символов")
		return
	}
	place, err := a.geocoder.Find(r.Context(), q)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, place)
	case errors.Is(err, geocode.ErrNotFound):
		writeError(w, http.StatusNotFound, "address_not_found", "Адрес в Ростове-на-Дону не найден")
	default:
		writeError(w, http.StatusServiceUnavailable, "geocoder_unavailable", "Поиск адреса временно недоступен")
	}
}

func (a *API) CreateBooking(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		SlotID        string `json:"slot_id"`
		SourceChannel string `json:"source_channel"`
		// Weeks — записаться и на то же время в следующие недели (до 4).
		Weeks int `json:"weeks"`
		// WithFriend — сразу второе место для друга.
		WithFriend bool `json:"with_friend"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	if !validID(req.SlotID) {
		writeError(w, http.StatusBadRequest, "invalid_slot_id", "Поле slot_id должно быть UUID")
		return
	}
	channel := model.SourceChannel(req.SourceChannel)
	if channel == "" {
		channel = model.SourceChannelMiniApp
	}
	if channel != model.SourceChannelBot && channel != model.SourceChannelMiniApp {
		writeError(w, http.StatusBadRequest, "invalid_source_channel", "source_channel: bot или miniapp")
		return
	}
	if req.Weeks < 0 || req.Weeks > 4 {
		writeError(w, http.StatusBadRequest, "invalid_weeks", "weeks: от 1 до 4")
		return
	}
	view, created, err := a.bookings.Book(r.Context(), u.MaxUserID, u.Name, req.SlotID, channel)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeError(w, http.StatusNotFound, "slot_not_found", "Занятие не найдено")
			return
		}
		writeServiceError(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	if req.Weeks <= 1 && !req.WithFriend {
		writeJSON(w, status, view)
		return
	}
	resp := bookingResponse{BookingView: view}
	if req.Weeks > 1 {
		// Первая неделя уже записана; BookSeries вернёт её как существующую запись.
		resp.Series, _ = a.bookings.BookSeries(r.Context(), u.MaxUserID, u.Name, req.SlotID, req.Weeks, channel)
	}
	if req.WithFriend {
		place, err := a.bookings.AddFriend(r.Context(), view.ID, u.MaxUserID)
		if err != nil {
			resp.FriendError = "Для друга мест не осталось"
		} else {
			resp.Friend = place
		}
	}
	writeJSON(w, status, resp)
}

// bookingResponse — запись и то, что создано вместе с ней.
type bookingResponse struct {
	*model.BookingView
	Series      *service.SeriesResult `json:"series,omitempty"`
	Friend      *service.FriendPlace  `json:"friend,omitempty"`
	FriendError string                `json:"friend_error,omitempty"`
}

func (a *API) GetBooking(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись не найдена")
		return
	}
	view, err := a.bookings.Get(r.Context(), id, u.MaxUserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) CancelBooking(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись не найдена")
		return
	}
	if _, err := a.bookings.Cancel(r.Context(), id, u.MaxUserID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) Reschedule(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		NewSlotID string `json:"new_slot_id"`
	}
	if !validID(id) {
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись не найдена")
		return
	}
	if !decode(w, r, &req) || !validID(req.NewSlotID) {
		writeError(w, http.StatusBadRequest, "invalid_new_slot_id", "Поле new_slot_id должно быть UUID")
		return
	}
	view, err := a.bookings.Reschedule(r.Context(), id, req.NewSlotID, u.MaxUserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) ListByUser(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "user_id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "user_not_found", "Пользователь не найден")
		return
	}
	bookings, err := a.bookings.ListByUserID(r.Context(), id, u.MaxUserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bookings)
}

func (a *API) Waitlist(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		SlotID string `json:"slot_id"`
	}
	if !decode(w, r, &req) || !validID(req.SlotID) {
		writeError(w, http.StatusBadRequest, "invalid_slot_id", "Поле slot_id должно быть UUID")
		return
	}
	pos, err := a.bookings.JoinWaitlist(r.Context(), u.MaxUserID, u.Name, req.SlotID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int{"position": pos})
}

func (a *API) Feedback(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Rating int    `json:"rating"`
		Review string `json:"review"`
	}
	if !validID(id) || !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if _, err := a.bookings.Rate(r.Context(), id, u.MaxUserID, req.Rating); err != nil {
		writeServiceError(w, err)
		return
	}
	if strings.TrimSpace(req.Review) != "" {
		if err := a.bookings.Review(r.Context(), id, u.MaxUserID, req.Review); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Stats — агрегированная воронка пилота за 30 дней (без персональных данных).
func (a *API) Stats(w http.ResponseWriter, r *http.Request) {
	since := a.now().AddDate(0, 0, -30)
	stats, err := a.bookings.Store().FunnelStats(r.Context(), since)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"since": since, "unique_users": stats})
}

func (a *API) PartnerLogin(w http.ResponseWriter, r *http.Request) {
	if !a.logins.allow(clientIP(r), a.now()) {
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "Слишком много попыток, подождите минуту")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	token, expires, err := a.partner.Login(r.Context(), strings.TrimSpace(req.Code))
	if err != nil {
		if errors.Is(err, service.ErrPartnerCode) {
			writeError(w, http.StatusUnauthorized, "invalid_code", "Неверный код доступа")
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": token, "expires_at": expires})
}

func (a *API) requirePartner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Partner "))
		if err := a.partner.Authorize(r.Context(), token); err != nil {
			writeError(w, http.StatusUnauthorized, "partner_unauthorized", "Войдите в кабинет студии заново")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) PartnerBookings(w http.ResponseWriter, r *http.Request) {
	views, err := a.partner.Bookings(r.Context(), a.now())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"bookings": views})
}

func (a *API) PartnerCheckIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	view, err := a.partner.CheckIn(r.Context(), req.Code)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, view)
	case errors.Is(err, service.ErrBadCheckinCode):
		writeError(w, http.StatusBadRequest, "invalid_code", "Это не код СпортСлота: нужен QR-пропуск или 6 цифр")
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись с таким кодом не найдена")
	case errors.Is(err, service.ErrNotActive):
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"code": "already_checked_in", "error": "Посещение по этому коду уже отмечено", "booking": view})
	default:
		writeServiceError(w, err)
	}
}

func (a *API) PartnerRemind(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись не найдена")
		return
	}
	if err := a.partner.SendDemoReminder(r.Context(), id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parsePoint(latRaw, lonRaw string) (float64, float64, bool) {
	var lat, lon float64
	if latRaw == "" || lonRaw == "" {
		return 0, 0, false
	}
	if err := json.Unmarshal([]byte(latRaw), &lat); err != nil {
		return 0, 0, false
	}
	if err := json.Unmarshal([]byte(lonRaw), &lon); err != nil {
		return 0, 0, false
	}
	return lat, lon, lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}
