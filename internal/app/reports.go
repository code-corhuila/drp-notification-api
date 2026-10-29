package app

import (
	"context"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

type ReportListFilter struct {
	Actor Principal
	Kind  *domain.ReportKind
	State *domain.ReportState
	Page  int
	Limit int
}

type ReportPage struct {
	Items      []domain.Report
	Page       int
	Limit      int
	Total      int
	TotalPages int
}

type CreateReportCommand struct {
	Actor          Principal
	Kind           string
	PeriodStart    time.Time
	PeriodEnd      time.Time
	SpaceID        string
	IdempotencyKey string
	RawBody        string
	Bearer         string
}

type Reports struct {
	Store        ReportStore
	Idempotency  ReportIdempotency
	Payments     Catalog
	Reservations Catalog
	Spaces       Catalog
	Now          func() time.Time
	NewID        func() string
}

func (s Reports) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (s Reports) newID() string {
	return Mailbox{NewID: s.NewID}.newID()
}

func (s Reports) requireAdmin(actor Principal) error {
	if actor.Subject == "" {
		return ErrUnauthorized
	}
	if !actor.Admin() {
		return ErrForbidden
	}
	return nil
}

func (s Reports) List(ctx context.Context, f ReportListFilter) (ReportPage, error) {
	if err := s.requireAdmin(f.Actor); err != nil {
		return ReportPage{}, err
	}
	if f.Page < 1 || f.Limit < 1 || f.Limit > 100 {
		return ReportPage{}, ErrValidation
	}
	all, err := s.Store.ListByOwner(ctx, f.Actor.Subject)
	if err != nil {
		return ReportPage{}, err
	}
	filtered := make([]domain.Report, 0, len(all))
	for _, r := range all {
		if f.Kind != nil && r.Kind != *f.Kind {
			continue
		}
		if f.State != nil && r.State != *f.State {
			continue
		}
		filtered = append(filtered, r)
	}
	sortReportsNewest(filtered)
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
		return ReportPage{Items: []domain.Report{}, Page: f.Page, Limit: f.Limit, Total: total, TotalPages: totalPages}, nil
	}
	end := start + f.Limit
	if end > total {
		end = total
	}
	return ReportPage{Items: filtered[start:end], Page: f.Page, Limit: f.Limit, Total: total, TotalPages: totalPages}, nil
}

func (s Reports) Get(ctx context.Context, actor Principal, id string) (domain.Report, error) {
	if err := s.requireAdmin(actor); err != nil {
		return domain.Report{}, err
	}
	r, err := s.Store.FindByID(ctx, id)
	if err != nil || r.RequestedBy != actor.Subject {
		return domain.Report{}, ErrNotFound
	}
	return r, nil
}

func (s Reports) Create(ctx context.Context, cmd CreateReportCommand) (domain.Report, bool, error) {
	if err := s.requireAdmin(cmd.Actor); err != nil {
		return domain.Report{}, false, err
	}
	if len(cmd.IdempotencyKey) < 8 || len(cmd.IdempotencyKey) > 128 {
		return domain.Report{}, false, ErrValidation
	}
	if cmd.PeriodEnd.Sub(cmd.PeriodStart) > 366*24*time.Hour {
		return domain.Report{}, false, ErrBusiness
	}
	if cmd.Kind == string(domain.KindSpaceOccupancy) && cmd.SpaceID == "" {
		return domain.Report{}, false, ErrBusiness
	}
	if cmd.Kind != string(domain.KindSpaceOccupancy) && cmd.SpaceID != "" {
		return domain.Report{}, false, ErrBusiness
	}
	if existing, r, ok := s.Idempotency.Get(ctx, cmd.Actor.Subject, cmd.IdempotencyKey); ok {
		if existing != cmd.RawBody {
			return domain.Report{}, false, ErrConflict
		}
		return r, true, nil
	}

	draft, err := domain.NewReport(s.newID(), cmd.Kind, cmd.Actor.Subject, cmd.SpaceID, cmd.IdempotencyKey, cmd.PeriodStart, cmd.PeriodEnd, s.now())
	if err != nil {
		return domain.Report{}, false, ErrValidation
	}

	payments, paySrc, err := s.read(ctx, s.Payments, cmd.Bearer)
	if err != nil {
		return domain.Report{}, false, err
	}
	reservations, resSrc, err := s.read(ctx, s.Reservations, cmd.Bearer)
	if err != nil {
		return domain.Report{}, false, err
	}
	spaces, spSrc, err := s.read(ctx, s.Spaces, cmd.Bearer)
	if err != nil {
		return domain.Report{}, false, err
	}
	sources := []domain.ReportSource{paySrc, resSrc, spSrc}
	rows := snapshotRows(draft, payments, reservations, spaces)
	ready, err := draft.Ready(s.now(), sources, rows)
	if err != nil {
		return domain.Report{}, false, err
	}
	if err := s.Store.Save(ctx, ready); err != nil {
		return domain.Report{}, false, err
	}
	s.Idempotency.Put(ctx, cmd.Actor.Subject, cmd.IdempotencyKey, cmd.RawBody, ready)
	return ready, false, nil
}

func (s Reports) read(ctx context.Context, catalog Catalog, bearer string) ([]map[string]any, domain.ReportSource, error) {
	if catalog == nil {
		return nil, domain.ReportSource{}, UpstreamError{Service: "upstream", Status: 503}
	}
	items, err := catalog.FetchAll(ctx, bearer)
	if err != nil {
		up, ok := err.(UpstreamError)
		if !ok {
			up = UpstreamError{Service: catalog.Service(), Status: 503}
		}
		if up.Service == "" {
			up.Service = catalog.Service()
		}
		return nil, domain.ReportSource{}, up
	}
	return items, domain.ReportSource{Service: catalog.Service(), Status: 200}, nil
}

func snapshotRows(r domain.Report, payments, reservations, spaces []map[string]any) []map[string]any {
	switch r.Kind {
	case domain.KindPaymentsSummary:
		count := 0
		amount := 0
		for _, p := range payments {
			if !inPeriod(timeField(p, "createdAt"), r.PeriodStart, r.PeriodEnd) {
				continue
			}
			count++
			amount += intField(p, "amountCents")
		}
		return []map[string]any{{"count": count, "amountCents": amount}}
	case domain.KindReservationsSummary:
		var pending, confirmed, cancelled int
		for _, item := range reservations {
			if !inPeriod(timeField(item, "startAt"), r.PeriodStart, r.PeriodEnd) {
				continue
			}
			switch strField(item, "state") {
			case "CONFIRMED":
				confirmed++
			case "CANCELLED":
				cancelled++
			default:
				pending++
			}
		}
		return []map[string]any{{
			"count":          pending + confirmed + cancelled,
			"paymentPending": pending,
			"confirmed":      confirmed,
			"cancelled":      cancelled,
			"spaces":         len(spaces),
		}}
	default:
		count, confirmed := 0, 0
		for _, item := range reservations {
			if strField(item, "spaceId") != r.SpaceID {
				continue
			}
			if !inPeriod(timeField(item, "startAt"), r.PeriodStart, r.PeriodEnd) {
				continue
			}
			count++
			if strField(item, "state") == "CONFIRMED" {
				confirmed++
			}
		}
		return []map[string]any{{"spaceId": r.SpaceID, "reservationCount": count, "confirmedCount": confirmed, "spaces": len(spaces)}}
	}
}

func inPeriod(t time.Time, start, end time.Time) bool {
	if t.IsZero() {
		return true
	}
	return !t.Before(start) && t.Before(end)
}

func timeField(m map[string]any, key string) time.Time {
	raw, ok := m[key].(string)
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func strField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func sortReportsNewest(items []domain.Report) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].CreatedAt.After(items[i].CreatedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}
