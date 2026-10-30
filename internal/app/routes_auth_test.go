package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"

	"github.com/puppe1990/libgenbooks/internal/catalog"
	appi18n "github.com/puppe1990/libgenbooks/internal/i18n"
	"github.com/puppe1990/libgenbooks/internal/libgen"
	"github.com/puppe1990/libgenbooks/internal/store"
)

func testApp(t *testing.T) (*App, store.Store) {
	t.Helper()
	users, err := store.NewSQLiteStore(":memory:", "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = users.Close() })
	books, err := catalog.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = books.Close() })

	root := projectRoot(t)
	views, err := view.Load(os.DirFS(filepath.Join(root, "web", "templates")), appi18n.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(cais.Config{Env: "test", Port: ":0"}, Deps{
		Views:     views,
		Store:     users,
		StaticDir: filepath.Join(root, "web", "static"),
		Books:     books,
		Libgen:    libgen.New("https://libgen.invalid"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, users
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("go.mod not found")
		}
		wd = parent
	}
}

func TestApp_ProtectedRoutesRedirectAnonymous(t *testing.T) {
	a, _ := testApp(t)
	paths := []string{
		"/",
		"/downloads",
		"/download?md5=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"/cover?md5=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"/configuracoes",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		a.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Errorf("%s status = %d, want 303", path, rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/login" {
			t.Errorf("%s Location = %q, want /login", path, loc)
		}
	}
}

func TestApp_LoginSignupHealthStayPublic(t *testing.T) {
	a, _ := testApp(t)
	for _, path := range []string{"/login", "/signup", "/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		a.Handler().ServeHTTP(rr, req)
		if rr.Code == http.StatusSeeOther && rr.Header().Get("Location") == "/login" {
			t.Errorf("%s bounced to /login", path)
		}
		if rr.Code >= 400 {
			t.Errorf("%s status = %d", path, rr.Code)
		}
	}
}

func TestApp_SignedInServesHome(t *testing.T) {
	a, users := testApp(t)
	hash, err := session.HashPassword("password12")
	if err != nil {
		t.Fatal(err)
	}
	id, err := users.CreateUser("reader@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	token, err := users.Sessions().Create(id)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: session.DefaultCookieName, Value: token})
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `name="q"`) {
		t.Fatalf("signed-in home missing search field, got: %s", rr.Body.String())
	}
}

func TestApp_DashboardRouteGone(t *testing.T) {
	a, users := testApp(t)
	hash, err := session.HashPassword("password12")
	if err != nil {
		t.Fatal(err)
	}
	id, err := users.CreateUser("reader@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	token, err := users.Sessions().Create(id)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: session.DefaultCookieName, Value: token})
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatal("/dashboard must not render")
	}
	if loc := rr.Header().Get("Location"); loc == "/dashboard" {
		t.Fatal("/dashboard must not redirect to itself")
	}
}

func TestApp_SignedInServesConfiguracoes(t *testing.T) {
	a, users := testApp(t)
	hash, err := session.HashPassword("password12")
	if err != nil {
		t.Fatal(err)
	}
	id, err := users.CreateUser("reader@example.com", hash)
	if err != nil {
		t.Fatal(err)
	}
	token, err := users.Sessions().Create(id)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/configuracoes", nil)
	req.AddCookie(&http.Cookie{Name: session.DefaultCookieName, Value: token})
	rr := httptest.NewRecorder()
	a.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `name="current_password"`) {
		t.Fatalf("configuracoes missing password form, got: %s", body)
	}
	if strings.Contains(body, `href="/dashboard"`) {
		t.Fatal("signed-in chrome must not link /dashboard")
	}
}
