// Package auth проверяет данные запуска мини-приложения MAX (initData).
// Алгоритм: https://dev.max.ru/docs/webapps/validation
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("init data is malformed")
	ErrSignature = errors.New("init data signature mismatch")
	ErrExpired   = errors.New("init data is expired")
)

type WebAppUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func (u WebAppUser) MaxUserID() string { return strconv.FormatInt(u.ID, 10) }

func (u WebAppUser) Name() string { return strings.TrimSpace(u.FirstName + " " + u.LastName) }

type LaunchData struct {
	User       WebAppUser
	StartParam string
	AuthDate   time.Time
}

// Validate проверяет подпись строки initData токеном бота и её возраст.
func Validate(initData, botToken string, maxAge time.Duration, now time.Time) (*LaunchData, error) {
	if initData == "" || botToken == "" {
		return nil, ErrMalformed
	}
	values := map[string]string{}
	for _, pair := range strings.Split(initData, "&") {
		key, raw, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, ErrMalformed
		}
		// Ключ должен встречаться один раз: иначе можно подсунуть значение мимо подписи.
		if _, dup := values[key]; dup {
			return nil, ErrMalformed
		}
		value, err := url.QueryUnescape(raw)
		if err != nil {
			return nil, ErrMalformed
		}
		values[key] = value
	}
	gotHash, ok := values["hash"]
	if !ok {
		return nil, ErrMalformed
	}
	delete(values, "hash")

	if !hmac.Equal([]byte(Sign(values, botToken)), []byte(strings.ToLower(gotHash))) {
		return nil, ErrSignature
	}

	ts, err := strconv.ParseInt(values["auth_date"], 10, 64)
	if err != nil {
		return nil, ErrMalformed
	}
	authDate := time.Unix(ts, 0)
	if maxAge > 0 && now.Sub(authDate) > maxAge {
		return nil, ErrExpired
	}

	var user WebAppUser
	if err := json.Unmarshal([]byte(values["user"]), &user); err != nil || user.ID == 0 {
		return nil, ErrMalformed
	}
	return &LaunchData{User: user, StartParam: values["start_param"], AuthDate: authDate}, nil
}

// Sign считает подпись набора параметров (без hash) — нужен и для тестов.
func Sign(values map[string]string, botToken string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+values[k])
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

// BuildInitData собирает подписанную строку initData (для тестов и локальной отладки).
func BuildInitData(values map[string]string, botToken string) string {
	q := url.Values{}
	for k, v := range values {
		q.Set(k, v)
	}
	q.Set("hash", Sign(values, botToken))
	return q.Encode()
}
