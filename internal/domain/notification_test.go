package domain

import (
	"testing"
	"time"
)

func TestNotificationIdempotency(t *testing.T) {
	now := time.Now().UTC()
	n, err := NewNotification("n1", "u1", "evt-1", "ReservationConfirmed", "IN_APP", now)
	if err != nil {
		t.Fatal(err)
	}
	if !DuplicateSource([]Notification{n}, "evt-1") {
		t.Fatal("expected duplicate")
	}
	failed, err := n.MarkFailed(now)
	if err != nil || failed.State != DeliveryFailed {
		t.Fatal(err)
	}
}

func TestReportRules(t *testing.T) {
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	start := now.Add(-24 * time.Hour)
	end := now
	r, err := NewReport("r1", "PAYMENTS_SUMMARY", "u1", "", "idem-1", start, end, now)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := r.Ready(now, []ReportSource{{Service: "drp-payment-api", Status: 200}}, []map[string]any{{"amountCents": 5000000}})
	if err != nil || ready.GeneratedAt == nil {
		t.Fatal(err)
	}
	if _, err := ready.Ready(now, nil, nil); err == nil {
		t.Fatal("immutable after generatedAt")
	}
	if _, err := NewReport("r2", "SPACE_OCCUPANCY", "u1", "", "k", start, end, now); err == nil {
		t.Fatal("SPACE_OCCUPANCY needs spaceId")
	}
}
