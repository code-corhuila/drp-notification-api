package httpadapter

import (
	"net/http"

	"github.com/code-corhuila/drp-notification-api/internal/app"
)

type Deps struct {
	Service       string
	Mailbox       app.Mailbox
	Reports       app.Reports
	Tokens        app.TokenVerifier
	InternalToken string
}

func NewMux(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", HealthHandler(d.Service))
	mux.Handle("POST /internal/events", RequireInternalToken(d.InternalToken)(ConsumeEvent(d.Mailbox)))
	if d.Tokens != nil {
		auth := RequireBearer(d.Tokens)
		mux.Handle("GET /api/v1/notifications", auth(ListNotifications(d.Mailbox)))
		mux.Handle("GET /api/v1/notifications/{notificationId}", auth(GetNotification(d.Mailbox)))
		mux.Handle("GET /api/v1/reports", auth(ListReports(d.Reports)))
		mux.Handle("POST /api/v1/reports", auth(CreateReport(d.Reports)))
		mux.Handle("GET /api/v1/reports/{reportId}", auth(GetReport(d.Reports)))
		mux.Handle("GET /api/v1/reports/{reportId}/export", auth(ExportReport(d.Reports)))
	}
	mux.Handle("/", NotFoundHandler())
	return WithCorrelation(stripSpoofedIdentity(mux))
}
