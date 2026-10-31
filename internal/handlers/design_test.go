package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHomeHandler_OffersSearchExamplesAndKeyboardSkip(t *testing.T) {
	rr := httptest.NewRecorder()
	newHomeHandler(t).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, want := range []string{`href="#amarra-main"`, `href="/?q=Jane+Austen"`, `href="/?q=astronomy"`, `id="search-help"`, `aria-describedby="search-help"`} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("initial search page missing %q", want)
		}
	}
}
