package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

var center = Point{47.2225, 39.7185}

func TestEstimateIsSane(t *testing.T) {
	far := Point{47.2893, 39.7147} // около 7,4 км по прямой
	walk := Estimate(center, far, Walking).Minutes()
	transit := Estimate(center, far, Transit).Minutes()
	drive := Estimate(center, far, Driving).Minutes()
	if walk < 90 || walk > 150 {
		t.Errorf("walking %d min", walk)
	}
	if !(drive < transit && transit < walk) {
		t.Errorf("want driving < transit < walking, got %d %d %d", drive, transit, walk)
	}
	near := Point{47.2235, 39.7185} // около 110 м
	if m := Estimate(center, near, Transit).Minutes(); m > 3 {
		t.Errorf("short transit trip must fall back to walking, got %d min", m)
	}
}

func fake2GIS(t *testing.T, handler func(req dgisRequest) (int, string)) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/get_dist_matrix" || r.URL.Query().Get("key") != "secret" || r.URL.Query().Get("version") != "2.0" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		var req dgisRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		status, body := handler(req)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestDGISRequestAndCache(t *testing.T) {
	srv, calls := fake2GIS(t, func(req dgisRequest) (int, string) {
		if req.Transport != "public_transport" || req.PublicTransport == nil {
			t.Errorf("transit params missing: %+v", req)
		}
		if len(req.Sources) != 1 || req.Sources[0] != 0 || len(req.Targets) != len(req.Points)-1 {
			t.Errorf("bad sources/targets: %+v", req)
		}
		var routes []string
		for _, target := range req.Targets {
			status := "OK"
			if target == 2 {
				status = "ROUTE_NOT_FOUND"
			}
			routes = append(routes, `{"status":"`+status+`","source_id":0,"target_id":`+itoa(target)+`,"distance":1000,"duration":`+itoa(600+target)+`}`)
		}
		return 200, `{"routes":[` + strings.Join(routes, ",") + `]}`
	})
	d := NewDGIS(srv.URL, "secret")
	to := []Point{{47.23, 39.72}, {47.24, 39.73}, {47.25, 39.74}}

	trips := d.Trips(context.Background(), center, to, Transit)
	if trips[0].Seconds != 601 || trips[0].Estimated || trips[2].Seconds != 603 {
		t.Fatalf("unexpected trips: %+v", trips)
	}
	if !trips[1].Estimated {
		t.Fatalf("route not found must be estimated: %+v", trips[1])
	}

	// Повтор из соседней точки (в пределах округления) берётся из кэша, кроме ненайденного маршрута.
	again := d.Trips(context.Background(), Point{center.Lat + 0.0003, center.Lon}, to[:1], Transit)
	if again[0].Seconds != 601 || calls.Load() != 1 {
		t.Fatalf("cache miss: %+v, calls=%d", again, calls.Load())
	}
}

func TestDGISBatchesAndFallsBack(t *testing.T) {
	var sizes []int
	srv, _ := fake2GIS(t, func(req dgisRequest) (int, string) {
		sizes = append(sizes, len(req.Targets))
		return 403, `{"type":"error","error_message":"key limit"}`
	})
	d := NewDGIS(srv.URL, "secret")
	to := make([]Point, 30)
	for i := range to {
		to[i] = Point{47.2 + float64(i)/1000, 39.7}
	}
	trips := d.Trips(context.Background(), center, to, Walking)
	if len(sizes) != 2 || sizes[0] != 24 || sizes[1] != 6 {
		t.Fatalf("want batches 24+6, got %v", sizes)
	}
	for _, tr := range trips {
		if !tr.Estimated || tr.Seconds == 0 {
			t.Fatalf("failed request must give estimates: %+v", tr)
		}
	}
	if d.Pairs() != 30 {
		t.Fatalf("pairs = %d", d.Pairs())
	}
}

func TestErrorDoesNotLeakKey(t *testing.T) {
	d := NewDGIS("http://127.0.0.1:1", "secret")
	_, err := d.request(context.Background(), center, []Point{{47.23, 39.72}}, Walking)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error must not contain the key: %v", err)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
