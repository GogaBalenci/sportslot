package handler

import (
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sportslot/internal/model"
	"sportslot/internal/payments"
	"sportslot/internal/service"
)

// --- Клиент: отзывы, друг, оплата, абонементы ---

func (a *API) Review(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Text string `json:"text"`
	}
	if !validID(id) || !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_request", "Некорректный запрос")
		return
	}
	if err := a.bookings.Review(r.Context(), id, u.MaxUserID, req.Text); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AddFriend — второе место на то же занятие и ссылка для друга.
func (a *API) AddFriend(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Запись не найдена")
		return
	}
	place, err := a.bookings.AddFriend(r.Context(), id, u.MaxUserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

func (a *API) Friend(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Запись не найдена")
		return
	}
	place, err := a.bookings.Friend(r.Context(), id, u.MaxUserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"friend": place})
}

func (a *API) ClaimFriend(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	view, err := a.bookings.ClaimFriend(r.Context(), req.Token, u.MaxUserID, u.Name)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			writeError(w, http.StatusNotFound, "invite_not_found", "Приглашение уже принято или запись отменена")
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (a *API) PayBooking(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Запись не найдена")
		return
	}
	p, err := a.commerce.PayTrial(r.Context(), u.MaxUserID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) Payment(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Платёж не найден")
		return
	}
	p, err := a.commerce.Payment(r.Context(), u.MaxUserID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) MyPasses(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	list, err := a.commerce.MyPasses(r.Context(), u.MaxUserID, u.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"passes": list})
}

func (a *API) BuyPass(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Абонемент не найден")
		return
	}
	order, err := a.commerce.RequestPass(r.Context(), u.MaxUserID, u.Name, id)
	if err != nil && (order == nil || !errors.Is(err, service.ErrPaymentFailed)) {
		writeServiceError(w, err)
		return
	}
	// Платёжная система не ответила: заявка сохранена, оплатить можно позже.
	writeJSON(w, http.StatusOK, map[string]interface{}{"user_pass": order.UserPass, "payment": order.Payment,
		"created": order.Created, "payment_error": err != nil})
}

func (a *API) PayPass(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Абонемент не найден")
		return
	}
	p, err := a.commerce.PayPass(r.Context(), u.MaxUserID, id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) CancelPassRequest(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	id := chi.URLParam(r, "id")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Абонемент не найден")
		return
	}
	if err := a.commerce.CancelPassRequest(r.Context(), u.MaxUserID, id); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Уведомления платёжных систем ---

func (a *API) YooKassaWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
	if err != nil || !validID(venueParam(r)) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное уведомление")
		return
	}
	err = a.commerce.YooKassaNotification(r.Context(), venueParam(r), raw)
	switch {
	case err == nil, errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrInvalidRequest):
		// Неизвестный платёж повторять бессмысленно — отвечаем 200, чтобы ЮKassa не слала его снова.
		w.WriteHeader(http.StatusOK)
	default:
		log.Printf("yookassa notification: %v", err)
		writeError(w, http.StatusInternalServerError, "retry", "Повторите позже")
	}
}

func (a *API) TBankWebhook(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<16))
	if err != nil || !validID(venueParam(r)) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	err = a.commerce.TBankNotification(r.Context(), venueParam(r), raw)
	switch {
	case err == nil, errors.Is(err, service.ErrNotFound):
		// Т-Касса ждёт в ответ ровно «OK», иначе повторяет уведомление.
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("OK"))
	case errors.Is(err, payments.ErrBadSignature):
		http.Error(w, "bad signature", http.StatusForbidden)
	default:
		log.Printf("tbank notification: %v", err)
		http.Error(w, "retry", http.StatusInternalServerError)
	}
}

// --- Кабинет: настройки, абонементы, оплаты, отзывы ---

func (a *API) StudioSettings(w http.ResponseWriter, r *http.Request) {
	v, err := a.commerce.Settings(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) StudioSaveSettings(w http.ResponseWriter, r *http.Request) {
	var in service.SettingsInput
	if !decode(w, r, &in) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	v, err := a.commerce.SaveSettings(r.Context(), accessFrom(r.Context()), venueParam(r), in)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRequest) {
			writeError(w, http.StatusBadRequest, "invalid_settings", "Проверьте платёжную систему, идентификатор магазина и ключ")
			return
		}
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) StudioCheckPayments(w http.ResponseWriter, r *http.Request) {
	if err := a.commerce.CheckPayments(r.Context(), accessFrom(r.Context()), venueParam(r)); err != nil {
		if errors.Is(err, service.ErrPaymentFailed) {
			writeError(w, http.StatusBadGateway, "payment_check_failed", "Платёжная система не приняла ключи: проверьте shopId и секретный ключ")
			return
		}
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) StudioCommerce(w http.ResponseWriter, r *http.Request) {
	out, err := a.commerce.StudioCommerce(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) StudioSavePass(w http.ResponseWriter, r *http.Request) {
	var p model.Pass
	if !decode(w, r, &p) {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}
	p.ID = chi.URLParam(r, "passID")
	if p.ID != "" && !validID(p.ID) {
		writeError(w, http.StatusNotFound, "not_found", "Абонемент не найден")
		return
	}
	saved, err := a.commerce.SavePass(r.Context(), accessFrom(r.Context()), venueParam(r), p)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRequest) {
			writeError(w, http.StatusBadRequest, "invalid_pass", "Название 2–120 символов, 1–100 занятий, срок 1–365 дней, цена от 1 ₽")
			return
		}
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (a *API) StudioConfirmPass(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userPassID")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
		return
	}
	up, err := a.commerce.ConfirmPass(r.Context(), accessFrom(r.Context()), venueParam(r), id)
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, up)
}

func (a *API) StudioDeclinePass(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "userPassID")
	if !validID(id) {
		writeError(w, http.StatusNotFound, "not_found", "Заявка не найдена")
		return
	}
	if err := a.commerce.DeclinePass(r.Context(), accessFrom(r.Context()), venueParam(r), id); err != nil {
		writeStudioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) StudioReviews(w http.ResponseWriter, r *http.Request) {
	out, err := a.commerce.Reviews(r.Context(), accessFrom(r.Context()), venueParam(r))
	if err != nil {
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
