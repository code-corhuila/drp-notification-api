package memory

import (
	"context"
	"sync"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

type Reports struct {
	mu   sync.Mutex
	byID map[string]domain.Report
}

func NewReports() *Reports {
	return &Reports{byID: map[string]domain.Report{}}
}

func (s *Reports) Save(_ context.Context, r domain.Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[r.ID] = r
	return nil
}

func (s *Reports) FindByID(_ context.Context, id string) (domain.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.byID[id]
	if !ok {
		return domain.Report{}, app.ErrNotFound
	}
	return r, nil
}

func (s *Reports) ListByOwner(_ context.Context, requestedBy string) ([]domain.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.Report, 0)
	for _, r := range s.byID {
		if r.RequestedBy == requestedBy {
			out = append(out, r)
		}
	}
	return out, nil
}

type ReportKeys struct {
	mu   sync.Mutex
	body map[string]string
	rep  map[string]domain.Report
}

func NewReportKeys() *ReportKeys {
	return &ReportKeys{body: map[string]string{}, rep: map[string]domain.Report{}}
}

func key(actor, k string) string { return actor + "\x00" + k }

func (s *ReportKeys) Get(_ context.Context, actor, k string) (string, domain.Report, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := key(actor, k)
	body, ok := s.body[id]
	if !ok {
		return "", domain.Report{}, false
	}
	return body, s.rep[id], true
}

func (s *ReportKeys) Put(_ context.Context, actor, k, body string, r domain.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := key(actor, k)
	s.body[id] = body
	s.rep[id] = r
}

type StaticCatalog struct {
	Name  string
	Items []map[string]any
	Err   error
}

func (c StaticCatalog) Service() string { return c.Name }

func (c StaticCatalog) FetchAll(context.Context, string) ([]map[string]any, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	if c.Items == nil {
		return []map[string]any{}, nil
	}
	return c.Items, nil
}
