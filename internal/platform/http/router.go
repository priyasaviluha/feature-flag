package http

import (
	"net/http"

	"flag_backend/internal/config"
	"flag_backend/internal/middleware"
	"flag_backend/internal/modules/admin"
	"flag_backend/internal/modules/configuration"
	"flag_backend/internal/modules/environment"
	"flag_backend/internal/modules/feature"
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
	environmentRoutes.HandleFunc("PATCH /api/environments/{id}", environmentHandler.Update)
	environmentRoutes.HandleFunc("DELETE /api/environments/{id}", environmentHandler.Delete)
	configurationRepository := configuration.NewRepository(db)
	configurationService := configuration.NewService(configurationRepository)
	configurationHandler := configuration.NewHandler(configurationService)
	environmentRoutes.HandleFunc("GET /api/environments/{environmentId}/configs", configurationHandler.List)
	environmentRoutes.HandleFunc("GET /api/environments/{environmentId}/configs/{featureKey}", configurationHandler.Get)
	environmentRoutes.HandleFunc("PUT /api/environments/{environmentId}/configs/{featureKey}", configurationHandler.Update)
	router.Handle("/api/environments", middleware.Authenticate([]byte(cfg.JWTSecret))(environmentRoutes))
	router.Handle("/api/environments/", middleware.Authenticate([]byte(cfg.JWTSecret))(environmentRoutes))

	featureRepository := feature.NewRepository(db)
	featureService := feature.NewService(featureRepository)
	featureHandler := feature.NewHandler(featureService)
	featureRoutes := http.NewServeMux()
	featureRoutes.HandleFunc("POST /api/features", featureHandler.Create)
	featureRoutes.HandleFunc("GET /api/features", featureHandler.List)
	featureRoutes.HandleFunc("GET /api/features/{id}", featureHandler.Get)
	featureRoutes.HandleFunc("PATCH /api/features/{id}", featureHandler.Update)
	featureRoutes.HandleFunc("DELETE /api/features/{id}", featureHandler.Archive)
	router.Handle("/api/features", middleware.Authenticate([]byte(cfg.JWTSecret))(featureRoutes))
	router.Handle("/api/features/", middleware.Authenticate([]byte(cfg.JWTSecret))(featureRoutes))
	return middleware.CORS(cfg.AllowedOrigins)(router)
}
