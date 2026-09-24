package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type ctxKey string

const ctxKeyMaxUserID ctxKey = "max_user_id"

// RequestLogger логирует метод, путь, статус и длительность каждого запроса.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, ww.Status(), time.Since(start))
	})
}

// RequireMaxUserID достаёт заголовок X-MAX-User-ID и кладёт его в контекст.
// Если заголовок отсутствует - возвращает 401.
func RequireMaxUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-MAX-User-ID")
		if userID == "" {
			writeError(w, http.StatusUnauthorized, "missing_header", "Заголовок X-MAX-User-ID обязателен")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyMaxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalMaxUserID достаёт заголовок X-MAX-User-ID, если он есть, но не требует его.
func OptionalMaxUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-MAX-User-ID")
		if userID != "" {
			ctx := context.WithValue(r.Context(), ctxKeyMaxUserID, userID)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func maxUserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(ctxKeyMaxUserID).(string)
	return v, ok
}
