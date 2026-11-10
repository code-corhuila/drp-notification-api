package mongo

import (
	"context"
	"errors"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Reports persists snapshots. No DDL (Anexo J / ADR-007).
type Reports struct {
	col *mongo.Collection
}

type reportSourceDoc struct {
	Service string `bson:"service"`
	Status  int    `bson:"status"`
}

type reportDoc struct {
	ID             string            `bson:"_id"`
	Kind           string            `bson:"kind"`
	State          string            `bson:"state"`
	RequestedBy    string            `bson:"requestedBy"`
	PeriodStart    string            `bson:"periodStart"`
	PeriodEnd      string            `bson:"periodEnd"`
	SpaceID        *string           `bson:"spaceId"`
	IdempotencyKey string            `bson:"idempotencyKey,omitempty"`
	RequestBody    string            `bson:"requestBody,omitempty"`
	Sources        []reportSourceDoc `bson:"sources,omitempty"`
	Rows           []map[string]any  `bson:"rows,omitempty"`
	GeneratedAt    *time.Time        `bson:"generatedAt,omitempty"`
	CreatedAt      time.Time         `bson:"createdAt"`
}

func (s *Reports) Save(ctx context.Context, r domain.Report) error {
	_, err := s.col.InsertOne(ctx, toReportDoc(r, ""))
	return err
}

func (s *Reports) FindByID(ctx context.Context, id string) (domain.Report, error) {
	var row reportDoc
	err := s.col.FindOne(ctx, bson.M{"_id": id}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Report{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Report{}, err
	}
	return fromReportDoc(row)
}

func (s *Reports) ListByOwner(ctx context.Context, requestedBy string) ([]domain.Report, error) {
	cur, err := s.col.Find(ctx, bson.M{"requestedBy": requestedBy})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]domain.Report, 0)
	for cur.Next(ctx) {
		var row reportDoc
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		r, err := fromReportDoc(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, cur.Err()
}

// ReportKeys uses the same reports documents (unique sparse idempotencyKey).
type ReportKeys struct {
	col *mongo.Collection
}

func (k *ReportKeys) Get(ctx context.Context, actor, key string) (string, domain.Report, bool) {
	var row reportDoc
	err := k.col.FindOne(ctx, bson.M{"requestedBy": actor, "idempotencyKey": key}).Decode(&row)
	if err != nil {
		return "", domain.Report{}, false
	}
	r, err := fromReportDoc(row)
	if err != nil {
		return "", domain.Report{}, false
	}
	return row.RequestBody, r, true
}

func (k *ReportKeys) Put(ctx context.Context, actor, key, body string, r domain.Report) {
	_, _ = k.col.UpdateOne(ctx, bson.M{"_id": r.ID, "requestedBy": actor, "idempotencyKey": key}, bson.M{
		"$set": bson.M{"requestBody": body},
	})
}

func toReportDoc(r domain.Report, body string) reportDoc {
	var space *string
	if r.SpaceID != "" {
		s := r.SpaceID
		space = &s
	}
	sources := make([]reportSourceDoc, 0, len(r.Sources))
	for _, src := range r.Sources {
		sources = append(sources, reportSourceDoc{Service: src.Service, Status: src.Status})
	}
	return reportDoc{
		ID: r.ID, Kind: string(r.Kind), State: string(r.State), RequestedBy: r.RequestedBy,
		PeriodStart: r.PeriodStart.UTC().Format(time.RFC3339), PeriodEnd: r.PeriodEnd.UTC().Format(time.RFC3339),
		SpaceID: space, IdempotencyKey: r.IdempotencyKey, RequestBody: body,
		Sources: sources, Rows: r.Rows, GeneratedAt: r.GeneratedAt, CreatedAt: r.CreatedAt.UTC(),
	}
}

func fromReportDoc(row reportDoc) (domain.Report, error) {
	start, err := time.Parse(time.RFC3339, row.PeriodStart)
	if err != nil {
		return domain.Report{}, err
	}
	end, err := time.Parse(time.RFC3339, row.PeriodEnd)
	if err != nil {
		return domain.Report{}, err
	}
	space := ""
	if row.SpaceID != nil {
		space = *row.SpaceID
	}
	sources := make([]domain.ReportSource, 0, len(row.Sources))
	for _, src := range row.Sources {
		sources = append(sources, domain.ReportSource{Service: src.Service, Status: src.Status})
	}
	return domain.Report{
		ID: row.ID, Kind: domain.ReportKind(row.Kind), State: domain.ReportState(row.State),
		RequestedBy: row.RequestedBy, PeriodStart: start.UTC(), PeriodEnd: end.UTC(),
		SpaceID: space, IdempotencyKey: row.IdempotencyKey, Sources: sources, Rows: row.Rows,
		GeneratedAt: row.GeneratedAt, CreatedAt: row.CreatedAt.UTC(),
	}, nil
}
