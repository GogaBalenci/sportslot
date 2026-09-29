package handler

import (
	"context"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"sportslot/internal/auth"
)

type ctxKey string

const ctxKeyUser ctxKey = "user"

// User — кто делает запрос к API.
type User struct {
	MaxUserID string
	Name      string
}

func userFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(User)
	return u, ok
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, ww.Status(), time.Since(start).Round(time.Millisecond))
	})
}

// UserAuth определяет пользователя. Основной способ — подписанные данные запуска
// мини-приложения MAX в заголовке `Authorization: tma <initData>`.
// Заголовок X-MAX-User-ID принимается только для тестовых учётных записей
// (проверки DATA-API) и в режиме разработки.
type UserAuth struct {
	BotToken  string
	MaxAge    time.Duration
	TestUsers map[string]bool
	Dev       bool
}

func (a UserAuth) resolve(r *http.Request) (User, int, string) {
	if raw := r.Header.Get("Authorization"); raw != "" {
		scheme, data, _ := strings.Cut(raw, " ")
		if !strings.EqualFold(scheme, "tma") {
			return User{}, http.StatusUnauthorized, "Неподдерживаемая схема авторизации"
		}
		launch, err := auth.Validate(strings.TrimSpace(data), a.BotToken, a.MaxAge, time.Now())
		if err != nil {
			if err == auth.ErrExpired {
				return User{}, http.StatusUnauthorized, "Сессия устарела, откройте приложение заново"
			}
			return User{}, http.StatusUnauthorized, "Подпись данных MAX не прошла проверку"
		}
		return User{MaxUserID: launch.User.MaxUserID(), Name: launch.User.Name()}, 0, ""
	}
	if id := strings.TrimSpace(r.Header.Get("X-MAX-User-ID")); id != "" {
		if a.TestUsers[id] || a.Dev {
			return User{MaxUserID: id, Name: r.Header.Get("X-MAX-User-Name")}, 0, ""
		}
		return User{}, http.StatusUnauthorized, "X-MAX-User-ID принимается только для тестовых учётных записей"
	}
	return User{}, http.StatusUnauthorized, "Нужна авторизация: откройте приложение в MAX"
}

func (a UserAuth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, status, msg := a.resolve(r)
		if status != 0 {
			writeError(w, status, "unauthorized", msg)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUser, user)))
	})
}

// limiter — простое ограничение попыток по IP (вход партнёра).
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string][]time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, hits: map[string][]time.Time{}}
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	return true
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		first, _, _ := strings.Cut(fwd, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
