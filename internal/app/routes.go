package app

import (
	"net/http"

	"github.com/puppe1990/amarra-cais/pkg/amarra/live"
	"github.com/puppe1990/amarra-cais/pkg/cais"
	"github.com/puppe1990/amarra-cais/pkg/cais/middleware"

	"github.com/puppe1990/libgenbooks/internal/handlers"
)

func registerRoutes(r *cais.Router, deps Deps, cfg cais.Config) {
	home := handlers.NewHomeHandler(deps.Views, deps.Site, deps.Catalog, cfg, deps.Books, deps.Libgen)
	download := handlers.NewDownloadHandler(deps.Views, deps.Site, deps.Catalog, cfg, deps.Books, deps.Libgen)
	downloads := handlers.NewDownloadsHandler(deps.Views, deps.Site, deps.Catalog, cfg, deps.Books)
	cover := handlers.NewCoverHandler(deps.Libgen)
	dashboard := handlers.NewDashboardHandler(deps.Views, deps.Store, deps.Site, deps.Catalog, cfg)
	auth := handlers.NewAuthHandler(deps.Views, deps.Store, deps.Site, deps.Store.Sessions(), cfg, deps.Catalog)

	loginLimit := middleware.NewRateLimiter(10, cfg)
	resetLimit := middleware.NewRateLimiter(10, cfg)

	r.Get("/", middleware.RequireAuthFunc("/login", home.ServeHTTP))
	r.Get("/download", middleware.RequireAuthFunc("/login", download.ServeHTTP))
	r.Get("/downloads", middleware.RequireAuthFunc("/login", downloads.ServeHTTP))
	r.Get("/cover", middleware.RequireAuthFunc("/login", cover.ServeHTTP))
	r.Get("/login", auth.Login)
	r.Post("/login", loginLimit.Middleware(http.HandlerFunc(auth.LoginPost)).ServeHTTP)
	r.Get("/signup", auth.SignUp)
	r.Post("/signup", loginLimit.Middleware(http.HandlerFunc(auth.SignUpPost)).ServeHTTP)
	r.Get("/forgot-password", auth.ForgotPassword)
	r.Post("/forgot-password", resetLimit.Middleware(http.HandlerFunc(auth.ForgotPasswordPost)).ServeHTTP)
	r.Get("/reset-password", auth.ResetPassword)
	r.Post("/reset-password", resetLimit.Middleware(http.HandlerFunc(auth.ResetPasswordPost)).ServeHTTP)
	r.Post("/logout", auth.LogoutPost)
	r.Post("/locale", handlers.PostLocale(cfg))
	r.Get("/dashboard", middleware.RequireAuthFunc("/login", dashboard.ServeHTTP))
}

func registerLiveViews(hub *live.Hub, deps Deps) {
	_ = hub
	_ = deps
	// cais:live-views
}
