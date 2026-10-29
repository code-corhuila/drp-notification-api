package httpadapter

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

var canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type notificationDTO struct {
	ID            string         `json:"id"`
	UserID        string         `json:"userId"`
	SourceEventID string         `json:"sourceEventId"`
	EventType     string         `json:"eventType"`
	Channel       string         `json:"channel"`
	State         string         `json:"state"`
	Payload       map[string]any `json:"payload,omitempty"`
	CreatedAt     string         `json:"createdAt"`
	UpdatedAt     string         `json:"updatedAt,omitempty"`
}

type listEnvelope struct {
	Data []notificationDTO `json:"data"`
	Meta metaDTO           `json:"meta"`
}

type metaDTO struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

func toDTO(n domain.Notification) notificationDTO {
	return notificationDTO{
		ID:            n.ID,
		UserID:        n.UserID,
		SourceEventID: n.SourceEventID,
		EventType:     string(n.EventType),
		Channel:       string(n.Channel),
		State:         string(n.State),
		Payload:       n.Payload,
		CreatedAt:     n.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     n.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func ListNotifications(box app.Mailbox) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, limit, berr := parsePageLimit(r)
		if berr != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", berr.Error(), berr.details)
			return
		}
		filter := app.ListFilter{Actor: principalFrom(r), Page: page, Limit: limit}
		if raw := strings.TrimSpace(r.URL.Query().Get("state")); raw != "" {
			st := domain.DeliveryState(raw)
			if st != domain.DeliveryPending && st != domain.DeliverySent && st != domain.DeliveryFailed {
				writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "El estado no es válido", []map[string]string{{"field": "state", "message": "Debe ser un estado de entrega conocido"}})
				return
			}
			filter.State = &st
		}
		got, err := box.List(r.Context(), filter)
		if errors.Is(err, app.ErrValidation) {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Parámetros de paginación inválidos", nil)
			return
		}
		if err != nil {
			writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
			return
		}
		data := make([]notificationDTO, 0, len(got.Items))
		for _, item := range got.Items {
			data = append(data, toDTO(item))
		}
		writeJSON(w, http.StatusOK, listEnvelope{
			Data: data,
			Meta: metaDTO{Page: got.Page, Limit: got.Limit, Total: got.Total, TotalPages: got.TotalPages},
		})
	}
}

func GetNotification(box app.Mailbox) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("notificationId")
		if !canonicalUUID.MatchString(id) {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "El identificador no es un UUID canónico", []map[string]string{{"field": "notificationId", "message": "Debe ser un UUID en minúsculas con guiones"}})
			return
		}
		got, err := box.Get(r.Context(), principalFrom(r), id)
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "La notificación no existe", nil)
			return
		}
		if err != nil {
			writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
			return
		}
		writeJSON(w, http.StatusOK, toDTO(got))
	}
}

type bindError struct {
	msg     string
	details []map[string]string
}

func (e *bindError) Error() string { return e.msg }

func parsePageLimit(r *http.Request) (int, int, *bindError) {
	page := 1
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, &bindError{msg: "El parámetro page debe ser un entero", details: []map[string]string{{"field": "page", "message": "Debe ser un entero"}}}
		}
		if n < 1 {
			return 0, 0, &bindError{msg: "El parámetro page debe ser mayor o igual a 1", details: []map[string]string{{"field": "page", "message": "Debe ser mayor o igual a 1"}}}
		}
		page = n
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, &bindError{msg: "El parámetro limit debe ser un entero", details: []map[string]string{{"field": "limit", "message": "Debe ser un entero"}}}
		}
		if n < 1 || n > 100 {
			return 0, 0, &bindError{msg: "El parámetro limit debe estar entre 1 y 100", details: []map[string]string{{"field": "limit", "message": "Debe estar entre 1 y 100"}}}
		}
		limit = n
	}
	return page, limit, nil
}
