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
	if got := openingHours("Mo-Fr 07:00-22:00; Sa-Su 09:00-20:00"); got != "пн-пт 07:00-22:00, сб-вс 09:00-20:00" {
		t.Errorf("got %q", got)
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
