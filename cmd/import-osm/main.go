// import-osm превращает выгрузку Overpass (tools/fetch_rostov_osm.sh) в каталог
// площадок seed-data/rostov_catalog.json, который API загружает при старте.
//
//	go run ./cmd/import-osm -in rostov_osm.json -out seed-data/rostov_catalog.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"sportslot/internal/catalog"
	"sportslot/internal/seed"
)

type overpassResponse struct {
	OSM3S struct {
		TimestampOSMBase string `json:"timestamp_osm_base"`
	} `json:"osm3s"`
	Elements []element `json:"elements"`
}

type element struct {
	Type   string                      `json:"type"`
	ID     int64                       `json:"id"`
	Lat    float64                     `json:"lat"`
	Lon    float64                     `json:"lon"`
	Center *struct{ Lat, Lon float64 } `json:"center"`
	Tags   map[string]string           `json:"tags"`
}

func main() {
	in := flag.String("in", "rostov_osm.json", "выгрузка Overpass в формате JSON")
	out := flag.String("out", "seed-data/rostov_catalog.json", "куда записать каталог")
	flag.Parse()

	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatalf("read %s: %v", *in, err)
	}
	var resp overpassResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		log.Fatalf("parse %s: %v", *in, err)
	}

	venues, stats := convert(resp.Elements)

	exportedAt := time.Now().UTC()
	if t, err := time.Parse(time.RFC3339, resp.OSM3S.TimestampOSMBase); err == nil {
		exportedAt = t
	}
	file := seed.CatalogFile{
		Source:     "OpenStreetMap",
		License:    "ODbL 1.0, © участники OpenStreetMap",
		ExportedAt: exportedAt,
		Venues:     venues,
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	fmt.Printf("элементов в выгрузке: %d, в каталоге: %d\n", len(resp.Elements), len(venues))
	for _, reason := range sortedKeys(stats) {
		fmt.Printf("  отброшено (%s): %d\n", reason, stats[reason])
	}
}

func convert(elements []element) ([]seed.CatalogVenue, map[string]int) {
	skipped := map[string]int{}
	seen := map[string]bool{}
	var venues []seed.CatalogVenue

	for _, el := range elements {
		tags := el.Tags
		name := strings.TrimSpace(firstNonEmpty(tags["name:ru"], tags["name"]))
		lat, lon := el.Lat, el.Lon
		if el.Center != nil {
			lat, lon = el.Center.Lat, el.Center.Lon
		}

		switch {
		case name == "":
			skipped["без названия"]++
			continue
		case tags["access"] == "private" || tags["access"] == "no":
			skipped["закрытый доступ"]++
			continue
		case !inRostov(tags["addr:city"], lat, lon):
			skipped["за пределами Ростова"]++
			continue
		case isIgnoredSport(tags["sport"]):
			skipped["не про тренировки"]++
			continue
		}

		sports := detectSports(name, tags)
		key := strings.ToLower(name) + fmt.Sprintf("|%.3f|%.3f", lat, lon)
		if seen[key] {
			skipped["дубликат"]++
			continue
		}
		seen[key] = true

		venues = append(venues, seed.CatalogVenue{
			ExternalID:   fmt.Sprintf("osm:%s/%d", el.Type, el.ID),
			Name:         name,
			Sports:       sports,
			Lat:          lat,
			Lon:          lon,
			Address:      address(tags),
			District:     catalog.NearestDistrict(lat, lon).ID,
			Phone:        firstNonEmpty(tags["contact:phone"], tags["phone"]),
			Website:      firstNonEmpty(tags["contact:website"], tags["website"], tags["contact:vk"]),
			OpeningHours: tags["opening_hours"],
			SourceURL:    fmt.Sprintf("https://www.openstreetmap.org/%s/%d", el.Type, el.ID),
		})
	}

	sort.Slice(venues, func(i, j int) bool { return venues[i].ExternalID < venues[j].ExternalID })
	return venues, skipped
}

// inRostov отсекает Батайск и Аксай, попавшие в прямоугольник выгрузки.
// Южнее 47.18 — левый берег у Батайска, восточнее 39.83 — Аксай.
func inRostov(city string, lat, lon float64) bool {
	if city != "" && !strings.Contains(strings.ToLower(city), "ростов") {
		return false
	}
	return lat >= 47.18 && lon <= 39.83
}

func isIgnoredSport(sport string) bool {
	for _, s := range strings.Split(sport, ";") {
		switch strings.TrimSpace(s) {
		case "billiards", "mtb", "cycling", "motor", "karting", "paintball", "bmx":
			return true
		}
	}
	return false
}

var tagSports = map[string]string{
	"swimming": "swimming", "diving": "swimming", "water_polo": "swimming",
	"yoga": "yoga", "pilates": "yoga",
	"boxing": "boxing", "martial_arts": "boxing", "karate": "boxing", "judo": "boxing",
	"taekwondo": "boxing", "sambo": "boxing", "kickboxing": "boxing", "mma": "boxing",
	"wrestling": "boxing", "aikido": "boxing", "jiu-jitsu": "boxing",
	"fitness": "fitness", "crossfit": "fitness", "weightlifting": "fitness",
	"powerlifting": "fitness", "gymnastics": "fitness", "climbing": "fitness",
	"dance":  "dance",
	"soccer": "team", "football": "team", "volleyball": "team", "beachvolleyball": "team",
	"basketball": "team", "handball": "team", "futsal": "team", "ice_hockey": "team",
	"tennis": "tennis", "table_tennis": "tennis", "padel": "tennis", "badminton": "tennis", "squash": "tennis",
	"athletics": "athletics", "running": "athletics",
}

func detectSports(name string, tags map[string]string) []string {
	found := map[string]bool{}
	for _, s := range strings.Split(tags["sport"], ";") {
		if id, ok := tagSports[strings.TrimSpace(s)]; ok {
			found[id] = true
		}
	}
	if id := catalog.ParseSport(name); id != "" {
		found[id] = true
	}
	switch tags["leisure"] {
	case "swimming_pool":
		found["swimming"] = true
	case "fitness_centre":
		found["fitness"] = true
	case "dance":
		found["dance"] = true
	}
	if tags["amenity"] == "dojo" {
		found["boxing"] = true
	}
	if len(found) == 0 {
		return []string{"multi"}
	}
	var sports []string
	for _, s := range catalog.Sports {
		if found[s.ID] {
			sports = append(sports, s.ID)
		}
	}
	return sports
}

func address(tags map[string]string) string {
	if full := tags["addr:full"]; full != "" {
		return full
	}
	street, house := tags["addr:street"], tags["addr:housenumber"]
	switch {
	case street != "" && house != "":
		return street + ", " + house
	case street != "":
		return street
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
