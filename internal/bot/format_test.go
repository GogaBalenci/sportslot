package bot

import (
	"testing"
	"time"

	"sportslot/internal/catalog"
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
