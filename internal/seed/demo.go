package seed

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/model"
	"sportslot/internal/repository/postgres"
)

// Тестовая учётная запись и служебные занятия с фиксированными id —
// на них ссылаются проверки в openapi/DATA-API.yaml.
const (
	TestUserID       = "c1111111-0000-0000-0000-000000000001"
	TestMaxUserID    = "max-test-user-001"
	FlagshipVenueID  = "a1111111-0000-0000-0000-000000000001"
	ReferenceSlotOne = "b1111111-0000-0000-0000-000000000001"
	ReferenceSlotTwo = "b1111111-0000-0000-0000-000000000002"
)

type lesson struct {
	title    string
	sport    string
	days     []time.Weekday
	hour     int
	minute   int
	duration time.Duration
	quota    int
}

type demoVenue struct {
	venue   model.Venue
	lessons []lesson
}

var (
	weekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday}
	mwf      = []time.Weekday{time.Monday, time.Wednesday, time.Friday}
	tt       = []time.Weekday{time.Tuesday, time.Thursday}
)

func price(p int) *int { return &p }

// DemoVenues — модельные студии-партнёры, по одной в каждом районе Ростова.
var DemoVenues = []demoVenue{
	{
		venue: model.Venue{ID: FlagshipVenueID, Name: "Спортцентр «Прибой»", District: "voroshilovsky",
			Address: "пр. Космонавтов, 32", Lat: 47.2893, Lon: 39.7147,
			Sports:      []string{"swimming", "fitness", "yoga", "boxing"},
			WhatToBring: "Удобную форму, сменную обувь и воду. В бассейн — шапочку, очки и сланцы.",
			TrialPrice:  price(0),
			Description: "Спортцентр с бассейном, залом бокса и студией йоги — можно попробовать несколько направлений в одном месте."},
		lessons: []lesson{
			{"Бокс для новичков", "boxing", mwf, 19, 0, time.Hour, 10},
			{"Плавание для взрослых", "swimming", tt, 8, 0, 45 * time.Minute, 6},
			{"Плавание для взрослых", "swimming", tt, 20, 0, 45 * time.Minute, 6},
			{"Плавание для взрослых", "swimming", []time.Weekday{time.Saturday}, 10, 0, 45 * time.Minute, 6},
			{"Йога для начинающих", "yoga", []time.Weekday{time.Monday, time.Wednesday}, 9, 0, time.Hour, 12},
			{"Йога для начинающих", "yoga", tt, 19, 30, time.Hour, 12},
			{"Йога для начинающих", "yoga", []time.Weekday{time.Sunday}, 11, 0, time.Hour, 12},
			{"Функциональная тренировка", "fitness", weekdays, 7, 30, 50 * time.Minute, 12},
			{"Функциональная тренировка", "fitness", []time.Weekday{time.Tuesday, time.Thursday, time.Saturday}, 18, 30, 50 * time.Minute, 12},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000002", Name: "Студия йоги «Равновесие»",
			District: "kirovsky", Address: "ул. Пушкинская, 140", Lat: 47.2268, Lon: 39.7242,
			Sports: []string{"yoga"}, WhatToBring: "Удобную одежду. Коврики есть в студии.", TrialPrice: price(300),
			Description: "Небольшая студия хатха-йоги: группы для начинающих утром и вечером, коврики и пледы есть."},
		lessons: []lesson{
			{"Хатха-йога для начинающих", "yoga", weekdays, 8, 0, time.Hour, 10},
			{"Хатха-йога для начинающих", "yoga", weekdays, 19, 0, time.Hour, 10},
			{"Мягкая йога", "yoga", []time.Weekday{time.Saturday, time.Sunday}, 10, 30, 75 * time.Minute, 10},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000003", Name: "Клуб единоборств «Стойка»",
			District: "sovetsky", Address: "ул. Малиновского, 16", Lat: 47.2337, Lon: 39.6181,
			Sports: []string{"boxing"}, WhatToBring: "Спортивную форму и полотенце. Бинты и перчатки дадим.", TrialPrice: price(0),
			Description: "Бокс и кикбоксинг для взрослых с нуля: техника и работа в парах без жёстких спаррингов."},
		lessons: []lesson{
			{"Бокс: техника с нуля", "boxing", mwf, 18, 0, time.Hour, 8},
			{"Кикбоксинг для взрослых", "boxing", mwf, 20, 0, time.Hour, 8},
			{"Бокс: техника с нуля", "boxing", []time.Weekday{time.Saturday}, 12, 0, time.Hour, 8},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000004", Name: "Школа танцев «Ритм»",
			District: "proletarsky", Address: "ул. 20-я Линия, 5", Lat: 47.2346, Lon: 39.7612,
			Sports: []string{"dance"}, WhatToBring: "Свободную одежду и сменную обувь.", TrialPrice: price(250),
			Description: "Современные танцы и латина для взрослых без подготовки — группы собираются по уровню."},
		lessons: []lesson{
			{"Современные танцы с нуля", "dance", tt, 19, 0, time.Hour, 12},
			{"Латина для начинающих", "dance", []time.Weekday{time.Monday, time.Wednesday}, 20, 0, time.Hour, 12},
			{"Латина для начинающих", "dance", []time.Weekday{time.Saturday}, 13, 0, time.Hour, 12},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000005", Name: "Зал «Кузница»",
			District: "oktyabrsky", Address: "пр. Ленина, 99", Lat: 47.2491, Lon: 39.6902,
			Sports: []string{"fitness"}, WhatToBring: "Кроссовки, форму и воду.", TrialPrice: price(0),
			Description: "Функциональный тренинг в мини-группах до 10 человек — тренер следит за техникой каждого."},
		lessons: []lesson{
			{"Функциональный тренинг", "fitness", weekdays, 7, 0, 50 * time.Minute, 10},
			{"Функциональный тренинг", "fitness", weekdays, 19, 0, 50 * time.Minute, 10},
			{"Круговая тренировка", "fitness", []time.Weekday{time.Saturday}, 11, 0, time.Hour, 10},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000006", Name: "Бассейн «Волна»",
			District: "pervomaysky", Address: "пр. Шолохова, 78", Lat: 47.2689, Lon: 39.7681,
			Sports: []string{"swimming"}, WhatToBring: "Шапочку, очки, сланцы и полотенце.", TrialPrice: price(400),
			Description: "25-метровый бассейн: плавание с инструктором для взрослых и аквааэробика по воскресеньям."},
		lessons: []lesson{
			{"Плавание с инструктором", "swimming", mwf, 7, 0, 45 * time.Minute, 6},
			{"Плавание с инструктором", "swimming", mwf, 21, 0, 45 * time.Minute, 6},
			{"Аквааэробика", "swimming", []time.Weekday{time.Sunday}, 9, 0, 45 * time.Minute, 6},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000007", Name: "Спортклуб «Команда»",
			District: "zheleznodorozhny", Address: "ул. Профсоюзная, 33", Lat: 47.2176, Lon: 39.6591,
			Sports: []string{"team"}, WhatToBring: "Форму и кроссовки для зала.", TrialPrice: price(0),
			Description: "Любительский мини-футбол и волейбол для взрослых — можно прийти одному, команду соберём."},
		lessons: []lesson{
			{"Мини-футбол для взрослых", "team", tt, 20, 0, 90 * time.Minute, 12},
			{"Волейбол: игры для новичков", "team", []time.Weekday{time.Wednesday}, 19, 0, 90 * time.Minute, 12},
			{"Волейбол: игры для новичков", "team", []time.Weekday{time.Saturday}, 12, 0, 90 * time.Minute, 12},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000008", Name: "Студия «Гибкость»",
			District: "leninsky", Address: "ул. Большая Садовая, 12", Lat: 47.2205, Lon: 39.6978,
			Sports: []string{"yoga", "dance"}, WhatToBring: "Удобную одежду и носки.", TrialPrice: price(200),
			Description: "Растяжка, пилатес и пластика в небольших группах — мягкая нагрузка для тех, кто начинает с нуля."},
		lessons: []lesson{
			{"Растяжка и пилатес", "yoga", mwf, 20, 0, time.Hour, 10},
			{"Растяжка и пилатес", "yoga", []time.Weekday{time.Saturday}, 10, 0, time.Hour, 10},
			{"Стрип-пластика для начинающих", "dance", tt, 20, 30, time.Hour, 10},
		},
	},
}

// SyncDemo создаёт демо-партнёров. Их расписание хранится как шаблоны недели
// (как у настоящих студий) и заполняется из кода только для студии без шаблонов,
// поэтому правки в кабинете переживают перезапуск.
func SyncDemo(ctx context.Context, store *postgres.Store, now time.Time) (int, error) {
	now = now.In(catalog.Moscow)
	for _, dv := range DemoVenues {
		v := dv.venue
		v.Source = model.VenueSourceDemo
		v.BookingMode = model.BookingModeInstant
		v.SportType = v.Sports[0]
		v.Level = "beginner"
		if err := store.UpsertVenue(ctx, &v); err != nil {
			return 0, fmt.Errorf("demo venue %s: %w", v.Name, err)
		}
		n, err := store.CountTemplates(ctx, v.ID)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			continue
		}
		for _, l := range dv.lessons {
			for _, day := range l.days {
				t := &model.Template{
					ID:      stableID(fmt.Sprintf("tpl|%s|%s|%d|%02d:%02d", v.ID, l.title, day, l.hour, l.minute)),
					VenueID: v.ID, Weekday: int(day), StartMinute: l.hour*60 + l.minute,
					DurationMin: int(l.duration.Minutes()), Title: l.title, SportType: l.sport, Level: "beginner", Quota: l.quota,
				}
				if err := store.SaveTemplate(ctx, t); err != nil {
					return 0, fmt.Errorf("demo template %s: %w", l.title, err)
				}
			}
		}
	}

	created, err := ExpandSchedules(ctx, store, now, "")
	if err != nil {
		return created, err
	}
	for i, id := range []string{ReferenceSlotOne, ReferenceSlotTwo} {
		date := now.AddDate(0, 0, 2+i)
		start := time.Date(date.Year(), date.Month(), date.Day(), 20, 30, 0, 0, catalog.Moscow)
		ref := &model.Slot{ID: id, VenueID: FlagshipVenueID, Title: "Бокс: вечерняя группа", SportType: "boxing",
			Level: "beginner", StartAt: start, EndAt: start.Add(time.Hour), QuotaTotal: 12}
		if err := store.KeepSlotInFuture(ctx, ref); err != nil {
			return created, err
		}
	}
	return created, store.EnsureUserWithID(ctx, TestUserID, TestMaxUserID, "Тестовый пользователь")
}

// ExpandSchedules разворачивает шаблоны студий на две недели вперёд.
// venueID пустой — для всех студий с онлайн-записью. Id занятия зависит от
// студии, названия и времени, поэтому повторный запуск ничего не дублирует.
func ExpandSchedules(ctx context.Context, store *postgres.Store, now time.Time, venueID string) (int, error) {
	now = now.In(catalog.Moscow)
	templates, err := store.ActiveTemplates(ctx, venueID)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, t := range templates {
		// Четыре недели вперёд: в поиске видны две, остальное — для записи на несколько недель.
		for day := 0; day <= 28; day++ {
			date := now.AddDate(0, 0, day)
			if int(date.Weekday()) != t.Weekday {
				continue
			}
			start := time.Date(date.Year(), date.Month(), date.Day(), t.StartMinute/60, t.StartMinute%60, 0, 0, catalog.Moscow)
			if !start.After(now) {
				continue
			}
			id := stableID(t.VenueID + "|" + t.Title + "|" + start.Format(time.RFC3339))
			booked := 0
			if t.Source == model.VenueSourceDemo {
				booked = prebooked(id, t.Quota)
			}
			slot := &model.Slot{
				ID: id, VenueID: t.VenueID, Title: t.Title, SportType: t.SportType, Level: t.Level,
				StartAt: start, EndAt: start.Add(time.Duration(t.DurationMin) * time.Minute),
				QuotaTotal: t.Quota, QuotaBooked: booked, TemplateID: t.ID,
			}
			added, err := store.InsertTemplateSlot(ctx, slot)
			if err != nil {
				return created, err
			}
			if added {
				created++
			}
		}
	}
	return created, nil
}

func hasDay(days []time.Weekday, d time.Weekday) bool {
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

// prebooked — сколько мест уже занято другими клиентами студии. Детерминировано
// по id занятия: часть занятий почти заполнена, некоторые заполнены целиком —
// так в демо видны «осталось 2 места» и лист ожидания.
func prebooked(slotID string, quota int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(slotID))
	switch r := h.Sum32() % 10; {
	case r == 0:
		return quota
	case r <= 2:
		return quota - 1 - int(r%2)
	default:
		return int(r) * quota / 20
	}
}
