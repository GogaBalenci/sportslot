package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultDGISURL — облачный адрес Routing API 2ГИС.
const DefaultDGISURL = "https://routing.api.2gis.com"

// В синхронном режиме 2ГИС принимает до 25 точек отправления или прибытия.
const batchSize = 24

// DGIS — матрица расстояний 2ГИС (POST /get_dist_matrix).
// Оплачивается каждая пара точек, поэтому ответы кэшируются: пешеходное время
// почти не меняется, автомобильное и транспортное зависят от пробок и расписания.
type DGIS struct {
	baseURL string
	key     string
	client  *http.Client
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]cached

	pairs    atomic.Int64
	lastWarn atomic.Int64
}

type cached struct {
	trip    Trip
	expires time.Time
}

func NewDGIS(baseURL, key string) *DGIS {
	if baseURL == "" {
		baseURL = DefaultDGISURL
	}
	return &DGIS{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		client:  &http.Client{Timeout: 5 * time.Second},
		now:     time.Now,
		cache:   map[string]cached{},
	}
}

func ttl(mode Mode) time.Duration {
	if mode == Walking {
		return 24 * time.Hour
	}
	return 15 * time.Minute
}

// Точка пользователя округляется примерно до 150–200 м: соседние запросы
// попадают в кэш, а на минутах в пути это не сказывается.
func cacheKey(mode Mode, from, to Point) string {
	r := func(x float64) float64 { return math.Round(x*500) / 500 }
	return fmt.Sprintf("%s|%.3f,%.3f|%.5f,%.5f", mode, r(from.Lat), r(from.Lon), to.Lat, to.Lon)
}

func (d *DGIS) Trips(ctx context.Context, from Point, to []Point, mode Mode) []Trip {
	trips := make([]Trip, len(to))
	var missing []int
	now := d.now()

	d.mu.Lock()
	for i, p := range to {
		if c, ok := d.cache[cacheKey(mode, from, p)]; ok && now.Before(c.expires) {
			trips[i] = c.trip
		} else {
			missing = append(missing, i)
		}
	}
	d.mu.Unlock()

	for start := 0; start < len(missing); start += batchSize {
		end := start + batchSize
		if end > len(missing) {
			end = len(missing)
		}
		idx := missing[start:end]
		points := make([]Point, len(idx))
		for j, i := range idx {
			points[j] = to[i]
		}
		got, err := d.request(ctx, from, points, mode)
		if err != nil {
			d.warn(err)
		}
		d.mu.Lock()
		if len(d.cache) > 20000 {
			d.cache = map[string]cached{}
		}
		for j, i := range idx {
			if got != nil && got[j] != nil {
				trips[i] = *got[j]
				d.cache[cacheKey(mode, from, to[i])] = cached{trip: *got[j], expires: now.Add(ttl(mode))}
			} else {
				trips[i] = Estimate(from, to[i], mode)
			}
		}
		d.mu.Unlock()
	}
	return trips
}

type dgisPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type dgisRequest struct {
	Points          []dgisPoint            `json:"points"`
	Sources         []int                  `json:"sources"`
	Targets         []int                  `json:"targets"`
	Transport       string                 `json:"transport"`
	PublicTransport map[string]interface{} `json:"public_transport_params,omitempty"`
}

type dgisResponse struct {
	Routes []struct {
		Status   string `json:"status"`
		SourceID int    `json:"source_id"`
		TargetID int    `json:"target_id"`
		Distance int    `json:"distance"`
		Duration int    `json:"duration"`
	} `json:"routes"`
}

// request возвращает поездку для каждой точки; nil — маршрут не построен.
func (d *DGIS) request(ctx context.Context, from Point, to []Point, mode Mode) ([]*Trip, error) {
	body := dgisRequest{
		Points:  []dgisPoint{{from.Lat, from.Lon}},
		Sources: []int{0},
	}
	for i, p := range to {
		body.Points = append(body.Points, dgisPoint{p.Lat, p.Lon})
		body.Targets = append(body.Targets, i+1)
	}
	switch mode {
	case Transit:
		body.Transport = "public_transport"
		body.PublicTransport = map[string]interface{}{
			"transport":       []string{"bus", "trolleybus", "tram", "shuttle_bus"},
			"enable_schedule": true,
		}
	case Driving:
		body.Transport = "driving"
	default:
		body.Transport = "walking"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("key", d.key)
	q.Set("version", "2.0")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+"/get_dist_matrix?"+q.Encode(), bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("build request")
	}
	req.Header.Set("Content-Type", "application/json")

	d.pairs.Add(int64(len(to)))
	resp, err := d.client.Do(req)
	if err != nil {
		// Текст url.Error содержит адрес запроса вместе с ключом — в логи он не попадает.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("2gis matrix: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusNoContent {
		return make([]*Trip, len(to)), nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("2gis matrix: status %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	var parsed dgisResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("2gis matrix: decode: %v", err)
	}
	out := make([]*Trip, len(to))
	for _, r := range parsed.Routes {
		i := r.TargetID - 1
		if r.Status != "OK" || i < 0 || i >= len(to) {
			continue
		}
		out[i] = &Trip{Seconds: r.Duration, Meters: r.Distance}
	}
	return out, nil
}

// warn пишет в лог не чаще раза в минуту, чтобы недоступный сервис не засорял логи.
func (d *DGIS) warn(err error) {
	now := d.now().Unix()
	last := d.lastWarn.Load()
	if now-last < 60 || !d.lastWarn.CompareAndSwap(last, now) {
		return
	}
	log.Printf("routing: %v; travel times are estimated (pairs requested since start: %d)", err, d.pairs.Load())
}

// Pairs — сколько пар точек запрошено с момента запуска (для контроля лимита ключа).
func (d *DGIS) Pairs() int64 { return d.pairs.Load() }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
