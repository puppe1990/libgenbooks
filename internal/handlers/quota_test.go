package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
