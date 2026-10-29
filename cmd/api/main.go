package main

import (
	"log"
	"net/http"
	"os"
	"time"

	httpadapter "github.com/code-corhuila/drp-notification-api/internal/adapters/http"
	"github.com/code-corhuila/drp-notification-api/internal/adapters/memory"
	"github.com/code-corhuila/drp-notification-api/internal/adapters/security"
	"github.com/code-corhuila/drp-notification-api/internal/adapters/upstream"
	"github.com/code-corhuila/drp-notification-api/internal/app"
)

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8085"
	}
	service := os.Getenv("SERVICE_NAME")
	if service == "" {
		service = "notification-service"
	}
	jwksURL := os.Getenv("IDENTITY_JWKS_URL")
	if jwksURL == "" {
		jwksURL = "http://identity-api:8081/api/v1/auth/jwks"
	}

	tokens := security.NewVerifier(security.HTTPJWKS{
		URL: jwksURL,
		Client: &http.Client{
			Timeout: 3 * time.Second,
		},
	})
	httpClient := &http.Client{Timeout: 3 * time.Second}
	mux := httpadapter.NewMux(httpadapter.Deps{
		Service: service,
		Mailbox: app.Mailbox{
			Store: memory.NewMailbox(),
		},
		Reports: app.Reports{
			Store:        memory.NewReports(),
			Idempotency:  memory.NewReportKeys(),
			Payments:     catalog("drp-payment-api", os.Getenv("PAYMENT_API_URL"), "/api/v1/payments", httpClient),
			Reservations: catalog("drp-reservation-api", os.Getenv("RESERVATION_API_URL"), "/api/v1/reservations", httpClient),
			Spaces:       catalog("drp-space-api", os.Getenv("SPACE_API_URL"), "/api/v1/spaces", httpClient),
		},
		Tokens:        tokens,
		InternalToken: os.Getenv("INTERNAL_EVENT_TOKEN"),
	})

	log.Printf("drp-notification-api listening on %s (memory inbox + reports; POST /internal/events; E-20…E-22 aggregate HTTP; JWKS %s; Mongo later)", addr, jwksURL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func catalog(name, base, path string, client *http.Client) app.Catalog {
	if base == "" {
		return memory.StaticCatalog{Name: name}
	}
	return upstream.HTTPCatalog{Name: name, BaseURL: base, Path: path, Client: client}
}
