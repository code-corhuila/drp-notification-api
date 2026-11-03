package mongo

import (
	"context"
	"errors"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Mailbox persists deliveries in database notification. No DDL here (Anexo J).
type Mailbox struct {
	col *mongo.Collection
}

type doc struct {
	ID            string         `bson:"_id"`
	UserID        string         `bson:"userId"`
	SourceEventID string         `bson:"sourceEventId"`
	EventType     string         `bson:"eventType"`
	Channel       string         `bson:"channel"`
	State         string         `bson:"state"`
	Payload       map[string]any `bson:"payload,omitempty"`
	CreatedAt     time.Time      `bson:"createdAt"`
	UpdatedAt     time.Time      `bson:"updatedAt"`
}

// Store is the notification Mongo database (Anexo J). Collections come from Liquibase.
type Store struct {
	Notifications *Mailbox
	Reports       *Reports
	Keys          *ReportKeys
}

func Open(ctx context.Context, uri string) (*Store, error) {
	if uri == "" {
		return nil, errors.New("empty mongo uri")
	}
	cli, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := cli.Ping(ctx, nil); err != nil {
		_ = cli.Disconnect(ctx)
		return nil, err
	}
	db := cli.Database("notification")
	reports := db.Collection("reports")
	return &Store{
		Notifications: &Mailbox{col: db.Collection("notifications")},
		Reports:       &Reports{col: reports},
		Keys:          &ReportKeys{col: reports},
	}, nil
}

func (m *Mailbox) Save(ctx context.Context, n domain.Notification) error {
	_, err := m.col.InsertOne(ctx, toDoc(n))
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (m *Mailbox) FindByID(ctx context.Context, id string) (domain.Notification, error) {
	var row doc
	err := m.col.FindOne(ctx, bson.M{"_id": id}).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Notification{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Notification{}, err
	}
	return fromDoc(row), nil
}

func (m *Mailbox) FindBySource(ctx context.Context, sourceEventID string) (domain.Notification, bool) {
	var row doc
	err := m.col.FindOne(ctx, bson.M{"sourceEventId": sourceEventID}).Decode(&row)
	if err != nil {
		return domain.Notification{}, false
	}
	return fromDoc(row), true
}

func (m *Mailbox) ListByUser(ctx context.Context, userID string) ([]domain.Notification, error) {
	cur, err := m.col.Find(ctx, bson.M{"userId": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]domain.Notification, 0)
	for cur.Next(ctx) {
		var row doc
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		out = append(out, fromDoc(row))
	}
	return out, cur.Err()
}

func toDoc(n domain.Notification) doc {
	return doc{
		ID: n.ID, UserID: n.UserID, SourceEventID: n.SourceEventID,
		EventType: string(n.EventType), Channel: string(n.Channel), State: string(n.State),
		Payload: n.Payload, CreatedAt: n.CreatedAt.UTC(), UpdatedAt: n.UpdatedAt.UTC(),
	}
}

func fromDoc(row doc) domain.Notification {
	return domain.Notification{
		ID: row.ID, UserID: row.UserID, SourceEventID: row.SourceEventID,
		EventType: domain.EventType(row.EventType), Channel: domain.Channel(row.Channel),
		State: domain.DeliveryState(row.State), Payload: row.Payload,
		CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt.UTC(),
	}
}
