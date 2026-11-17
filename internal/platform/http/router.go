package http

import (
	"net/http"

	"flag_backend/internal/config"
	"flag_backend/internal/middleware"
	"flag_backend/internal/modules/admin"
	"flag_backend/internal/modules/apikey"
	"flag_backend/internal/modules/audit"
	"flag_backend/internal/modules/configuration"
	"flag_backend/internal/modules/environment"
	"flag_backend/internal/modules/feature"
	"flag_backend/internal/modules/sdk"
	"flag_backend/internal/modules/usergroup"
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
	userGroupRepository := usergroup.NewRepository(db)
	userGroupHandler := usergroup.NewHandler(usergroup.NewService(userGroupRepository))
	environmentRoutes.HandleFunc("GET /api/environments/{environmentId}/configs", configurationHandler.List)
	environmentRoutes.HandleFunc("GET /api/environments/{environmentId}/configs/{featureKey}", configurationHandler.Get)
	environmentRoutes.HandleFunc("PUT /api/environments/{environmentId}/configs/{featureKey}", configurationHandler.Update)
	environmentRoutes.HandleFunc("GET /api/environments/{environmentId}/features/{featureId}/groups", userGroupHandler.GetEffective)
	environmentRoutes.HandleFunc("PUT /api/environments/{environmentId}/features/{featureId}/groups/{groupId}", userGroupHandler.PutRule)
	environmentRoutes.HandleFunc("DELETE /api/environments/{environmentId}/features/{featureId}/groups/{groupId}", userGroupHandler.DeleteRule)
	router.Handle("/api/environments", middleware.Authenticate([]byte(cfg.JWTSecret))(environmentRoutes))
	router.Handle("/api/environments/", middleware.Authenticate([]byte(cfg.JWTSecret))(environmentRoutes))
	userGroupRoutes := http.NewServeMux()
	userGroupRoutes.HandleFunc("POST /api/user-groups", userGroupHandler.Create)
	userGroupRoutes.HandleFunc("GET /api/user-groups", userGroupHandler.List)
	userGroupRoutes.HandleFunc("GET /api/user-groups/{id}", userGroupHandler.Get)
	userGroupRoutes.HandleFunc("PATCH /api/user-groups/{id}", userGroupHandler.Patch)
	userGroupRoutes.HandleFunc("DELETE /api/user-groups/{id}", userGroupHandler.Archive)
	router.Handle("/api/user-groups", middleware.Authenticate([]byte(cfg.JWTSecret))(userGroupRoutes))
	router.Handle("/api/user-groups/", middleware.Authenticate([]byte(cfg.JWTSecret))(userGroupRoutes))

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
	apiKeyRepository := apikey.NewRepository(db)
	apiKeyHandler := apikey.NewHandler(apikey.NewService(apiKeyRepository))
	apiKeyRoutes := http.NewServeMux()
	apiKeyRoutes.HandleFunc("POST /api-keys", apiKeyHandler.Create)
	apiKeyRoutes.HandleFunc("GET /api-keys", apiKeyHandler.List)
	apiKeyRoutes.HandleFunc("DELETE /api-keys/{id}", apiKeyHandler.Revoke)
	router.Handle("/api-keys", middleware.Authenticate([]byte(cfg.JWTSecret))(apiKeyRoutes))
	router.Handle("/api-keys/", middleware.Authenticate([]byte(cfg.JWTSecret))(apiKeyRoutes))
	auditRepository := audit.NewRepository(db)
	auditHandler := audit.NewHandler(audit.NewService(auditRepository))
	auditRoutes := http.NewServeMux()
	auditRoutes.HandleFunc("GET /audit-logs", auditHandler.List)
	auditRoutes.HandleFunc("GET /audit-logs/{id}", auditHandler.Get)
	router.Handle("/audit-logs", middleware.Authenticate([]byte(cfg.JWTSecret))(auditRoutes))
	router.Handle("/audit-logs/", middleware.Authenticate([]byte(cfg.JWTSecret))(auditRoutes))
	sdkHandler := sdk.NewHandler(configurationRepository, configurationService)
	sdkRoutes := http.NewServeMux()
	sdkRoutes.HandleFunc("GET /sdk/v1/configs", sdkHandler.List)
	sdkRoutes.HandleFunc("GET /sdk/v1/configs/{featureKey}", sdkHandler.Get)
	router.Handle("/sdk/v1/", apikey.AuthenticateSDKKey(apiKeyRepository)(sdkRoutes))
	return middleware.CORS(cfg.AllowedOrigins)(router)
}
