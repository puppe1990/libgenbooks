package handlers

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/view"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/flash"
	"github.com/puppe1990/amarra-cais/pkg/cais/httpx"
	"github.com/puppe1990/amarra-cais/pkg/cais/i18n"
	"github.com/puppe1990/amarra-cais/pkg/cais/meta"
	"github.com/puppe1990/amarra-cais/pkg/cais/session"
	"github.com/puppe1990/amarra-cais/pkg/cais/validate"

	"github.com/puppe1990/libgenbooks/internal/store"
)

type ConfiguracoesHandler struct {
	views   *view.Renderer
	store   store.Store
	site    meta.Site
	catalog *i18n.Catalog
	cfg     cais.Config
}

func NewConfiguracoesHandler(views *view.Renderer, s store.Store, site meta.Site, catalog *i18n.Catalog, cfg cais.Config) *ConfiguracoesHandler {
	return &ConfiguracoesHandler{views: views, store: s, site: site, catalog: catalog, cfg: cfg}
}

func (h *ConfiguracoesHandler) t(r *http.Request, key string, args ...any) string {
	return i18n.CatalogOr(r, h.catalog).T(key, args...)
}

func (h *ConfiguracoesHandler) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := session.UserID(r); !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	h.render(w, r, nil, 0)
}

func (h *ConfiguracoesHandler) Post(w http.ResponseWriter, r *http.Request) {
	userID, ok := session.UserID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := httpx.ParseFormOrJSON(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	user, err := h.store.FindUser(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if errs := h.passwordFieldErrors(r, user.PasswordHash); errs.Any() {
		h.render(w, r, errs, http.StatusUnprocessableEntity)
		return
	}
	h.saveNewPassword(w, r, userID)
}

func (h *ConfiguracoesHandler) passwordFieldErrors(r *http.Request, currentHash string) validate.FieldErrors {
	var errs validate.FieldErrors
	password := r.FormValue("password")
	if !session.VerifyPassword(currentHash, r.FormValue("current_password")) {
		errs.Add("current_password", h.t(r, "config.wrong_password"))
	}
	if err := validate.MinLength(password, 8); err != nil {
		errs.Add("password", h.t(r, "auth.password_too_short"))
	}
	if password != r.FormValue("password_confirmation") {
		errs.Add("password_confirmation", h.t(r, "auth.password_mismatch"))
	}
	return errs
}

func (h *ConfiguracoesHandler) saveNewPassword(w http.ResponseWriter, r *http.Request, userID int64) {
	hash, err := session.HashPassword(r.FormValue("password"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.store.ChangePassword(userID, hash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := session.SignOut(w, h.store.Sessions(), r, session.CookieOptionsFromConfig(h.cfg)); err != nil {
		httpx.ServerError(w, err, h.cfg)
		return
	}
	flash.Set(w, "notice", h.t(r, "auth.reset_success"), h.cfg.CookieSecure())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *ConfiguracoesHandler) render(w http.ResponseWriter, r *http.Request, errs validate.FieldErrors, status int) {
	writeView(w, r, h.views, h.cfg, "app", "configuracoes", amarraData(r, h.site, map[string]any{
		"Title":     h.t(r, "config.title"),
		"ActiveNav": "configuracoes",
		"Errors":    errs,
	}), status)
}
