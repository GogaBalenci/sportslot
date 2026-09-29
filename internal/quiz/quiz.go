// Package quiz — подбор вида спорта по ответам на несколько вопросов.
// Никакой магии: у каждого варианта ответа есть баллы для видов спорта,
// побеждают виды с наибольшей суммой. Таблица целиком описана в README.
package quiz

import (
	"sort"
	"strings"

	"sportslot/internal/catalog"
)

type Option struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Hint — как этот ответ звучит в объяснении результата.
	Hint   string         `json:"-"`
	Scores map[string]int `json:"-"`
}

type Question struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Options []Option `json:"options"`
}

var Questions = []Question{
	{
		ID:    "goal",
		Title: "Зачем хочешь заниматься?",
		Options: []Option{
			{ID: "health", Title: "Здоровье и тонус", Hint: "хочешь больше энергии",
				Scores: map[string]int{"swimming": 2, "yoga": 1, "fitness": 1, "team": 1}},
			{ID: "weight", Title: "Похудеть", Hint: "хочешь сбросить вес",
				Scores: map[string]int{"fitness": 2, "swimming": 2, "boxing": 1, "dance": 1}},
			{ID: "strength", Title: "Сила и форма", Hint: "хочешь подтянуть форму",
				Scores: map[string]int{"fitness": 3, "boxing": 2}},
			{ID: "stress", Title: "Снять стресс", Hint: "хочешь переключиться после работы",
				Scores: map[string]int{"yoga": 3, "swimming": 1, "boxing": 1}},
			{ID: "company", Title: "Найти компанию", Hint: "хочешь заниматься с людьми",
				Scores: map[string]int{"team": 3, "dance": 2}},
		},
	},
	{
		ID:    "water",
		Title: "Как ты относишься к воде?",
		Options: []Option{
			{ID: "love", Title: "Люблю плавать", Hint: "любишь воду", Scores: map[string]int{"swimming": 4}},
			{ID: "ok", Title: "Нормально", Scores: map[string]int{"swimming": 1}},
			{ID: "no", Title: "Не моё", Scores: map[string]int{"swimming": -4}},
		},
	},
	{
		ID:    "format",
		Title: "Как тебе комфортнее?",
		Options: []Option{
			{ID: "solo", Title: "Сам по себе", Hint: "предпочитаешь заниматься в своём ритме",
				Scores: map[string]int{"swimming": 1, "fitness": 1, "yoga": 1}},
			{ID: "group", Title: "В группе с тренером", Hint: "хочешь заниматься в группе",
				Scores: map[string]int{"yoga": 1, "dance": 2, "fitness": 1, "boxing": 1}},
			{ID: "team", Title: "В команде с мячом", Hint: "любишь игры с мячом",
				Scores: map[string]int{"team": 4}},
		},
	},
	{
		ID:    "pace",
		Title: "Какой темп хочется?",
		Options: []Option{
			{ID: "calm", Title: "Спокойный", Hint: "хочешь спокойный темп",
				Scores: map[string]int{"yoga": 3, "swimming": 1}},
			{ID: "medium", Title: "Средний", Hint: "хочешь умеренную нагрузку",
				Scores: map[string]int{"dance": 1, "swimming": 1, "team": 1, "fitness": 1}},
			{ID: "max", Title: "Выложиться полностью", Hint: "хочешь выкладываться",
				Scores: map[string]int{"boxing": 2, "fitness": 2, "team": 1}},
		},
	},
	{
		ID:    "contact",
		Title: "Спарринги и контакт — как тебе?",
		Options: []Option{
			{ID: "yes", Title: "Интересно попробовать", Hint: "не против спаррингов", Scores: map[string]int{"boxing": 4}},
			{ID: "no", Title: "Лучше без этого", Scores: map[string]int{"boxing": -4}},
		},
	},
}

type Result struct {
	Sport  string `json:"sport"`
	Title  string `json:"title"`
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

func QuestionByIndex(i int) (Question, bool) {
	if i < 0 || i >= len(Questions) {
		return Question{}, false
	}
	return Questions[i], true
}

func findOption(q Question, optionID string) (Option, bool) {
	for _, o := range q.Options {
		if o.ID == optionID {
			return o, true
		}
	}
	return Option{}, false
}

// Valid проверяет, что ответ существует.
func Valid(questionID, optionID string) bool {
	for _, q := range Questions {
		if q.ID == questionID {
			_, ok := findOption(q, optionID)
			return ok
		}
	}
	return false
}

// Recommend возвращает до limit видов спорта. answers: id вопроса -> id ответа,
// неотвеченные вопросы просто не влияют на результат.
func Recommend(answers map[string]string, limit int) []Result {
	scores := map[string]int{}
	var chosen []Option
	for _, q := range Questions {
		o, ok := findOption(q, answers[q.ID])
		if !ok {
			continue
		}
		chosen = append(chosen, o)
		for sport, pts := range o.Scores {
			scores[sport] += pts
		}
	}

	var results []Result
	for _, s := range catalog.Sports {
		if !s.Bookable {
			continue
		}
		results = append(results, Result{Sport: s.ID, Title: s.Title, Score: scores[s.ID]})
	}
	// При равенстве баллов порядок справочника: так результат воспроизводим.
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	for i := range results {
		results[i].Reason = reason(results[i].Sport, chosen)
	}
	return results
}

// reason собирает объяснение из ответов, которые сильнее всего повлияли на результат.
func reason(sport string, chosen []Option) string {
	type hint struct {
		text string
		pts  int
	}
	var hints []hint
	for _, o := range chosen {
		if o.Hint != "" && o.Scores[sport] >= 2 {
			hints = append(hints, hint{o.Hint, o.Scores[sport]})
		}
	}
	sort.SliceStable(hints, func(i, j int) bool { return hints[i].pts > hints[j].pts })
	if len(hints) > 2 {
		hints = hints[:2]
	}
	switch len(hints) {
	case 0:
		return "Хороший вариант для старта: занятия для новичков есть в разных районах."
	case 1:
		return upperFirst(hints[0].text) + "."
	}
	return upperFirst(hints[0].text) + " и " + hints[1].text + "."
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}
