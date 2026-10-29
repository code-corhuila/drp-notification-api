package app

import "time"

type Health struct {
	Status    string
	Service   string
	Timestamp time.Time
}

func CheckHealth(service string, now time.Time) Health {
	if service == "" {
		service = "notification-service"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return Health{Status: "ok", Service: service, Timestamp: now.UTC()}
}
