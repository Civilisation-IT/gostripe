package api

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"gostripe/conf"
	"gostripe/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v72"
)

const (
	audHeaderName  = "X-JWT-AUD"
	defaultVersion = "unknown version"
)

var bearerRegexp = regexp.MustCompile(`^(?:B|b)earer (\S+$)`)

// API is the main REST API
type API struct {
	handler http.Handler
	db      *storage.Connection
	config  *conf.GlobalConfiguration
	version string
}

// NewAPIWithVersion creates a new REST API using the specified version
func NewAPIWithVersion(ctx context.Context, globalConfig *conf.GlobalConfiguration, db *storage.Connection, version string) *API {
	api := &API{config: globalConfig, db: db, version: version}

	// Initialize Stripe
	stripe.Key = globalConfig.Stripe.SecretKey

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(recoverer)
	r.Use(addRequestID(globalConfig))

	// CORS
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	r.Use(corsHandler.Handler)

	// Health check
	r.Get("/health", api.HealthCheck)

	// Stripe endpoints
	r.Post("/create-checkout-session", api.requireAuthentication(api.CreateCheckoutSession))
	r.Post("/webhooks", api.HandleWebhook)
	r.Get("/get-subscription-status", api.requireAuthentication(api.GetSubscriptionStatus))
	r.Post("/cancel-subscription", api.requireAuthentication(api.CancelSubscription))

	api.handler = r

	return api
}

// ListenAndServe starts the API server
func (a *API) ListenAndServe(hostAndPort string) {
	server := &http.Server{
		Addr:    hostAndPort,
		Handler: a.handler,
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		logrus.Infof("GoStripe API started on: %s", hostAndPort)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logrus.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-done

	// Create a deadline for server shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(ctx); err != nil {
		logrus.Fatalf("Server shutdown error: %v", err)
	}
}

// HealthCheck is the endpoint for checking the health of the API
func (a *API) HealthCheck(w http.ResponseWriter, r *http.Request) {
	sendJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": a.version,
	})
}
