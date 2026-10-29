package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/adapters/memory"
	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

func reportsSvc(t *testing.T, payments, reservations, spaces app.Catalog) app.Reports {
	t.Helper()
	fixed := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	seq := 0
	return app.Reports{
		Store:        memory.NewReports(),
		Idempotency:  memory.NewReportKeys(),
		Payments:     payments,
		Reservations: reservations,
		Spaces:       spaces,
		Now:          func() time.Time { return fixed },
		NewID: func() string {
			seq++
			return "bbbbbbbb-bbbb-4bbb-8bbb-00000000000" + string(rune('0'+seq))
		},
	}
}

func TestUserCannotCreateReport(t *testing.T) {
	svc := reportsSvc(t, memory.StaticCatalog{Name: "drp-payment-api"}, memory.StaticCatalog{Name: "drp-reservation-api"}, memory.StaticCatalog{Name: "drp-space-api"})
	_, _, err := svc.Create(context.Background(), app.CreateReportCommand{
		Actor:          app.Principal{Subject: member, Role: "USER"},
		Kind:           "PAYMENTS_SUMMARY",
		PeriodStart:    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		IdempotencyKey: "idem-key-01",
		RawBody:        `{"kind":"PAYMENTS_SUMMARY"}`,
	})
	if err != app.ErrForbidden {
		t.Fatalf("err %v", err)
	}
}

func TestCreateReadySnapshotAndReplay(t *testing.T) {
	pay := memory.StaticCatalog{Name: "drp-payment-api", Items: []map[string]any{
		{"amountCents": 5000000, "createdAt": "2026-09-15T00:00:00Z"},
	}}
	svc := reportsSvc(t, pay, memory.StaticCatalog{Name: "drp-reservation-api"}, memory.StaticCatalog{Name: "drp-space-api"})
	cmd := app.CreateReportCommand{
		Actor:          app.Principal{Subject: admin, Role: "ADMIN"},
		Kind:           "PAYMENTS_SUMMARY",
		PeriodStart:    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		IdempotencyKey: "idem-key-01",
		RawBody:        `{"kind":"PAYMENTS_SUMMARY"}`,
		Bearer:         "token",
	}
	got, replay, err := svc.Create(context.Background(), cmd)
	if err != nil || replay || got.State != domain.ReportReady || got.GeneratedAt == nil {
		t.Fatalf("create %+v replay=%v err=%v", got, replay, err)
	}
	if got.Rows[0]["amountCents"] != 5000000 {
		t.Fatalf("rows %#v", got.Rows)
	}
	again, replay, err := svc.Create(context.Background(), cmd)
	if err != nil || !replay || again.ID != got.ID {
		t.Fatalf("replay %+v replay=%v err=%v", again, replay, err)
	}
}

func TestUpstreamFailureStoresNothing(t *testing.T) {
	svc := reportsSvc(t,
		memory.StaticCatalog{Name: "drp-payment-api", Err: app.UpstreamError{Service: "drp-payment-api", Status: 503}},
		memory.StaticCatalog{Name: "drp-reservation-api"},
		memory.StaticCatalog{Name: "drp-space-api"},
	)
	_, _, err := svc.Create(context.Background(), app.CreateReportCommand{
		Actor:          app.Principal{Subject: admin, Role: "ADMIN"},
		Kind:           "PAYMENTS_SUMMARY",
		PeriodStart:    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		IdempotencyKey: "idem-key-02",
		RawBody:        `{"kind":"PAYMENTS_SUMMARY"}`,
		Bearer:         "token",
	})
	up, ok := err.(app.UpstreamError)
	if !ok || up.Service != "drp-payment-api" {
		t.Fatalf("err %v", err)
	}
	page, err := svc.List(context.Background(), app.ReportListFilter{Actor: app.Principal{Subject: admin, Role: "ADMIN"}, Page: 1, Limit: 20})
	if err != nil || page.Total != 0 {
		t.Fatalf("must not store: %+v err=%v", page, err)
	}
}

func TestOccupancyRequiresSpaceID(t *testing.T) {
	svc := reportsSvc(t, memory.StaticCatalog{Name: "drp-payment-api"}, memory.StaticCatalog{Name: "drp-reservation-api"}, memory.StaticCatalog{Name: "drp-space-api"})
	_, _, err := svc.Create(context.Background(), app.CreateReportCommand{
		Actor:          app.Principal{Subject: admin, Role: "ADMIN"},
		Kind:           "SPACE_OCCUPANCY",
		PeriodStart:    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:      time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		IdempotencyKey: "idem-key-03",
		RawBody:        `{}`,
	})
	if err != app.ErrBusiness {
		t.Fatalf("err %v", err)
	}
}
