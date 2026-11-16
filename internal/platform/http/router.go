package http

import (
	"net/http"

	"flag_backend/internal/config"
	"flag_backend/internal/middleware"
	"flag_backend/internal/modules/admin"
	"flag_backend/internal/modules/environment"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewRouter(db *pgxpool.Pool, cfg config.Config) http.Handler {
	repository := admin.NewRepository(db)
	service := admin.NewService(repository, cfg)
	handler := admin.NewHandler(service, cfg.CookieSecure)

	router := http.NewServeMux()
	router.HandleFunc("POST /api/admin/login", handler.Login)
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/admin/logout", handler.Logout)
	protected.HandleFunc("GET /api/admin/me", handler.Me)
	router.Handle("/api/admin/", middleware.Authenticate([]byte(cfg.JWTSecret))(protected))

	environmentRepository := environment.NewRepository(db)
	environmentService := environment.NewService(environmentRepository)
	environmentHandler := environment.NewHandler(environmentService)
	environmentRoutes := http.NewServeMux()
	environmentRoutes.HandleFunc("GET /api/environments", environmentHandler.List)
	environmentRoutes.HandleFunc("GET /api/environments/{id}", environmentHandler.Get)
	router.Handle("/api/environments", middleware.Authenticate([]byte(cfg.JWTSecret))(environmentRoutes))
	router.Handle("/api/environments/", middleware.Authenticate([]byte(cfg.JWTSecret))(environmentRoutes))
	return middleware.CORS(cfg.AllowedOrigins)(router)
}
