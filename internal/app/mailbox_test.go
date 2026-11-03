package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/adapters/memory"
	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

const (
	member = "44444444-4444-4444-4444-444444444444"
	admin  = "66666666-6666-4666-8666-666666666666"
	event1 = "55555555-5555-4555-8555-555555555555"
)

func mailbox(t *testing.T) app.Mailbox {
	t.Helper()
	fixed := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	seq := 0
	return app.Mailbox{
		Store: memory.NewMailbox(),
		Now:   func() time.Time { return fixed },
		NewID: func() string {
			seq++
			return "aaaaaaaa-aaaa-4aaa-8aaa-00000000000" + string(rune('0'+seq))
		},
	}
}

func TestApplyWritesSentInAppAndDedupsSource(t *testing.T) {
	m := mailbox(t)
	ctx := context.Background()
	got, applied, err := m.Apply(ctx, app.InboundEvent{
		EventID:   event1,
		EventType: string(domain.EventReservationConfirmed),
		UserID:    member,
		Payload:   map[string]any{"reservationId": "r1", "userId": member},
	})
	if err != nil || !applied || got.State != domain.DeliverySent || got.Channel != domain.ChannelInApp {
		t.Fatalf("first apply %+v applied=%v err=%v", got, applied, err)
	}
	again, applied, err := m.Apply(ctx, app.InboundEvent{
		EventID:   event1,
		EventType: string(domain.EventReservationConfirmed),
		UserID:    member,
	})
	if err != nil || applied || again.ID != got.ID {
		t.Fatalf("dup %+v applied=%v err=%v", again, applied, err)
	}
}

func TestListAndGetAreOwnInboxEvenForAdmin(t *testing.T) {
	m := mailbox(t)
	ctx := context.Background()
	_, _, err := m.Apply(ctx, app.InboundEvent{EventID: event1, EventType: string(domain.EventPaymentConfirmed), UserID: member})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.Apply(ctx, app.InboundEvent{EventID: "77777777-7777-4777-8777-777777777777", EventType: string(domain.EventPaymentConfirmed), UserID: admin})
	if err != nil {
		t.Fatal(err)
	}

	page, err := m.List(ctx, app.ListFilter{Actor: app.Principal{Subject: admin, Role: "ADMIN"}, Page: 1, Limit: 20})
	if err != nil || page.Total != 1 || page.Items[0].UserID != admin {
		t.Fatalf("admin list %+v err=%v", page, err)
	}

	if _, err := m.Get(ctx, app.Principal{Subject: admin, Role: "ADMIN"}, page.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	owned, err := m.List(ctx, app.ListFilter{Actor: app.Principal{Subject: member, Role: "USER"}, Page: 1, Limit: 20})
	if err != nil || owned.Total != 1 {
		t.Fatalf("member list %+v err=%v", owned, err)
	}
	if _, err := m.Get(ctx, app.Principal{Subject: admin, Role: "ADMIN"}, owned.Items[0].ID); err != app.ErrNotFound {
		t.Fatalf("admin must not read foreign inbox: %v", err)
	}
}

func TestUnknownEventTypeIsValidation(t *testing.T) {
	m := mailbox(t)
	_, _, err := m.Apply(context.Background(), app.InboundEvent{
		EventID:   event1,
		EventType: "SpaceCreated",
		UserID:    member,
	})
	if err != app.ErrValidation {
		t.Fatalf("err %v", err)
	}
}
