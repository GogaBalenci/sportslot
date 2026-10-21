// Package geocode переводит адрес в Ростове-на-Дону в координаты через
// геокодер 2ГИС (тот же ключ, что и для времени в пути).
package geocode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultURL — облачный Catalog API 2ГИС.
const DefaultURL = "https://catalog.api.2gis.com"

var (
	ErrNotFound    = errors.New("address not found")
	ErrUnavailable = errors.New("geocoder is unavailable")
)

// Ростов с запасом на окраины: адрес из другого города не принимаем.
const minLat, maxLat, minLon, maxLon = 47.13, 47.40, 39.40, 39.92

type Place struct {
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Address string  `json:"address"`
}

type Geocoder struct {
	baseURL string
	key     string
	client  *http.Client
}

func New(baseURL, key string) *Geocoder {
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Geocoder{baseURL: strings.TrimRight(baseURL, "/"), key: key, client: &http.Client{Timeout: 5 * time.Second}}
}

func (g *Geocoder) Enabled() bool { return g != nil && g.key != "" }

var cityPrefix = regexp.MustCompile(`(?i)^\s*(г\.?\s*)?ростов(-на-дону)?[,\s]*`)

// Query нормализует запрос: убирает «Ростов-на-Дону» из начала и добавляет
// его обратно один раз, чтобы поиск шёл по городу.
func Query(text string) string {
	t := strings.TrimSpace(cityPrefix.ReplaceAllString(text, ""))
	if t == "" {
		return ""
	}
	return "Ростов-на-Дону, " + t
}

type response struct {
	Meta struct {
		Code  int `json:"code"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"meta"`
	Result struct {
		Items []struct {
			FullName    string `json:"full_name"`
			AddressName string `json:"address_name"`
			Name        string `json:"name"`
			Point       *struct {
				Lat float64 `json:"lat"`
				Lon float64 `json:"lon"`
			} `json:"point"`
		} `json:"items"`
	} `json:"result"`
}

// Find возвращает первое найденное место в Ростове.
func (g *Geocoder) Find(ctx context.Context, text string) (*Place, error) {
	if !g.Enabled() {
		return nil, ErrUnavailable
	}
	q := Query(text)
	if q == "" {
		return nil, ErrNotFound
	}
	params := url.Values{}
	params.Set("q", q)
	params.Set("fields", "items.point")
	params.Set("key", g.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL+"/3.0/items/geocode?"+params.Encode(), nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	resp, err := g.client.Do(req)
	if err != nil {
		// url.Error содержит адрес запроса с ключом — наружу отдаём только причину.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}
	if parsed.Meta.Code == 404 {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK || (parsed.Meta.Code != 0 && parsed.Meta.Code != 200) {
		msg := ""
		if parsed.Meta.Error != nil {
			msg = parsed.Meta.Error.Message
		}
		return nil, fmt.Errorf("%w: code %d %s", ErrUnavailable, parsed.Meta.Code, msg)
	}
	for _, item := range parsed.Result.Items {
		p := item.Point
		if p == nil || p.Lat < minLat || p.Lat > maxLat || p.Lon < minLon || p.Lon > maxLon {
			continue
		}
		address := item.AddressName
		if address == "" {
			address = item.Name
		}
		if address == "" {
			address = strings.TrimPrefix(item.FullName, "Ростов-на-Дону, ")
		}
		return &Place{Lat: p.Lat, Lon: p.Lon, Address: address}, nil
	}
	return nil, ErrNotFound
}
