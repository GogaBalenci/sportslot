package bot

import (
	"testing"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/routing"
	"sportslot/internal/service"
)

func TestDayLabel(t *testing.T) {
	now := time.Date(2026, 9, 29, 20, 0, 0, 0, catalog.Moscow)
	cases := map[time.Time]string{
		now.Add(5 * time.Hour):                              "завтра",
		now.Add(30 * time.Minute):                           "сегодня",
		time.Date(2026, 10, 1, 19, 0, 0, 0, catalog.Moscow): "чт, 1 октября",
	}
	for in, want := range cases {
		if got := dayLabel(in, now); got != want {
			t.Errorf("dayLabel(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestSeatsLabel(t *testing.T) {
	cases := map[int]string{0: "мест нет", 1: "осталось 1 место", 2: "осталось 2 места", 5: "5 мест", 22: "22 места", 11: "11 мест"}
	for n, want := range cases {
		if got := seatsLabel(n); got != want {
			t.Errorf("seatsLabel(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestOpeningHours(t *testing.T) {
	cases := map[string]string{
		"Mo-Fr 07:00-22:00; Sa-Su 09:00-20:00":          "пн–пт 07:00–22:00; сб, вс 09:00–20:00",
		"Mo,We,Fr 11:00-22:00; Tu,Th,Sa 11:00-22:00":    "пн–сб 11:00–22:00",
		"Mo,We 10:00-20:00; Tu 09:00-12:00,14:00-18:00": "пн, ср 10:00–20:00; вт 09:00–12:00, 14:00–18:00",
		"Mo-Su 08:00-23:00; PH off":                     "пн–вс 08:00–23:00",
		"24/7":                                          "круглосуточно",
	}
	for in, want := range cases {
		if got := openingHours(in); got != want {
			t.Errorf("openingHours(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTravelLabel(t *testing.T) {
	cases := map[string]*service.Travel{
		"12 мин пешком":          {Mode: routing.Walking, Minutes: 12},
		"≈ 25 мин на транспорте": {Mode: routing.Transit, Minutes: 25, Estimated: true},
		"1 ч 05 мин на машине":   {Mode: routing.Driving, Minutes: 65},
	}
	for want, tr := range cases {
		if got := travelLabel(tr); got != want {
			t.Errorf("travelLabel = %q, want %q", got, want)
		}
	}
}

func TestParseTravel(t *testing.T) {
	if m, l := parseTravel("на автобусе минут 25"); m != routing.Transit || l != 25 {
		t.Errorf("got %s %d", m, l)
	}
	if m, l := parseTravel("пешком"); m != routing.Walking || l != 15 {
		t.Errorf("got %s %d", m, l)
	}
	if m, _ := parseTravel("как получится"); m != "" {
		t.Errorf("unexpected mode %s", m)
	}
}

func TestPhoneLabel(t *testing.T) {
	cases := map[string]string{
		"29 61 0 61":                      "+7 863 296-10-61",
		"+7 (863) 201-02-03":              "+7 863 201-02-03",
		"8 918 555-44-33; +7 863 2000000": "+7 918 555-44-33",
		"доб. 12":                         "доб. 12",
	}
	for in, want := range cases {
		if got := phoneLabel(in); got != want {
			t.Errorf("phoneLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRouteURL(t *testing.T) {
	if got := routeURL(nil, 47.1, 39.2); got != "https://yandex.ru/maps/?rtext=~47.100000,39.200000&rtt=auto" {
		t.Errorf("got %q", got)
	}
	got := routeURL(&Origin{Lat: 47.2, Lon: 39.7, Mode: routing.Walking}, 47.1, 39.2)
	if got != "https://yandex.ru/maps/?rtext=47.200000,39.700000~47.100000,39.200000&rtt=pd" {
		t.Errorf("got %q", got)
	}
}
