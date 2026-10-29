package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

type InboundEvent struct {
	EventID   string
	EventType string
	UserID    string
	Payload   map[string]any
}

type ListFilter struct {
	Actor Principal
	State *domain.DeliveryState
	Page  int
	Limit int
}

type NotificationPage struct {
	Items      []domain.Notification
	Page       int
	Limit      int
	Total      int
	TotalPages int
}

type Mailbox struct {
	Store NotificationStore
	Now   func() time.Time
	NewID func() string
}

func (m Mailbox) now() time.Time {
	if m.Now == nil {
		return time.Now().UTC()
	}
	return m.Now().UTC()
}

func (m Mailbox) newID() string {
	if m.NewID != nil {
		return m.NewID()
	}
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	h := hex.EncodeToString(buf[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func (m Mailbox) Apply(ctx context.Context, ev InboundEvent) (domain.Notification, bool, error) {
	if ev.EventID == "" || ev.UserID == "" || ev.EventType == "" {
		return domain.Notification{}, false, ErrValidation
	}
	if existing, ok := m.Store.FindBySource(ctx, ev.EventID); ok {
		return existing, false, nil
	}
	n, err := domain.NewNotification(m.newID(), ev.UserID, ev.EventID, ev.EventType, string(domain.ChannelInApp), m.now())
	if err != nil {
		return domain.Notification{}, false, ErrValidation
	}
	n.Payload = ev.Payload
	sent, err := n.MarkSent(m.now())
	if err != nil {
		return domain.Notification{}, false, err
	}
	if err := m.Store.Save(ctx, sent); err != nil {
		return domain.Notification{}, false, err
	}
	return sent, true, nil
}

func (m Mailbox) List(ctx context.Context, f ListFilter) (NotificationPage, error) {
	if f.Page < 1 || f.Limit < 1 || f.Limit > 100 {
		return NotificationPage{}, ErrValidation
	}
	if f.Actor.Subject == "" {
		return NotificationPage{}, ErrUnauthorized
	}
	all, err := m.Store.ListByUser(ctx, f.Actor.Subject)
	if err != nil {
		return NotificationPage{}, err
	}
	filtered := make([]domain.Notification, 0, len(all))
	for _, n := range all {
		if f.State != nil && n.State != *f.State {
			continue
		}
		filtered = append(filtered, n)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	total := len(filtered)
	totalPages := total / f.Limit
	if total%f.Limit != 0 {
		totalPages++
	}
	if totalPages == 0 {
		totalPages = 1
	}
	start := (f.Page - 1) * f.Limit
	if start >= total {
		return NotificationPage{Items: []domain.Notification{}, Page: f.Page, Limit: f.Limit, Total: total, TotalPages: totalPages}, nil
	}
	end := start + f.Limit
	if end > total {
		end = total
	}
	return NotificationPage{Items: filtered[start:end], Page: f.Page, Limit: f.Limit, Total: total, TotalPages: totalPages}, nil
}

func (m Mailbox) Get(ctx context.Context, actor Principal, id string) (domain.Notification, error) {
	if actor.Subject == "" {
		return domain.Notification{}, ErrUnauthorized
	}
	n, err := m.Store.FindByID(ctx, id)
	if err != nil {
		return domain.Notification{}, ErrNotFound
	}
	if n.UserID != actor.Subject {
		return domain.Notification{}, ErrNotFound
	}
	return n, nil
}
