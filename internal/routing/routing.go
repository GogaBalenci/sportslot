// Package routing считает время в пути от пользователя до площадок.
// Основной источник — матрица расстояний 2ГИС; если её нет или она не ответила,
// время оценивается по прямому расстоянию и средней скорости (такие значения
// помечаются как приблизительные).
package routing

import (
	"context"
	"math"

	"sportslot/internal/catalog"
)

type Mode string

const (
	Walking Mode = "walking"
	Transit Mode = "transit"
	Driving Mode = "driving"
)

var Modes = []Mode{Walking, Transit, Driving}

func ParseMode(s string) (Mode, bool) {
	for _, m := range Modes {
		if string(m) == s {
			return m, true
		}
	}
	return "", false
}

// Title — как способ звучит в тексте: «12 мин пешком».
func (m Mode) Title() string {
	switch m {
	case Walking:
		return "пешком"
	case Transit:
		return "на транспорте"
	case Driving:
		return "на машине"
	}
	return ""
}

type Point struct {
	Lat, Lon float64
}

// Trip — время и расстояние по дороге до одной площадки.
type Trip struct {
	Seconds   int
	Meters    int
	Estimated bool
}

// Minutes округляет вверх: «до 15 минут» не должно включать 15,5.
func (t Trip) Minutes() int {
	m := int(math.Ceil(float64(t.Seconds) / 60))
	if m < 1 {
		return 1
	}
	return m
}

// Router возвращает время в пути до каждой точки в порядке to.
// Ошибок не бывает: при сбое источника значения оцениваются.
type Router interface {
	Trips(ctx context.Context, from Point, to []Point, mode Mode) []Trip
}

// Estimator — оценка без внешних сервисов.
type Estimator struct{}

func (Estimator) Trips(_ context.Context, from Point, to []Point, mode Mode) []Trip {
	trips := make([]Trip, len(to))
	for i, p := range to {
		trips[i] = Estimate(from, p, mode)
	}
	return trips
}

// Estimate — прямое расстояние с поправкой на извилистость дорог и средняя
// скорость. Для транспорта добавлено время на подход к остановке и ожидание;
// на коротких расстояниях пешком быстрее, и берётся меньшее.
func Estimate(from, to Point, mode Mode) Trip {
	km := catalog.DistanceKM(from.Lat, from.Lon, to.Lat, to.Lon)
	walk := km * 1.3 / 4.8 * 3600
	var seconds, meters float64
	switch mode {
	case Transit:
		seconds = 8*60 + km*1.35/18*3600
		if walk < seconds {
			seconds = walk
		}
		meters = km * 1.35 * 1000
	case Driving:
		seconds = 3*60 + km*1.3/30*3600
		meters = km * 1.3 * 1000
	default:
		seconds = walk
		meters = km * 1.3 * 1000
	}
	return Trip{Seconds: int(math.Round(seconds)), Meters: int(math.Round(meters)), Estimated: true}
}

// MaxStraightKM — дальше этого расстояния по прямой за minutes не добраться
// даже в хороших условиях. Нужна, чтобы не запрашивать заведомо далёкие залы.
func MaxStraightKM(mode Mode, minutes int) float64 {
	speed := map[Mode]float64{Walking: 6, Transit: 25, Driving: 45}[mode]
	if speed == 0 {
		speed = 45
	}
	return speed * float64(minutes) / 60
}
