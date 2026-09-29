package catalog

import "testing"

func TestParseSport(t *testing.T) {
	cases := map[string]string{
		"Хочу на бокс":       "boxing",
		"бассейн рядом":      "swimming",
		"yoga":               "yoga",
		"что-нибудь игровое": "team",
		"вышивание":          "",
	}
	for in, want := range cases {
		if got := ParseSport(in); got != want {
			t.Errorf("ParseSport(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseDistrict(t *testing.T) {
	cases := map[string]string{
		"Ворошиловский":      "voroshilovsky",
		"живу на Северном":   "voroshilovsky",
		"пролетарский район": "proletarsky",
		"где-то на Западном": "sovetsky",
	}
	for in, want := range cases {
		d, ok := ParseDistrict(in)
		if !ok || d.ID != want {
			t.Errorf("ParseDistrict(%q) = %q, want %q", in, d.ID, want)
		}
	}
	if _, ok := ParseDistrict("Москва"); ok {
		t.Error("unexpected district for Москва")
	}
}

func TestNearestDistrict(t *testing.T) {
	if d := NearestDistrict(47.285, 39.715); d.ID != "voroshilovsky" {
		t.Errorf("got %s", d.ID)
	}
	if km := DistanceKM(47.2225, 39.7185, 47.2805, 39.7115); km < 6 || km > 7 {
		t.Errorf("unexpected distance %.2f", km)
	}
}
