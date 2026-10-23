package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"sportslot/internal/logo"
	"sportslot/internal/service"
)

// VenueLogo отдаёт логотип студии. Адрес содержит версию, поэтому кэш долгий.
func (a *API) VenueLogo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validID(id) {
		http.NotFound(w, r)
		return
	}
	ct, data, err := a.bookings.Store().Logo(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

func (a *API) StudioUploadLogo(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, logo.MaxUploadBytes+1))
	if err != nil || len(raw) > logo.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "logo_too_large", "Файл больше 5 МБ")
		return
	}
	v, err := a.studio.SetLogo(r.Context(), accessFrom(r.Context()), venueParam(r), raw)
	if err != nil {
		var le *service.ErrLogo
		if errors.As(err, &le) {
			writeError(w, http.StatusBadRequest, "invalid_logo", le.Message)
			return
		}
		writeStudioError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (a *API) StudioDeleteLogo(w http.ResponseWriter, r *http.Request) {
	if err := a.studio.DeleteLogo(r.Context(), accessFrom(r.Context()), venueParam(r)); err != nil {
		writeStudioError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
