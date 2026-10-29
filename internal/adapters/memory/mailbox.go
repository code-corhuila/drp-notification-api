package memory

import (
	"context"
	"sync"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

type Mailbox struct {
	mu    sync.Mutex
	byID  map[string]domain.Notification
	bySrc map[string]string
}

func NewMailbox() *Mailbox {
	return &Mailbox{byID: map[string]domain.Notification{}, bySrc: map[string]string{}}
}

func (m *Mailbox) Save(_ context.Context, n domain.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byID[n.ID] = n
	m.bySrc[n.SourceEventID] = n.ID
	return nil
}

func (m *Mailbox) FindByID(_ context.Context, id string) (domain.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.byID[id]
	if !ok {
		return domain.Notification{}, app.ErrNotFound
	}
	return n, nil
}

func (m *Mailbox) FindBySource(_ context.Context, sourceEventID string) (domain.Notification, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.bySrc[sourceEventID]
	if !ok {
		return domain.Notification{}, false
	}
	n, ok := m.byID[id]
	return n, ok
}

func (m *Mailbox) ListByUser(_ context.Context, userID string) ([]domain.Notification, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Notification, 0)
	for _, n := range m.byID {
		if n.UserID == userID {
			out = append(out, n)
		}
	}
	return out, nil
}
