package geocode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuery(t *testing.T) {
	cases := map[string]string{
		"Большая Садовая, 1":            "Ростов-на-Дону, Большая Садовая, 1",
		"ростов-на-дону, пр. Ленина 99": "Ростов-на-Дону, пр. Ленина 99",
		"г. Ростов Малиновского 16":     "Ростов-на-Дону, Малиновского 16",
		"   ": "",
	}
	for in, want := range cases {
		if got := Query(in); got != want {
			t.Errorf("Query(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3.0/items/geocode" || r.URL.Query().Get("key") != "k" || r.URL.Query().Get("fields") != "items.point" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		switch {
		case strings.Contains(r.URL.Query().Get("q"), "Садовая"):
			_, _ = w.Write([]byte(`{"meta":{"code":200},"result":{"items":[
				{"address_name":"Московская, 1","full_name":"Москва, Московская, 1","point":{"lat":55.75,"lon":37.61}},
				{"address_name":"Большая Садовая улица, 1","full_name":"Ростов-на-Дону, Большая Садовая улица, 1","point":{"lat":47.2205,"lon":39.7020}}]}}`))
		case strings.Contains(r.URL.Query().Get("q"), "limit"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"meta":{"code":403,"error":{"message":"key limit"}}}`))
		default:
			_, _ = w.Write([]byte(`{"meta":{"code":404,"error":{"message":"Results not found"}}}`))
		}
	}))
	defer srv.Close()
	g := New(srv.URL, "k")

	p, err := g.Find(context.Background(), "Большая Садовая, 1")
	if err != nil || p.Lat != 47.2205 || p.Address != "Большая Садовая улица, 1" {
		t.Fatalf("got %+v, %v (results outside Rostov must be skipped)", p, err)
	}
	if _, err := g.Find(context.Background(), "нигде"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := g.Find(context.Background(), "limit"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if _, err := New("", "").Find(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("no key: want ErrUnavailable, got %v", err)
	}
	broken := New("http://127.0.0.1:1", "secret")
	if _, err := broken.Find(context.Background(), "x"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error must not leak the key: %v", err)
	}
}
