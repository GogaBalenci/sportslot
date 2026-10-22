package payments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestTBankTokenSkipsNestedAndSortsKeys(t *testing.T) {
	params := map[string]interface{}{
		"TerminalKey": "TinkoffBankTest", "Amount": json.Number("19200"), "OrderId": "21090",
		"Description": "Подарочная карта на 1000 рублей", "DATA": map[string]string{"Phone": "+71234567890"},
		"Receipt": map[string]interface{}{"Email": "a@test.ru"},
	}
	// Конкатенация по алфавиту: Amount, Description, OrderId, Password, TerminalKey.
	want := sha256hex("19200" + "Подарочная карта на 1000 рублей" + "21090" + "usaf8fw8fsw21g" + "TinkoffBankTest")
	if got := TBankToken(params, "usaf8fw8fsw21g"); got != want {
		t.Fatalf("token %s, want %s", got, want)
	}
	params["Success"] = true
	withBool := TBankToken(params, "p")
	params["Success"] = false
	if TBankToken(params, "p") == withBool {
		t.Fatal("booleans must take part in the token as true/false")
	}
}

func TestParseTBankNotification(t *testing.T) {
	params := map[string]interface{}{"TerminalKey": "T", "OrderId": "o-1", "Success": true, "Status": "CONFIRMED",
		"PaymentId": json.Number("13660"), "ErrorCode": "0", "Amount": json.Number("100000")}
	params["Token"] = TBankToken(params, "secret")
	raw, _ := json.Marshal(params)
	n, err := ParseTBankNotification(raw, "secret")
	if err != nil || n.OrderID != "o-1" || n.PaymentID != "13660" || n.Amount != 100000 || TBankStatus(n.Status) != StatusSucceeded {
		t.Fatalf("notification: %+v %v", n, err)
	}
	if _, err := ParseTBankNotification(raw, "other"); err != ErrBadSignature {
		t.Fatalf("wrong password must fail, got %v", err)
	}
	if PeekTBankOrder(raw) != "o-1" {
		t.Fatal("peek order id")
	}
}

func TestMoneyConversion(t *testing.T) {
	for _, c := range []struct {
		kop  int
		text string
	}{{40000, "400.00"}, {105, "1.05"}, {1, "0.01"}} {
		if rubles(c.kop) != c.text || kopecks(c.text) != c.kop {
			t.Fatalf("%d <-> %s", c.kop, c.text)
		}
	}
	if kopecks("400.5") != 40050 {
		t.Fatal("one decimal digit")
	}
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
