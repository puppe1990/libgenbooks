package zlibrary

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/cais/fakedata"
)

type fixtureBook struct {
	ID        int64  `json:"id"`
	Hash      string `json:"hash"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Extension string `json:"extension"`
	Filesize  int64  `json:"filesize"`
	MD5       string `json:"md5"`
}

// booksPayload renders the API's search envelope for any set of books.
func booksPayload(t *testing.T, books ...fixtureBook) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"success": 1, "books": books})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestSearch_SendsMessageInQueryAndParsesBooks(t *testing.T) {
	fakedata.Seed(42)
	title := fakedata.Title()
	author := fakedata.Name()
	query := fakedata.Sentence()
	fixture := booksPayload(t,
		fixtureBook{ID: 124226041, Hash: "4345e0", Title: title, Author: author, Extension: "epub", Filesize: 2828749, MD5: "8fa84457392dcd39d561ad5c9dc30723"},
		fixtureBook{ID: 5731855, Hash: "f0a9d9", Title: fakedata.Title(), Author: fakedata.Name(), Extension: "pdf", Filesize: 100},
	)

	var gotPath, gotQuery, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("message")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(srv.Close)

	books, err := New(srv.URL).Search(context.Background(), query, 1)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/eapi/book/search" || gotQuery != query {
		t.Fatalf("request = %s?message=%q, want the query in the URL", gotPath, gotQuery)
	}
	if !strings.Contains(gotBody, `"page":1`) {
		t.Errorf("body = %q, want the page", gotBody)
	}
	if len(books) != 2 {
		t.Fatalf("books = %+v, want 2", books)
	}
	first := books[0]
	if first.ID != 124226041 || first.Hash != "4345e0" || first.MD5 != "8fa84457392dcd39d561ad5c9dc30723" {
		t.Errorf("first = %+v", first)
	}
	if first.Title != title || first.Author != author || first.Extension != "epub" || first.Filesize != 2828749 {
		t.Errorf("first = %+v, want the fixture fields decoded", first)
	}
}

func TestSearch_SurfacesTheAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":0,"error":"Requested book not found"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := New(srv.URL).Search(context.Background(), fakedata.Sentence(), 1)
	if err == nil || !strings.Contains(err.Error(), "Requested book not found") {
		t.Fatalf("err = %v, want the API error text", err)
	}
}

func TestDownloadLink_LogsInWithFormAndSendsTheSessionCookie(t *testing.T) {
	fakedata.Seed(42)
	email := fakedata.Email()
	password := fakedata.Sentence()
	link := fakedata.URL()

	var logins int
	var loginForm, loginType, fileCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/eapi/user/login":
			logins++
			loginType = r.Header.Get("Content-Type")
			_ = r.ParseForm()
			loginForm = r.PostFormValue("email") + "/" + r.PostFormValue("password")
			_, _ = w.Write([]byte(`{"success":1,"user":{"id":42,"remix_userkey":"KEY1"}}`))
		case "/eapi/book/1/abc123/file":
			fileCookie = r.Header.Get("Cookie")
			payload, _ := json.Marshal(map[string]any{"success": 1, "file": map[string]string{"downloadLink": link}})
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL)
	client.Email = email
	client.Password = password
	book := Book{ID: 1, Hash: "abc123"}

	got, err := client.DownloadLink(context.Background(), book)
	if err != nil {
		t.Fatal(err)
	}
	if got != link {
		t.Fatalf("link = %q, want %q", got, link)
	}
	if !strings.Contains(loginType, "application/x-www-form-urlencoded") {
		t.Errorf("login content-type = %q, want form-encoded", loginType)
	}
	if loginForm != email+"/"+password {
		t.Errorf("login form = %q, want the configured credentials", loginForm)
	}
	if !strings.Contains(fileCookie, "remix_userid=42") || !strings.Contains(fileCookie, "remix_userkey=KEY1") {
		t.Errorf("file cookie = %q", fileCookie)
	}

	if _, err := client.DownloadLink(context.Background(), book); err != nil {
		t.Fatal(err)
	}
	if logins != 1 {
		t.Errorf("logins = %d, want the session reused", logins)
	}
}

func TestDownloadLink_ReloginsWhenTheSessionExpires(t *testing.T) {
	fakedata.Seed(42)
	link := fakedata.URL()

	var logins, fileCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/eapi/user/login":
			logins++
			key := "K1"
			if logins > 1 {
				key = "K2"
			}
			_, _ = w.Write([]byte(`{"success":1,"user":{"id":7,"remix_userkey":"` + key + `"}}`))
		case "/eapi/book/2/def456/file":
			fileCalls++
			if strings.Contains(r.Header.Get("Cookie"), "K1") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"success":0,"error":"Please login"}`))
				return
			}
			payload, _ := json.Marshal(map[string]any{"success": 1, "file": map[string]string{"downloadLink": link}})
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := New(srv.URL)
	client.Email = fakedata.Email()
	client.Password = fakedata.Sentence()
	got, err := client.DownloadLink(context.Background(), Book{ID: 2, Hash: "def456"})
	if err != nil {
		t.Fatal(err)
	}
	if got != link {
		t.Fatalf("link = %q, want %q", got, link)
	}
	if logins != 2 || fileCalls != 2 {
		t.Fatalf("logins/fileCalls = %d/%d, want one relogin", logins, fileCalls)
	}
}

func TestDownloadLink_WithoutCredentialsNamesTheEnvVars(t *testing.T) {
	client := New(fakedata.URL())
	_, err := client.DownloadLink(context.Background(), Book{ID: 1, Hash: "abc123"})
	if err == nil || !strings.Contains(err.Error(), "LIBGEN_ZLIB_EMAIL") {
		t.Fatalf("err = %v, want the missing credentials named", err)
	}
}

func TestDownloadLink_RejectsBooksWithoutIDOrHash(t *testing.T) {
	client := New(fakedata.URL())
	if _, err := client.DownloadLink(context.Background(), Book{ID: 0, Hash: "abc"}); err == nil {
		t.Fatal("book without id must fail")
	}
	if _, err := client.DownloadLink(context.Background(), Book{ID: 1}); err == nil {
		t.Fatal("book without hash must fail")
	}
}
