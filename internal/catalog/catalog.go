// Package catalog — справочники, общие для бота, API и импорта данных:
// виды спорта, районы Ростова-на-Дону и расчёт расстояний.
package catalog

import (
	"math"
	"strings"
	"time"
	_ "time/tzdata"
)

type Sport struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Short — подпись для кнопок, где мало места.
	Short string `json:"short"`
	// Bookable — вид спорта есть у партнёров с онлайн-записью и участвует в подборе.
	Bookable bool `json:"bookable"`
}

var Sports = []Sport{
	{ID: "swimming", Title: "Плавание", Short: "Плавание", Bookable: true},
	{ID: "yoga", Title: "Йога и растяжка", Short: "Йога", Bookable: true},
	{ID: "boxing", Title: "Бокс и единоборства", Short: "Единоборства", Bookable: true},
	{ID: "fitness", Title: "Функциональный тренинг", Short: "Фитнес", Bookable: true},
	{ID: "dance", Title: "Танцы", Short: "Танцы", Bookable: true},
	{ID: "team", Title: "Игровые виды", Short: "Игровые", Bookable: true},
	{ID: "tennis", Title: "Теннис", Short: "Теннис"},
	{ID: "athletics", Title: "Лёгкая атлетика и бег", Short: "Бег"},
	{ID: "multi", Title: "Спорткомплекс", Short: "Спорткомплекс"},
}

func SportByID(id string) (Sport, bool) {
	for _, s := range Sports {
		if s.ID == id {
			return s, true
		}
	}
	return Sport{}, false
}

func SportTitle(id string) string {
	if s, ok := SportByID(id); ok {
		return s.Title
	}
	return id
}

// ParseSport распознаёт вид спорта в свободном тексте пользователя.
func ParseSport(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return ""
	}
	if _, ok := SportByID(t); ok {
		return t
	}
	for _, rule := range sportWords {
		for _, w := range rule.words {
			if strings.Contains(t, w) {
				return rule.sport
			}
		}
	}
	return ""
}

var sportWords = []struct {
	sport string
	words []string
}{
	{"swimming", []string{"плав", "бассейн", "аква"}},
	{"yoga", []string{"йог", "растяж", "стретч", "пилатес"}},
	{"boxing", []string{"бокс", "единобор", "карате", "дзюдо", "самбо", "борьб", "mma", "мма", "тхэквондо", "fight", "бойц"}},
	{"fitness", []string{"фитнес", "функционал", "тренаж", "кроссфит", "качал", "fitness", "gym"}},
	{"dance", []string{"танц", "хореограф"}},
	{"team", []string{"футбол", "волейбол", "баскетбол", "гандбол", "игров", "команд"}},
	{"tennis", []string{"теннис", "падел", "бадминтон"}},
	{"athletics", []string{"бег", "атлетик"}},
}

type District struct {
	ID    string  `json:"id"`
	Title string  `json:"title"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

// Districts — районы Ростова-на-Дону. Координаты — приблизительные центры
// жилой застройки района, их достаточно для сортировки по удалённости.
var Districts = []District{
	{ID: "voroshilovsky", Title: "Ворошиловский", Lat: 47.2805, Lon: 39.7115},
	{ID: "pervomaysky", Title: "Первомайский", Lat: 47.2700, Lon: 39.7640},
	{ID: "oktyabrsky", Title: "Октябрьский", Lat: 47.2480, Lon: 39.6880},
	{ID: "sovetsky", Title: "Советский", Lat: 47.2320, Lon: 39.6150},
	{ID: "zheleznodorozhny", Title: "Железнодорожный", Lat: 47.2180, Lon: 39.6560},
	{ID: "leninsky", Title: "Ленинский", Lat: 47.2210, Lon: 39.6960},
	{ID: "kirovsky", Title: "Кировский", Lat: 47.2260, Lon: 39.7200},
	{ID: "proletarsky", Title: "Пролетарский", Lat: 47.2330, Lon: 39.7650},
}

// CityCenter — точка по умолчанию, когда пользователь не указал район.
var CityCenter = District{ID: "center", Title: "центр", Lat: 47.2225, Lon: 39.7185}

func DistrictByID(id string) (District, bool) {
	for _, d := range Districts {
		if d.ID == id {
			return d, true
		}
	}
	return District{}, false
}

// ParseDistrict ищет название района в тексте («на Северном», «ворошиловский»).
func ParseDistrict(text string) (District, bool) {
	t := strings.ToLower(text)
	for _, d := range Districts {
		stem := strings.ToLower(d.Title)
		if len([]rune(stem)) > 6 {
			stem = string([]rune(stem)[:6])
		}
		if strings.Contains(t, stem) {
			return d, true
		}
	}
	for word, id := range districtAliases {
		if strings.Contains(t, word) {
			d, _ := DistrictByID(id)
			return d, true
		}
	}
	return District{}, false
}

var districtAliases = map[string]string{
	"северн":    "voroshilovsky",
	"сжм":       "voroshilovsky",
	"сельмаш":   "pervomaysky",
	"западн":    "sovetsky",
	"зжм":       "sovetsky",
	"нахичеван": "proletarsky",
	"военвед":   "oktyabrsky",
	"центр":     "kirovsky",
	"вокзал":    "zheleznodorozhny",
	"темерник":  "zheleznodorozhny",
}

// NearestDistrict возвращает район, центр которого ближе всего к точке.
func NearestDistrict(lat, lon float64) District {
	best := Districts[0]
	bestDist := math.MaxFloat64
	for _, d := range Districts {
		if dist := DistanceKM(lat, lon, d.Lat, d.Lon); dist < bestDist {
			best, bestDist = d, dist
		}
	}
	return best
}

// DistanceKM — расстояние по формуле гаверсинусов.
func DistanceKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0
	toRad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadiusKM * math.Asin(math.Sqrt(a))
}

// Moscow — часовой пояс Ростова-на-Дону (UTC+3), в нём показываются все даты.
var Moscow = mustLoadLocation("Europe/Moscow")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("MSK", 3*60*60)
	}
	return loc
}
