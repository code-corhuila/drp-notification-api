package domain

import "time"

type ReportKind string

const (
	KindPaymentsSummary     ReportKind = "PAYMENTS_SUMMARY"
	KindReservationsSummary ReportKind = "RESERVATIONS_SUMMARY"
	KindSpaceOccupancy      ReportKind = "SPACE_OCCUPANCY"
)

type ReportState string

const (
	ReportPending ReportState = "PENDING"
	ReportReady   ReportState = "READY"
	ReportFailed  ReportState = "FAILED"
)

type ReportSource struct {
	Service string
	Status  int
}

// Report is an immutable snapshot after GeneratedAt (ADR-007). Built via HTTP, never tables.
type Report struct {
	ID            string
	Kind          ReportKind
	State         ReportState
	RequestedBy   string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	SpaceID       string
	IdempotencyKey string
	Sources       []ReportSource
	Rows          []map[string]any
	GeneratedAt   *time.Time
	CreatedAt     time.Time
}

func NewReport(id, kind, requestedBy, spaceID, idempotencyKey string, start, end, now time.Time) (Report, error) {
	k := ReportKind(kind)
	switch k {
	case KindPaymentsSummary, KindReservationsSummary, KindSpaceOccupancy:
	default:
		return Report{}, ErrInvalidInput
	}
	if id == "" || requestedBy == "" || start.IsZero() || !end.After(start) {
		return Report{}, ErrInvalidInput
	}
	if end.Sub(start) > 366*24*time.Hour {
		return Report{}, ErrInvalidInput
	}
	if k == KindSpaceOccupancy && spaceID == "" {
		return Report{}, ErrInvalidInput
	}
	if k != KindSpaceOccupancy && spaceID != "" {
		return Report{}, ErrInvalidInput
	}
	return Report{
		ID: id, Kind: k, State: ReportPending, RequestedBy: requestedBy,
		PeriodStart: start.UTC(), PeriodEnd: end.UTC(), SpaceID: spaceID,
		IdempotencyKey: idempotencyKey, CreatedAt: now,
	}, nil
}

func (r Report) Ready(now time.Time, sources []ReportSource, rows []map[string]any) (Report, error) {
	if r.GeneratedAt != nil || r.State != ReportPending {
		return Report{}, ErrImmutableReport
	}
	t := now
	r.State = ReportReady
	r.Sources = sources
	r.Rows = rows
	r.GeneratedAt = &t
	return r, nil
}

func (r Report) Fail(now time.Time, sources []ReportSource) (Report, error) {
	if r.GeneratedAt != nil || r.State != ReportPending {
		return Report{}, ErrImmutableReport
	}
	r.State = ReportFailed
	r.Sources = sources
	return r, nil
}
