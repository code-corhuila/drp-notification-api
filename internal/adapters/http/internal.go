package httpadapter

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/code-corhuila/drp-notification-api/internal/app"
)

const internalTokenHeader = "X-Internal-Token"

type inboundEvent struct {
	EventID       string          `json:"eventId"`
	EventType     string          `json:"eventType"`
	Payload       json.RawMessage `json:"payload"`
	CorrelationID string          `json:"correlationId"`
}

func RequireInternalToken(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				writeErr(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "El consumo interno no está configurado", nil)
				return
			}
			got := strings.TrimSpace(r.Header.Get(internalTokenHeader))
			if got == "" || got != expected {
				writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token de autenticación requerido", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ConsumeEvent(box app.Mailbox) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Datos de entrada inválidos", nil)
			return
		}
		var body inboundEvent
		if err := json.Unmarshal(raw, &body); err != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "JSON malformado", nil)
			return
		}
		payload := map[string]any{}
		if len(body.Payload) > 0 && string(body.Payload) != "null" {
			if err := json.Unmarshal(body.Payload, &payload); err != nil {
				writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "payload malformado", nil)
				return
			}
		}
		userID := stringField(payload, "userId")
		if userID == "" {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "El evento no trae userId", []map[string]string{{"field": "payload.userId", "message": "Obligatorio para crear la entrega"}})
			return
		}
		got, applied, err := box.Apply(r.Context(), app.InboundEvent{
			EventID:   strings.TrimSpace(body.EventID),
			EventType: strings.TrimSpace(body.EventType),
			UserID:    userID,
			Payload:   payload,
		})
		if errors.Is(err, app.ErrValidation) {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Evento de notificación inválido", nil)
			return
		}
		if err != nil {
			writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"applied":        applied,
			"notificationId": got.ID,
			"sourceEventId":  got.SourceEventID,
			"state":          string(got.State),
			"correlationId":  firstNonEmpty(body.CorrelationID, TraceID(r)),
		})
	}
}

func stringField(payload map[string]any, key string) string {
	v, ok := payload[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
