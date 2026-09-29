package quiz

import "testing"

func TestRecommendWaterAndCalm(t *testing.T) {
	got := Recommend(map[string]string{
		"goal": "health", "water": "love", "format": "solo", "pace": "calm", "contact": "no",
	}, 2)
	if len(got) != 2 || got[0].Sport != "swimming" {
		t.Fatalf("want swimming first, got %+v", got)
	}
	if got[1].Sport != "yoga" {
		t.Fatalf("want yoga second, got %+v", got)
	}
	if got[0].Reason != "Любишь воду и хочешь больше энергии." {
		t.Fatalf("unexpected reason: %q", got[0].Reason)
	}
}

func TestRecommendFighter(t *testing.T) {
	got := Recommend(map[string]string{
		"goal": "strength", "water": "no", "format": "group", "pace": "max", "contact": "yes",
	}, 2)
	if got[0].Sport != "boxing" || got[1].Sport != "fitness" {
		t.Fatalf("want boxing, fitness; got %+v", got)
	}
}

func TestRecommendCompany(t *testing.T) {
	got := Recommend(map[string]string{"goal": "company", "format": "team"}, 1)
	if got[0].Sport != "team" {
		t.Fatalf("want team, got %+v", got)
	}
}

func TestRecommendEmptyAnswersIsStable(t *testing.T) {
	a := Recommend(nil, 3)
	b := Recommend(map[string]string{"goal": "unknown"}, 3)
	for i := range a {
		if a[i].Sport != b[i].Sport {
			t.Fatalf("unstable order: %+v vs %+v", a, b)
		}
	}
}

func TestValid(t *testing.T) {
	if !Valid("water", "love") || Valid("water", "maybe") || Valid("nope", "love") {
		t.Fatal("Valid works incorrectly")
	}
}
