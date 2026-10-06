package domain

import "time"

type Channel string

const (
	ChannelEmail Channel = "EMAIL"
	ChannelInApp Channel = "IN_APP"
)

type DeliveryState string

const (
	DeliveryPending DeliveryState = "PENDING"
	DeliverySent    DeliveryState = "SENT"
	DeliveryFailed  DeliveryState = "FAILED"
)

type EventType string

const (
	EventUserRegistered         EventType = "user.registered"
	EventReservationCreated     EventType = "ReservationCreated"
	EventPaymentConfirmed       EventType = "PaymentConfirmed"
	EventPaymentFailed          EventType = "PaymentFailed"
	EventReservationConfirmed   EventType = "ReservationConfirmed"
	EventReservationCancelled   EventType = "ReservationCancelled"
)

func parseChannel(v string) (Channel, error) {
	switch Channel(v) {
	case ChannelEmail, ChannelInApp:
		return Channel(v), nil
	default:
		return "", ErrInvalidInput
	}
}

func parseEventType(v string) (EventType, error) {
	switch EventType(v) {
	case EventUserRegistered, EventReservationCreated, EventPaymentConfirmed,
		EventPaymentFailed, EventReservationConfirmed, EventReservationCancelled:
		return EventType(v), nil
	default:
		return "", ErrInvalidInput
	}
}

// Notification is the delivery aggregate. Unique sourceEventId is BR-006 / INV-NOT-002.
// A FAILED row must not ask reservation to leave CONFIRMED.
type Notification struct {
	ID            string
	UserID        string
	SourceEventID string
	EventType     EventType
	Channel       Channel
	State         DeliveryState
	Payload       map[string]any
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func NewNotification(id, userID, sourceEventID, eventType, channel string, now time.Time) (Notification, error) {
	et, err := parseEventType(eventType)
	if err != nil {
		return Notification{}, err
	}
	ch, err := parseChannel(channel)
	if err != nil {
		return Notification{}, err
	}
	if id == "" || userID == "" || sourceEventID == "" {
		return Notification{}, ErrInvalidInput
	}
	return Notification{
		ID: id, UserID: userID, SourceEventID: sourceEventID,
		EventType: et, Channel: ch, State: DeliveryPending, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (n Notification) MarkSent(now time.Time) (Notification, error) {
	if n.State != DeliveryPending {
		return Notification{}, ErrInvalidTransition
	}
	n.State = DeliverySent
	n.UpdatedAt = now
	return n, nil
}

func (n Notification) MarkFailed(now time.Time) (Notification, error) {
	if n.State != DeliveryPending {
		return Notification{}, ErrInvalidTransition
	}
	n.State = DeliveryFailed
	n.UpdatedAt = now
	return n, nil
}

func DuplicateSource(existing []Notification, sourceEventID string) bool {
	for _, n := range existing {
		if n.SourceEventID == sourceEventID {
			return true
		}
	}
	return false
}
