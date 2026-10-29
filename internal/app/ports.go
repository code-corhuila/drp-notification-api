package app

import (
	"context"

	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

type Principal struct {
	Subject string
	Role    string
}

func (p Principal) Admin() bool { return p.Role == "ADMIN" }

type TokenVerifier interface {
	Parse(ctx context.Context, raw string) (Principal, error)
}

type NotificationStore interface {
	Save(ctx context.Context, n domain.Notification) error
	FindByID(ctx context.Context, id string) (domain.Notification, error)
	FindBySource(ctx context.Context, sourceEventID string) (domain.Notification, bool)
	ListByUser(ctx context.Context, userID string) ([]domain.Notification, error)
}

type ReportStore interface {
	Save(ctx context.Context, r domain.Report) error
	FindByID(ctx context.Context, id string) (domain.Report, error)
	ListByOwner(ctx context.Context, requestedBy string) ([]domain.Report, error)
}

type ReportIdempotency interface {
	Get(ctx context.Context, actor, key string) (body string, r domain.Report, ok bool)
	Put(ctx context.Context, actor, key, body string, r domain.Report)
}

type Catalog interface {
	Service() string
	FetchAll(ctx context.Context, bearer string) ([]map[string]any, error)
}
