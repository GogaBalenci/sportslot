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

const demoNote = "Демо-партнёр: студия вымышлена, адрес условный, расписание модельное. " +
	"На ней можно пройти весь путь — от записи до отметки посещения по QR."

// DemoVenues — модельные студии-партнёры, по одной в каждом районе Ростова.
var DemoVenues = []demoVenue{
	{
		venue: model.Venue{ID: FlagshipVenueID, Name: "Спортцентр «Прибой»", District: "voroshilovsky",
			Address: "пр. Космонавтов, 32", Lat: 47.2893, Lon: 39.7147,
			Sports:      []string{"swimming", "fitness", "yoga", "boxing"},
			WhatToBring: "Удобную форму, сменную обувь и воду. В бассейн — шапочку, очки и сланцы.",
			TrialPrice:  price(0)},
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
			Sports: []string{"yoga"}, WhatToBring: "Удобную одежду. Коврики есть в студии.", TrialPrice: price(300)},
		lessons: []lesson{
			{"Хатха-йога для начинающих", "yoga", weekdays, 8, 0, time.Hour, 10},
			{"Хатха-йога для начинающих", "yoga", weekdays, 19, 0, time.Hour, 10},
			{"Мягкая йога", "yoga", []time.Weekday{time.Saturday, time.Sunday}, 10, 30, 75 * time.Minute, 10},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000003", Name: "Клуб единоборств «Стойка»",
			District: "sovetsky", Address: "ул. Малиновского, 16", Lat: 47.2337, Lon: 39.6181,
			Sports: []string{"boxing"}, WhatToBring: "Спортивную форму и полотенце. Бинты и перчатки дадим.", TrialPrice: price(0)},
		lessons: []lesson{
			{"Бокс: техника с нуля", "boxing", mwf, 18, 0, time.Hour, 8},
			{"Кикбоксинг для взрослых", "boxing", mwf, 20, 0, time.Hour, 8},
			{"Бокс: техника с нуля", "boxing", []time.Weekday{time.Saturday}, 12, 0, time.Hour, 8},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000004", Name: "Школа танцев «Ритм»",
			District: "proletarsky", Address: "ул. 20-я Линия, 5", Lat: 47.2346, Lon: 39.7612,
			Sports: []string{"dance"}, WhatToBring: "Свободную одежду и сменную обувь.", TrialPrice: price(250)},
		lessons: []lesson{
			{"Современные танцы с нуля", "dance", tt, 19, 0, time.Hour, 12},
			{"Латина для начинающих", "dance", []time.Weekday{time.Monday, time.Wednesday}, 20, 0, time.Hour, 12},
			{"Латина для начинающих", "dance", []time.Weekday{time.Saturday}, 13, 0, time.Hour, 12},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000005", Name: "Зал «Кузница»",
			District: "oktyabrsky", Address: "пр. Ленина, 99", Lat: 47.2491, Lon: 39.6902,
			Sports: []string{"fitness"}, WhatToBring: "Кроссовки, форму и воду.", TrialPrice: price(0)},
		lessons: []lesson{
			{"Функциональный тренинг", "fitness", weekdays, 7, 0, 50 * time.Minute, 10},
			{"Функциональный тренинг", "fitness", weekdays, 19, 0, 50 * time.Minute, 10},
			{"Круговая тренировка", "fitness", []time.Weekday{time.Saturday}, 11, 0, time.Hour, 10},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000006", Name: "Бассейн «Волна»",
			District: "pervomaysky", Address: "пр. Шолохова, 78", Lat: 47.2689, Lon: 39.7681,
			Sports: []string{"swimming"}, WhatToBring: "Шапочку, очки, сланцы и полотенце.", TrialPrice: price(400)},
		lessons: []lesson{
			{"Плавание с инструктором", "swimming", mwf, 7, 0, 45 * time.Minute, 6},
			{"Плавание с инструктором", "swimming", mwf, 21, 0, 45 * time.Minute, 6},
			{"Аквааэробика", "swimming", []time.Weekday{time.Sunday}, 9, 0, 45 * time.Minute, 6},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000007", Name: "Спортклуб «Команда»",
			District: "zheleznodorozhny", Address: "ул. Профсоюзная, 33", Lat: 47.2176, Lon: 39.6591,
			Sports: []string{"team"}, WhatToBring: "Форму и кроссовки для зала.", TrialPrice: price(0)},
		lessons: []lesson{
			{"Мини-футбол для взрослых", "team", tt, 20, 0, 90 * time.Minute, 12},
			{"Волейбол: игры для новичков", "team", []time.Weekday{time.Wednesday}, 19, 0, 90 * time.Minute, 12},
			{"Волейбол: игры для новичков", "team", []time.Weekday{time.Saturday}, 12, 0, 90 * time.Minute, 12},
		},
	},
	{
		venue: model.Venue{ID: "a1111111-0000-0000-0000-000000000008", Name: "Студия «Гибкость»",
			District: "leninsky", Address: "ул. Большая Садовая, 12", Lat: 47.2205, Lon: 39.6978,
			Sports: []string{"yoga", "dance"}, WhatToBring: "Удобную одежду и носки.", TrialPrice: price(200)},
		lessons: []lesson{
			{"Растяжка и пилатес", "yoga", mwf, 20, 0, time.Hour, 10},
			{"Растяжка и пилатес", "yoga", []time.Weekday{time.Saturday}, 10, 0, time.Hour, 10},
			{"Стрип-пластика для начинающих", "dance", tt, 20, 30, time.Hour, 10},
		},
	},
}

// SyncDemo создаёт демо-партнёров и расписание на две недели вперёд.
// Вызывается при старте и раз в сутки, поэтому демо не устаревает.
func SyncDemo(ctx context.Context, store *postgres.Store, now time.Time) (int, error) {
	now = now.In(catalog.Moscow)
	created := 0
	for _, dv := range DemoVenues {
		v := dv.venue
		v.Source = model.VenueSourceDemo
		v.BookingMode = model.BookingModeInstant
		v.SportType = v.Sports[0]
		v.Level = "beginner"
		v.Description = demoNote
		if err := store.UpsertVenue(ctx, &v); err != nil {
			return created, fmt.Errorf("demo venue %s: %w", v.Name, err)
		}
		for day := 0; day <= 14; day++ {
			date := now.AddDate(0, 0, day)
			for _, l := range dv.lessons {
				if !hasDay(l.days, date.Weekday()) {
					continue
				}
				start := time.Date(date.Year(), date.Month(), date.Day(), l.hour, l.minute, 0, 0, catalog.Moscow)
				if !start.After(now) {
					continue
				}
				id := stableID(v.ID + "|" + l.title + "|" + start.Format(time.RFC3339))
				slot := &model.Slot{
					ID: id, VenueID: v.ID, Title: l.title, SportType: l.sport, Level: "beginner",
					StartAt: start, EndAt: start.Add(l.duration),
					QuotaTotal: l.quota, QuotaBooked: prebooked(id, l.quota),
				}
				if err := store.InsertSlotIfAbsent(ctx, slot); err != nil {
					return created, err
				}
				created++
			}
		}
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
