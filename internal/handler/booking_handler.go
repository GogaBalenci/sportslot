package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"sportslot/internal/model"
	"sportslot/internal/service"

	"github.com/go-chi/chi/v5"
)

type BookingHandler struct {
	booking *service.BookingService
}

func NewBookingHandler(booking *service.BookingService) *BookingHandler {
	return &BookingHandler{booking: booking}
}

type createBookingRequest struct {
	SlotID        string `json:"slot_id"`
	SourceChannel string `json:"source_channel"`
}

func (h *BookingHandler) Create(w http.ResponseWriter, r *http.Request) {
	maxUserID, ok := maxUserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_header", "Заголовок X-MAX-User-ID обязателен")
		return
	}

	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}

	if req.SlotID == "" {
		writeError(w, http.StatusBadRequest, "missing_slot_id", "Поле slot_id обязательно")
		return
	}

	channel := model.SourceChannel(req.SourceChannel)
	if channel != model.SourceChannelBot && channel != model.SourceChannelMiniApp {
		writeError(w, http.StatusBadRequest, "invalid_source_channel", "source_channel должен быть 'bot' или 'miniapp'")
		return
	}

	booking, err := h.booking.CreateBooking(r.Context(), service.CreateBookingInput{
		MaxUserID:     maxUserID,
		SlotID:        req.SlotID,
		SourceChannel: channel,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrQuotaExceeded):
			writeError(w, http.StatusConflict, "quota_exceeded", "Квота на этот слот уже исчерпана")
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, "slot_not_found", "Слот не найден")
		default:
			writeError(w, http.StatusBadRequest, "booking_failed", err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, booking)
}

func (h *BookingHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	maxUserID, ok := maxUserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_header", "Заголовок X-MAX-User-ID обязателен")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Не указан id брони")
		return
	}

	if err := h.booking.CancelBooking(r.Context(), id, maxUserID); err != nil {
		if errors.Is(err, service.ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", "Бронь принадлежит другому пользователю")
			return
		}
		if errors.Is(err, service.ErrNotFound) {
			writeError(w, http.StatusNotFound, "booking_not_found", "Бронь не найдена")
			return
		}
		writeError(w, http.StatusInternalServerError, "cancel_failed", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type rescheduleRequest struct {
	NewSlotID string `json:"new_slot_id"`
}

func (h *BookingHandler) Reschedule(w http.ResponseWriter, r *http.Request) {
	maxUserID, ok := maxUserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_header", "Заголовок X-MAX-User-ID обязателен")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing_id", "Не указан id брони")
		return
	}

	var req rescheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Некорректное тело запроса")
		return
	}

	if req.NewSlotID == "" {
		writeError(w, http.StatusBadRequest, "missing_new_slot_id", "Поле new_slot_id обязательно")
		return
	}

	booking, err := h.booking.RescheduleBooking(r.Context(), id, req.NewSlotID, maxUserID)
	if err != nil {
		if errors.Is(err, service.ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", "Бронь принадлежит другому пользователю")
			return
		}
		switch {
		case errors.Is(err, service.ErrQuotaExceeded):
			writeError(w, http.StatusConflict, "quota_exceeded", "Квота на новый слот уже исчерпана")
		case errors.Is(err, service.ErrNotFound):
			writeError(w, http.StatusNotFound, "booking_or_slot_not_found", "Бронь или слот не найдены")
		default:
			writeError(w, http.StatusBadRequest, "reschedule_failed", err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, booking)
}

func (h *BookingHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	maxUserID, ok := maxUserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing_header", "Заголовок X-MAX-User-ID обязателен")
		return
	}

	userID := chi.URLParam(r, "user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "missing_user_id", "Не указан user_id")
		return
	}

	bookings, err := h.booking.ListUserBookings(r.Context(), userID, maxUserID)
	if err != nil {
		if errors.Is(err, service.ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", "Нельзя просматривать чужие брони")
			return
		}
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, bookings)
}
