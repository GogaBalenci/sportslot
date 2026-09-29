package bot

import (
	"fmt"
	"strings"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/model"
)

var weekdayShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
var weekdayLong = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
var monthGen = [...]string{"", "января", "февраля", "марта", "апреля", "мая", "июня", "июля",
	"августа", "сентября", "октября", "ноября", "декабря"}

func local(t time.Time) time.Time { return t.In(catalog.Moscow) }

// dayLabel — «сегодня», «завтра» или «ср, 1 октября».
func dayLabel(t, now time.Time) string {
	t, now = local(t), local(now)
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	today := time.Date(y2, m2, d2, 0, 0, 0, 0, catalog.Moscow)
	day := time.Date(y1, m1, d1, 0, 0, 0, 0, catalog.Moscow)
	switch int(day.Sub(today).Hours() / 24) {
	case 0:
		return "сегодня"
	case 1:
		return "завтра"
	}
	return fmt.Sprintf("%s, %d %s", weekdayShort[t.Weekday()], t.Day(), monthGen[t.Month()])
}

func fullDate(t time.Time) string {
	t = local(t)
	return fmt.Sprintf("%s, %d %s", upperFirst(weekdayLong[t.Weekday()]), t.Day(), monthGen[t.Month()])
}

func clock(t time.Time) string { return local(t).Format("15:04") }

// buttonTime — короткая подпись кнопки: «ср 1.10 · 19:00».
func buttonTime(t time.Time) string {
	t = local(t)
	return fmt.Sprintf("%s %d.%02d · %s", weekdayShort[t.Weekday()], t.Day(), int(t.Month()), t.Format("15:04"))
}

func plural(n int, one, few, many string) string {
	n10, n100 := n%10, n%100
	switch {
	case n10 == 1 && n100 != 11:
		return one
	case n10 >= 2 && n10 <= 4 && (n100 < 10 || n100 >= 20):
		return few
	}
	return many
}

func seatsLabel(n int) string {
	switch {
	case n <= 0:
		return "мест нет"
	case n <= 2:
		return fmt.Sprintf("осталось %d %s", n, plural(n, "место", "места", "мест"))
	}
	return fmt.Sprintf("%d %s", n, plural(n, "место", "места", "мест"))
}

func distanceLabel(km float64) string {
	if km < 1 {
		return fmt.Sprintf("%d м", int(km*1000)/50*50)
	}
	return strings.Replace(fmt.Sprintf("%.1f км", km), ".", ",", 1)
}

func trialLabel(v *model.Venue) string {
	switch {
	case v.TrialPrice == nil:
		return ""
	case *v.TrialPrice == 0:
		return "Пробное занятие бесплатно"
	}
	return fmt.Sprintf("Пробное занятие — %d ₽", *v.TrialPrice)
}

func districtTitle(id string) string {
	if d, ok := catalog.DistrictByID(id); ok {
		return d.Title + " район"
	}
	return ""
}

var osmDays = strings.NewReplacer("Mo", "пн", "Tu", "вт", "We", "ср", "Th", "чт", "Fr", "пт", "Sa", "сб", "Su", "вс",
	"PH", "праздники", "off", "выходной", "24/7", "круглосуточно", ";", ",")

// openingHours переводит формат OpenStreetMap («Mo-Fr 07:00-22:00») на русский.
func openingHours(raw string) string {
	return osmDays.Replace(raw)
}

func routeURL(lat, lon float64) string {
	return fmt.Sprintf("https://yandex.ru/maps/?rtext=~%.6f,%.6f&rtt=auto", lat, lon)
}

func accessLabel(v *model.Venue) string {
	if v.BookingMode == model.BookingModeInstant {
		return "Онлайн-запись через СпортСлот"
	}
	return "Запись напрямую в зале"
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}
