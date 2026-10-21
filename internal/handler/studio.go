package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"sportslot/internal/geocode"
	"sportslot/internal/model"
	"sportslot/internal/service"
)

const ctxKeyAccess ctxKey = "studio_access"

// LocationListener продолжает сценарий бота, когда геолокация пришла из мини-приложения.
type LocationListener interface {
	LocationFromApp(ctx context.Context, maxUserID, name string, lat, lon float64) (sport, when string)
}

func accessFrom(ctx context.Context) service.Access {
	a, _ := ctx.Value(ctxKeyAccess).(service.Access)
	return a
}

// resolveStudio определяет доступ к кабинету: по токену общего демо-кода
// («Authorization: Partner …») или по подписанным данным MAX администратора.
func (a *API) resolveStudio(r *http.Request) (service.Access, int, string, string) {
	if token, ok := partnerToken(r); ok {
		if err := a.partner.Authorize(r.Context(), token); err != nil {
			return service.Access{}, http.StatusUnauthorized, "partner_unauthorized", "Войдите в кабинет студии заново"
		}
		acc, err := a.studio.ResolveDemo(r.Context())
		if err != nil {
			return service.Access{}, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка"
		}
		return acc, 0, "", ""
	}
	user, status, msg := a.auth.resolve(r)
	if status != 0 {
		return service.Access{}, status, "unauthorized", msg
	}
	acc, err := a.studio.ResolveUser(r.Context(), user.MaxUserID, user.Name)
	if err != nil {
		return service.Access{}, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка"
	}
	return acc, 0, "", ""
}

func partnerToken(r *http.Request) (string, bool) {
	raw := r.Header.Get("Authorization")
	if len(raw) > 8 && raw[:8] == "Partner " {
		return raw[8:], true
	}
	return "", false
}

func (a *API) requireStudio(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acc, status, code, msg := a.resolveStudio(r)
		if status != 0 {
			writeError(w, status, code, msg)
			return
		}
		if len(acc.VenueIDs) == 0 && !acc.Platform {
			writeError(w, http.StatusForbidden, "no_studio", "Вы не администратор студии. Попросите ссылку-приглашение")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyAccess, acc)))
	})
}

func writeStudioError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNotPlatform):
		writeError(w, http.StatusForbidden, "platform_only", "Доступно только владельцу платформы")
	case errors.Is(err, service.ErrInvalidInvite):
		writeError(w, http.StatusBadRequest, "invalid_invite", "Приглашение недействительно или уже использовано")
	case errors.Is(err, service.ErrDemoReadOnly):
		writeError(w, http.StatusForbidden, "demo_read_only", "В демо-доступе профиль студии только просматривается")
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "Нет доступа к этой студии")
	case errors.Is(err, service.ErrBookingClosed):
		writeError(w, http.StatusConflict, "booking_closed", "Запись на это занятие закрыта")
	case errors.Is(err, geocode.ErrNotFound):
		writeError(w, http.StatusBadRequest, "address_not_found", "Адрес в Ростове-на-Дону не найден")
	case errors.Is(err, geocode.ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, "geocoder_unavailable", "Поиск адреса временно недоступен")
	default:
		writeServiceError(w, err)
	}
}

// StudioMe — студии, доступные пользователю. Не требует членства: мини-приложение
// по ответу решает, показывать кабинет или вход по коду.
func (a *API) StudioMe(w http.ResponseWriter, r *http.Request) {
	acc, status, code, msg := a.resolveStudio(r)
	if status != 0 {
		writeError(w, status, code, msg)
		return
	}
	studios, err := a.studio.Studios(r.Context(), acc)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"studios": studios, "platform": acc.Platform, "demo": acc.Demo})
}

func (a *API) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	venue, err := a.studio.AcceptInvite(r.Context(), u.MaxUserID, u.Name, req.Token)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, venue)
}

func venueParam(r *http.Request) string { return chi.URLParam(r, "venueID") }

func (a *API) StudioBookings(w http.ResponseWriter, r *http.Request) {
	views, err := a.studio.Bookings(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"bookings": views})
}

func (a *API) StudioBooking(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "bookingID")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись не найдена")
		return
	}
	card, err := a.studio.Booking(r.Context(), accessFrom(r.Context()), id)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

func (a *API) StudioRemind(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "bookingID")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись не найдена")
		return
	}
	if err := a.studio.Remind(r.Context(), accessFrom(r.Context()), id); err != nil {
		writeStudioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) StudioCheckIn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	view, err := a.studio.CheckIn(r.Context(), accessFrom(r.Context()), req.Code)
	a.writeCheckIn(w, view, err)
}

func (a *API) writeCheckIn(w http.ResponseWriter, view *model.BookingView, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, view)
	case errors.Is(err, service.ErrTrainingCheckin):
		writeJSON(w, http.StatusOK, map[string]interface{}{"training": true,
			"message": "Учебный пропуск отсканирован — сканер работает. С настоящим пропуском клиент сразу получит вопрос «Как прошло?»."})
	case errors.Is(err, service.ErrBadCheckinCode):
		writeError(w, http.StatusBadRequest, "invalid_code", "Это не код СпортСлота: нужен QR-пропуск или 6 цифр")
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "booking_not_found", "Запись с таким кодом в вашей студии не найдена")
	case errors.Is(err, service.ErrNotActive):
		writeJSON(w, http.StatusConflict, map[string]interface{}{
			"code": "already_checked_in", "error": "Посещение по этому коду уже отмечено", "booking": view})
	default:
		writeServiceError(w, err)
	}
}

func (a *API) StudioTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := a.studio.Templates(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"templates": list})
}

func (a *API) StudioSaveTemplate(w http.ResponseWriter, r *http.Request) {
	var t model.Template
	if !decode(w, r, &t) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	t.VenueID = venueParam(r)
	t.ID = chi.URLParam(r, "templateID")
	if t.ID != "" && !validID(t.ID) {
		writeError(w, http.StatusNotFound, "template_not_found", "Занятие в расписании не найдено")
		return
	}
	kept, err := a.studio.SaveTemplate(r.Context(), accessFrom(r.Context()), &t)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"template": t, "kept_with_bookings": kept})
}

func (a *API) StudioDeleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "templateID")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "template_not_found", "Занятие в расписании не найдено")
		return
	}
	kept, err := a.studio.DeleteTemplate(r.Context(), accessFrom(r.Context()), venueParam(r), id)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"kept_with_bookings": kept})
}

func (a *API) StudioSlots(w http.ResponseWriter, r *http.Request) {
	list, err := a.studio.Slots(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"slots": list})
}

func (a *API) StudioCloseSlot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Closed bool `json:"closed"`
	}
	slotID := chi.URLParam(r, "slotID")
	if !validID(slotID) || !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if err := a.studio.CloseSlot(r.Context(), accessFrom(r.Context()), venueParam(r), slotID, req.Closed); err != nil {
		writeStudioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) StudioCancelSlot(w http.ResponseWriter, r *http.Request) {
	slotID := chi.URLParam(r, "slotID")
	if !validID(slotID) {
		writeError(w, http.StatusNotFound, "slot_not_found", "Занятие не найдено")
		return
	}
	n, err := a.studio.CancelSlot(r.Context(), accessFrom(r.Context()), venueParam(r), slotID)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"notified": n})
}

func (a *API) StudioStats(w http.ResponseWriter, r *http.Request) {
	st, err := a.studio.Stats(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (a *API) StudioProfile(w http.ResponseWriter, r *http.Request) {
	var p service.VenueProfile
	if !decode(w, r, &p) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	v, err := a.studio.UpdateProfile(r.Context(), accessFrom(r.Context()), venueParam(r), p)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) StudioThreads(w http.ResponseWriter, r *http.Request) {
	list, err := a.studio.Threads(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"threads": list})
}

func (a *API) StudioThread(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if !validID(userID) {
		writeError(w, http.StatusNotFound, "user_not_found", "Клиент не найден")
		return
	}
	th, err := a.studio.Thread(r.Context(), accessFrom(r.Context()), venueParam(r), userID)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, th)
}

func (a *API) StudioReply(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	var req struct {
		Text string `json:"text"`
	}
	if !validID(userID) || !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	m, err := a.studio.Reply(r.Context(), accessFrom(r.Context()), venueParam(r), userID, req.Text)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// StudioMessageRef — куда ведёт кнопка из уведомления бота о новом сообщении.
func (a *API) StudioMessageRef(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "messageID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "message_not_found", "Сообщение не найдено")
		return
	}
	m, err := a.studio.ThreadByMessage(r.Context(), accessFrom(r.Context()), id)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"venue_id": m.VenueID, "user_id": m.UserID})
}

// --- Платформа ---

func (a *API) PlatformStudios(w http.ResponseWriter, r *http.Request) {
	list, err := a.studio.PlatformStudios(r.Context(), accessFrom(r.Context()))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"studios": list})
}

func (a *API) PlatformCatalog(w http.ResponseWriter, r *http.Request) {
	list, err := a.studio.SearchCatalog(r.Context(), accessFrom(r.Context()), r.URL.Query().Get("q"))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"venues": list})
}

func (a *API) PlatformAddStudio(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VenueID string `json:"venue_id"`
		service.NewStudio
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	acc := accessFrom(r.Context())
	var (
		v   *model.Venue
		err error
	)
	if req.VenueID != "" {
		if !validID(req.VenueID) {
			writeError(w, http.StatusNotFound, "venue_not_found", "Площадка не найдена")
			return
		}
		v, err = a.studio.Connect(r.Context(), acc, req.VenueID)
	} else {
		v, err = a.studio.CreateStudio(r.Context(), acc, req.NewStudio)
	}
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (a *API) PlatformInvite(w http.ResponseWriter, r *http.Request) {
	if !validID(venueParam(r)) {
		writeError(w, http.StatusNotFound, "venue_not_found", "Площадка не найдена")
		return
	}
	link, err := a.studio.CreateInvite(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (a *API) PlatformRemoveAdmin(w http.ResponseWriter, r *http.Request) {
	if err := a.studio.RemoveAdmin(r.Context(), accessFrom(r.Context()), venueParam(r), chi.URLParam(r, "maxUserID")); err != nil {
		writeStudioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Клиент: переписка со студией и геолокация из мини-приложения ---

func (a *API) ClientMessages(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	venueID := r.URL.Query().Get("venue_id")
	if !validID(venueID) {
		writeError(w, http.StatusBadRequest, "invalid_venue_id", "Параметр venue_id должен быть UUID")
		return
	}
	msgs, err := a.studio.ClientThread(r.Context(), u.MaxUserID, venueID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"messages": msgs})
}

func (a *API) ClientSendMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		VenueID string `json:"venue_id"`
		Text    string `json:"text"`
	}
	if !decode(w, r, &req) || !validID(req.VenueID) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	m, err := a.studio.SendClientMessage(r.Context(), u.MaxUserID, u.Name, req.VenueID, req.Text)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// SaveLocation — геолокация, определённая в мини-приложении (например, на
// компьютере, где кнопка геолокации в чате не работает). Сценарий в боте
// продолжается с этой точки.
func (a *API) SaveLocation(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		Lat     float64 `json:"lat"`
		Lon     float64 `json:"lon"`
		Address string  `json:"address"`
	}
	if !decode(w, r, &req) || req.Lat < 46.5 || req.Lat > 48 || req.Lon < 38.5 || req.Lon > 41 {
		writeError(w, http.StatusBadRequest, "invalid_location", "Точка должна быть в Ростове-на-Дону и окрестностях")
		return
	}
	if err := a.bookings.Store().SaveLocation(r.Context(), u.MaxUserID, req.Lat, req.Lon, req.Address); err != nil {
		writeServiceError(w, err)
		return
	}
	resp := map[string]string{}
	if a.location != nil {
		resp["sport"], resp["when"] = a.location.LocationFromApp(r.Context(), u.MaxUserID, u.Name, req.Lat, req.Lon)
	}
	writeJSON(w, http.StatusOK, resp)
}
