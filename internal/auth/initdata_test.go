package auth

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const token = "test-bot-token"

func sample(now time.Time) map[string]string {
	return map[string]string{
		"auth_date":   "1771409719",
		"chat":        `{"id":12345,"type":"DIALOG"}`,
		"query_id":    "4c0ab423-342b-4e45-aea4-2747dbc500cd",
		"start_param": "venue_abc",
		"user":        `{"id":67890,"first_name":"Max","last_name":"User","username":null}`,
	}
}

func TestValidateOK(t *testing.T) {
	now := time.Unix(1771409719, 0).Add(10 * time.Minute)
	data, err := Validate(BuildInitData(sample(now), token), token, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if data.User.MaxUserID() != "67890" || data.User.Name() != "Max User" || data.StartParam != "venue_abc" {
		t.Fatalf("unexpected launch data: %+v", data)
	}
}

func TestValidateRejectsTampering(t *testing.T) {
	now := time.Unix(1771409719, 0)
	signed := BuildInitData(sample(now), token)
	tampered := strings.Replace(signed, "67890", "11111", 1)
	if _, err := Validate(tampered, token, time.Hour, now); !errors.Is(err, ErrSignature) {
		t.Fatalf("want ErrSignature, got %v", err)
	}
	if _, err := Validate(signed, "other-token", time.Hour, now); !errors.Is(err, ErrSignature) {
		t.Fatalf("want ErrSignature for wrong token, got %v", err)
	}
	if _, err := Validate(signed+"&user=%7B%7D", token, time.Hour, now); !errors.Is(err, ErrMalformed) {
		t.Fatalf("want ErrMalformed for duplicate key, got %v", err)
	}
}

func TestValidateExpired(t *testing.T) {
	now := time.Unix(1771409719, 0).Add(2 * time.Hour)
	if _, err := Validate(BuildInitData(sample(now), token), token, time.Hour, now); !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}
