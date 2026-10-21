package bot

import (
	"fmt"
	"html"
	"strings"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/model"
	"sportslot/internal/routing"
	"sportslot/internal/service"
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

// travelLabel — «12 мин пешком», «1 ч 05 мин на транспорте»; оценка — со значком «≈».
func travelLabel(t *service.Travel) string {
	label := fmt.Sprintf("%d мин", t.Minutes)
	if t.Minutes >= 60 {
		label = fmt.Sprintf("%d ч %02d мин", t.Minutes/60, t.Minutes%60)
	}
	label += " " + t.Mode.Title()
	if t.Estimated {
		label = "≈ " + label
	}
	return label
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

var osmDayIndex = map[string]int{"Mo": 0, "Tu": 1, "We": 2, "Th": 3, "Fr": 4, "Sa": 5, "Su": 6}
var ruDays = [7]string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"}

// openingHours переводит часы работы OpenStreetMap на русский и склеивает дни
// с одинаковым временем: «Mo,We,Fr 11:00-22:00; Tu,Th,Sa 11:00-22:00» →
// «пн–сб 11:00–22:00». Непонятный формат переводится как есть.
func openingHours(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "24/7" {
		return "круглосуточно"
	}
	var hours [7]string
	for _, rule := range strings.Split(raw, ";") {
		rule = strings.TrimSpace(rule)
		// Праздничные дни в карточке не показываем.
		if rule == "" || strings.HasPrefix(rule, "PH") {
			continue
		}
		daysPart, timePart, ok := strings.Cut(rule, " ")
		days, okDays := expandDays(daysPart)
		if !ok || !okDays {
			return osmDays.Replace(raw)
		}
		timePart = strings.TrimSpace(timePart)
		text := strings.ReplaceAll(strings.ReplaceAll(timePart, "-", "–"), ",", ", ")
		if timePart == "off" || timePart == "closed" {
			text = "выходной"
		}
		for _, d := range days {
			hours[d] = text
		}
	}
	var order []string
	groups := map[string][]string{}
	for i := 0; i < 7; {
		if hours[i] == "" {
			i++
			continue
		}
		j := i
		for j+1 < 7 && hours[j+1] == hours[i] {
			j++
		}
		label := ruDays[i]
		switch j - i {
		case 0:
		case 1:
			label += ", " + ruDays[j]
		default:
			label += "–" + ruDays[j]
		}
		if _, seen := groups[hours[i]]; !seen {
			order = append(order, hours[i])
		}
		groups[hours[i]] = append(groups[hours[i]], label)
		i = j + 1
	}
	if len(order) == 0 {
		return osmDays.Replace(raw)
	}
	parts := make([]string, 0, len(order))
	for _, h := range order {
		parts = append(parts, strings.Join(groups[h], ", ")+" "+h)
	}
	return strings.Join(parts, "; ")
}

func expandDays(spec string) ([]int, bool) {
	var days []int
	for _, part := range strings.Split(spec, ",") {
		from, to, isRange := strings.Cut(part, "-")
		a, okA := osmDayIndex[from]
		if !okA {
			return nil, false
		}
		if !isRange {
			days = append(days, a)
			continue
		}
		b, okB := osmDayIndex[to]
		if !okB {
			return nil, false
		}
		for d := a; ; d = (d + 1) % 7 {
			days = append(days, d)
			if d == b {
				break
			}
		}
	}
	return days, true
}

// phoneLabel приводит номер к виду «+7 863 296-10-61». Короткие номера без
// кода в OpenStreetMap — ростовские, к ним добавляется код 863.
func phoneLabel(raw string) string {
	first, _, _ := strings.Cut(raw, ";")
	var digits []byte
	for i := 0; i < len(first); i++ {
		if first[i] >= '0' && first[i] <= '9' {
			digits = append(digits, first[i])
		}
	}
	d := string(digits)
	switch {
	case len(d) == 7:
		d = "7863" + d
	case len(d) == 10:
		d = "7" + d
	case len(d) == 11 && d[0] == '8':
		d = "7" + d[1:]
	}
	if len(d) != 11 || d[0] != '7' {
		return strings.TrimSpace(first)
	}
	if d[1:4] == "863" {
		return fmt.Sprintf("+7 863 %s-%s-%s", d[4:7], d[7:9], d[9:11])
	}
	return fmt.Sprintf("+7 %s %s-%s-%s", d[1:4], d[4:7], d[7:9], d[9:11])
}

// Origin — откуда пользователь добирается; нужна, чтобы маршрут открывался
// сразу с точкой старта и нужным способом передвижения.
type Origin struct {
	Lat, Lon float64
	Mode     routing.Mode
}

func originOf(st state) *Origin {
	if !st.HasPoint {
		return nil
	}
	return &Origin{Lat: st.Lat, Lon: st.Lon, Mode: routing.Mode(st.Mode)}
}

// routeURL — маршрут в Яндекс Картах. С известной точкой старта карта не
// спрашивает, откуда строить маршрут.
func routeURL(from *Origin, lat, lon float64) string {
	if from == nil {
		return fmt.Sprintf("https://yandex.ru/maps/?rtext=~%.6f,%.6f&rtt=auto", lat, lon)
	}
	rtt := map[routing.Mode]string{routing.Walking: "pd", routing.Transit: "mt", routing.Driving: "auto"}[from.Mode]
	if rtt == "" {
		rtt = "mt"
	}
	return fmt.Sprintf("https://yandex.ru/maps/?rtext=%.6f,%.6f~%.6f,%.6f&rtt=%s", from.Lat, from.Lon, lat, lon, rtt)
}

// esc экранирует текст для HTML-разметки сообщений MAX.
func esc(s string) string { return html.EscapeString(s) }

func modeIcon(m routing.Mode) string {
	switch m {
	case routing.Walking:
		return "🚶"
	case routing.Driving:
		return "🚗"
	}
	return "🚌"
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
