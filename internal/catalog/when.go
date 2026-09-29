package catalog

import "time"

type When struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

var Whens = []When{
	{ID: "today", Title: "Сегодня"},
	{ID: "tomorrow", Title: "Завтра"},
	{ID: "weekday_eve", Title: "Будни вечером"},
	{ID: "weekend", Title: "Выходные"},
	{ID: "any", Title: "Любое время"},
}

// SearchHorizon — на сколько дней вперёд показываем расписание.
const SearchHorizon = 14 * 24 * time.Hour

func WhenTitle(id string) string {
	for _, w := range Whens {
		if w.ID == id {
			return w.Title
		}
	}
	return "Любое время"
}

// WhenRange — интервал поиска для выбора пользователя.
func WhenRange(id string, now time.Time) (time.Time, time.Time) {
	now = now.In(Moscow)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Moscow)
	switch id {
	case "today":
		return now, midnight.AddDate(0, 0, 1)
	case "tomorrow":
		return midnight.AddDate(0, 0, 1), midnight.AddDate(0, 0, 2)
	}
	return now, now.Add(SearchHorizon)
}

// WhenMatches проверяет, подходит ли время начала занятия под выбор.
func WhenMatches(id string, start time.Time) bool {
	start = start.In(Moscow)
	switch id {
	case "weekday_eve":
		wd := start.Weekday()
		return wd != time.Saturday && wd != time.Sunday && start.Hour() >= 17
	case "weekend":
		wd := start.Weekday()
		return wd == time.Saturday || wd == time.Sunday
	}
	return true
}
