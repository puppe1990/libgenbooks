package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/passwordreset"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/libgenbooks/internal/store"
)

const afterAuthPath = "/"

type AuthHandler struct {
	views       *view.Renderer
	store       store.Store
	site        meta.Site
	sessions    session.Store
	cfg         cais.Config
	catalog     *i18n.Catalog
	resetNotify passwordreset.Notifier
}

func NewAuthHandler(views *view.Renderer, s store.Store, site meta.Site, sessions session.Store, cfg cais.Config, catalog *i18n.Catalog) *AuthHandler {
	return &AuthHandler{views: views, store: s, site: site, sessions: sessions, cfg: cfg, catalog: catalog}
}

// t translates Go-side copy with the request locale (#211); h.catalog is the
// boot fallback for requests that skipped LocaleMiddleware (unit tests).
func (h *AuthHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *AuthHandler) renderAuth(w http.ResponseWriter, r *http.Request, name string, extra map[string]any, status int) {
	if extra == nil {
		extra = map[string]any{}
	}
	if _, ok := extra["Title"]; !ok {
		extra["Title"] = h.t(r, "auth.login_title")
	}
	writeView(w, r, h.views, h.cfg, "app", name, amarraData(r, h.site, extra), status)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if _, ok := session.UserID(r); ok {
		http.Redirect(w, r, afterAuthPath, http.StatusSeeOther)
		return
	}
	email := ""
	if h.cfg.Env != "production" {
		email = "demo@example.com"
	}
	h.renderAuth(w, r, "login", map[string]any{
		"Title": h.t(r, "auth.login_title"),
		"Email": email,
	}, 0)
}

func (h *AuthHandler) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	user, err := h.store.FindUserByEmail(email)
	if err != nil || !session.VerifyPassword(user.PasswordHash, password) {
		h.renderAuth(w, r, "login", map[string]any{
			"Title":  h.t(r, "auth.login_title"),
			"Email":  email,
			"Errors": validate.FieldErrors{"email": h.t(r, "auth.invalid_credentials")},
		}, http.StatusUnprocessableEntity)
		return
	}

	if err := session.SignIn(w, h.sessions, r, user.ID, session.CookieOptionsFromConfig(h.cfg)); err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	flash.Set(w, "notice", h.t(r, "auth.welcome"), h.cfg.CookieSecure())
	http.Redirect(w, r, afterAuthPath, http.StatusSeeOther)
}

func (h *AuthHandler) LogoutPost(w http.ResponseWriter, r *http.Request) {
	if err := session.SignOut(w, h.sessions, r, session.CookieOptionsFromConfig(h.cfg)); err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	if _, ok := session.UserID(r); ok {
		http.Redirect(w, r, afterAuthPath, http.StatusSeeOther)
		return
	}
	h.renderAuth(w, r, "signup", map[string]any{"Title": h.t(r, "auth.signup_title")}, 0)
}

func (h *AuthHandler) SignUpPost(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirmation")

	var errs validate.FieldErrors
	if err := validate.Email(email); err != nil {
		errs.Add("email", h.t(r, "auth.email_invalid"))
	}
	if err := validate.MinLength(password, 8); err != nil {
		errs.Add("password", h.t(r, "auth.password_too_short"))
	}
	if password != confirm {
		errs.Add("password_confirmation", h.t(r, "auth.password_mismatch"))
	}
	if errs.Any() {
		h.renderAuth(w, r, "signup", map[string]any{
			"Title":  h.t(r, "auth.signup_title"),
			"Email":  email,
			"Errors": errs,
		}, http.StatusUnprocessableEntity)
		return
	}

	hash, err := session.HashPassword(password)
	if err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	userID, err := h.store.CreateUser(email, hash)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			h.renderAuth(w, r, "signup", map[string]any{
				"Title":  h.t(r, "auth.signup_title"),
				"Email":  email,
				"Errors": validate.FieldErrors{"email": h.t(r, "auth.email_taken")},
			}, http.StatusUnprocessableEntity)
			return
		}
		httpx.ServerError(w, err, h.cfg)
		return
	}

	if err := session.SignIn(w, h.sessions, r, userID, session.CookieOptionsFromConfig(h.cfg)); err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	flash.Set(w, "notice", h.t(r, "auth.welcome"), h.cfg.CookieSecure())
	http.Redirect(w, r, afterAuthPath, http.StatusSeeOther)
}

func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := session.UserID(r); ok {
		http.Redirect(w, r, afterAuthPath, http.StatusSeeOther)
		return
	}
	h.renderAuth(w, r, "forgot_password", map[string]any{"Title": h.t(r, "auth.forgot_password_title")}, 0)
}

func (h *AuthHandler) ForgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	var errs validate.FieldErrors
	if err := validate.Email(email); err != nil {
		errs.Add("email", h.t(r, "auth.email_invalid"))
	}
	if errs.Any() {
		h.renderAuth(w, r, "forgot_password", map[string]any{
			"Title":  h.t(r, "auth.forgot_password_title"),
			"Email":  email,
			"Errors": errs,
		}, http.StatusUnprocessableEntity)
		return
	}

	if user, err := h.store.FindUserByEmail(email); err == nil {
		token, err := h.store.CreatePasswordResetToken(user.ID)
		if err != nil {
			// Log without the token: a DB failure must not change the public
			// response or reveal whether the account exists (#223).
			log.Printf("password reset: create token for %s: %v", user.Email, err)
		} else if err := h.resetNotifier().NotifyReset(user.Email, token); err != nil {
			// Delivery failed. Log it and invalidate the token so no live
			// credential is left behind; the response stays identical to a
			// missing account so we do not enumerate users (#223).
			log.Printf("password reset: deliver for %s: %v", user.Email, err)
			if err := h.store.ClearPasswordResetTokens(user.ID); err != nil {
				log.Printf("password reset: clear failed token for %s: %v", user.Email, err)
			}
		}
	}

	flash.Set(w, "notice", h.t(r, "auth.reset_email_sent"), h.cfg.CookieSecure())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if _, ok := session.UserID(r); ok {
		http.Redirect(w, r, afterAuthPath, http.StatusSeeOther)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	extra := map[string]any{"Title": h.t(r, "auth.reset_password_title"), "Token": token}
	if token == "" {
		extra["Errors"] = validate.FieldErrors{"token": h.t(r, "auth.reset_invalid_token")}
		h.renderAuth(w, r, "reset_password", extra, http.StatusUnprocessableEntity)
		return
	}
	if _, ok := h.store.FindPasswordResetUserID(token); !ok {
		extra["Errors"] = validate.FieldErrors{"token": h.t(r, "auth.reset_invalid_token")}
		h.renderAuth(w, r, "reset_password", extra, http.StatusUnprocessableEntity)
		return
	}
	h.renderAuth(w, r, "reset_password", extra, 0)
}

func (h *AuthHandler) ResetPasswordPost(w http.ResponseWriter, r *http.Request) {
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirmation")

	var errs validate.FieldErrors
	if token == "" {
		errs.Add("token", h.t(r, "auth.reset_invalid_token"))
	} else if _, ok := h.store.FindPasswordResetUserID(token); !ok {
		h.renderAuth(w, r, "reset_password", map[string]any{
			"Title":  h.t(r, "auth.reset_password_title"),
			"Token":  token,
			"Errors": validate.FieldErrors{"token": h.t(r, "auth.reset_invalid_token")},
		}, http.StatusUnprocessableEntity)
		return
	}
	if err := validate.MinLength(password, 8); err != nil {
		errs.Add("password", h.t(r, "auth.password_too_short"))
	}
	if password != confirm {
		errs.Add("password_confirmation", h.t(r, "auth.password_mismatch"))
	}
	if errs.Any() {
		h.renderAuth(w, r, "reset_password", map[string]any{
			"Title":  h.t(r, "auth.reset_password_title"),
			"Token":  token,
			"Errors": errs,
		}, http.StatusUnprocessableEntity)
		return
	}

	hash, err := session.HashPassword(password)
	if err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	if err := h.store.ResetPasswordWithToken(token, hash); err != nil {
		h.renderAuth(w, r, "reset_password", map[string]any{
			"Title":  h.t(r, "auth.reset_password_title"),
			"Token":  token,
			"Errors": validate.FieldErrors{"token": h.t(r, "auth.reset_invalid_token")},
		}, http.StatusUnprocessableEntity)
		return
	}

	flash.Set(w, "notice", h.t(r, "auth.reset_success"), h.cfg.CookieSecure())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *AuthHandler) resetNotifier() passwordreset.Notifier {
	if h.resetNotify != nil {
		return h.resetNotify
	}
	return passwordreset.NotifierFromConfig(h.cfg, h.site.AppName)
}
