package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/puppe1990/amarra-cais/pkg/cais/fakedata"

	"github.com/puppe1990/libgenbooks/internal/zlibrary"
)

func TestQuotaHandler_DisabledWithoutAClient(t *testing.T) {
	rr := httptest.NewRecorder()
	NewQuotaHandler(nil).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/zlib/quota", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != false {
		t.Fatalf("body = %v, want enabled=false", body)
	}
}

func TestQuotaHandler_ReportsTheAccountQuota(t *testing.T) {
	fakedata.Seed(42)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eapi/user/login" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"success":1,"user":{"id":3,"remix_userkey":"KEY","downloads_today":4,"downloads_limit":10}}`))
	}))
	t.Cleanup(srv.Close)

	client := zlibrary.New(srv.URL)
	client.Email = fakedata.Email()
	client.Password = fakedata.Sentence()

	rr := httptest.NewRecorder()
	NewQuotaHandler(client).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/zlib/quota", nil))

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != true || body["used"] != float64(4) || body["limit"] != float64(10) {
		t.Fatalf("body = %v, want 4/10", body)
	}
	resetAt, err := time.Parse(time.RFC3339, body["resetAt"].(string))
	if err != nil {
		t.Fatalf("resetAt = %v, want RFC3339", body["resetAt"])
	}
	if hour, minute, second := resetAt.UTC().Clock(); hour != 21 || minute != 0 || second != 0 {
		t.Fatalf("resetAt = %v, want 21:00 UTC (00:00 UTC+3)", resetAt)
	}
	if until := time.Until(resetAt); until <= 0 || until > 25*time.Hour {
		t.Fatalf("resetAt = %v, want the next reset within a day", resetAt)
	}
}

func TestNextQuotaReset_UsesTheUTCPlusThreeMidnight(t *testing.T) {
	at := func(hour int) time.Time {
		return time.Date(2026, 11, 3, hour, 30, 0, 0, time.UTC)
	}
	sameDay := nextQuotaReset(at(14))
	wantSameDay := time.Date(2026, 11, 3, 21, 0, 0, 0, time.UTC)
	if !sameDay.Equal(wantSameDay) {
		t.Fatalf("nextQuotaReset(14:30 UTC) = %v, want %v", sameDay, wantSameDay)
	}
	nextDay := nextQuotaReset(at(22))
	wantNextDay := time.Date(2026, 11, 4, 21, 0, 0, 0, time.UTC)
	if !nextDay.Equal(wantNextDay) {
		t.Fatalf("nextQuotaReset(22:30 UTC) = %v, want %v", nextDay, wantNextDay)
	}
}

func TestQuotaHandler_UnavailableWhenTheAPIFails(t *testing.T) {
	fakedata.Seed(42)
	client := zlibrary.New("https://zlibrary.invalid")
	client.Email = fakedata.Email()
	client.Password = fakedata.Sentence()

	rr := httptest.NewRecorder()
	NewQuotaHandler(client).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/zlib/quota", nil))

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != true || body["unavailable"] != true {
		t.Fatalf("body = %v, want enabled+unavailable", body)
	}
}
