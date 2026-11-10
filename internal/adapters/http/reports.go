package httpadapter

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/app"
	"github.com/code-corhuila/drp-notification-api/internal/domain"
)

type reportDTO struct {
	ID          string            `json:"id"`
	Kind        string            `json:"kind"`
	State       string            `json:"state"`
	RequestedBy string            `json:"requestedBy"`
	PeriodStart string            `json:"periodStart"`
	PeriodEnd   string            `json:"periodEnd"`
	SpaceID     string            `json:"spaceId,omitempty"`
	Sources     []reportSourceDTO `json:"sources,omitempty"`
	Rows        []map[string]any  `json:"rows,omitempty"`
	GeneratedAt string            `json:"generatedAt,omitempty"`
	CreatedAt   string            `json:"createdAt"`
}

type reportSourceDTO struct {
	Service string `json:"service"`
	Status  int    `json:"status"`
}

type reportListEnvelope struct {
	Data []reportDTO `json:"data"`
	Meta metaDTO     `json:"meta"`
}

func toReportDTO(r domain.Report) reportDTO {
	dto := reportDTO{
		ID:          r.ID,
		Kind:        string(r.Kind),
		State:       string(r.State),
		RequestedBy: r.RequestedBy,
		PeriodStart: r.PeriodStart.UTC().Format(time.RFC3339),
		PeriodEnd:   r.PeriodEnd.UTC().Format(time.RFC3339),
		SpaceID:     r.SpaceID,
		Rows:        r.Rows,
		CreatedAt:   r.CreatedAt.UTC().Format(time.RFC3339),
	}
	if r.GeneratedAt != nil {
		dto.GeneratedAt = r.GeneratedAt.UTC().Format(time.RFC3339)
	}
	for _, src := range r.Sources {
		dto.Sources = append(dto.Sources, reportSourceDTO{Service: src.Service, Status: src.Status})
	}
	return dto
}

func ListReports(svc app.Reports) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r); err != nil {
			writeReportAuth(w, r, err)
			return
		}
		page, limit, berr := parsePageLimit(r)
		if berr != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", berr.Error(), berr.details)
			return
		}
		filter := app.ReportListFilter{Actor: principalFrom(r), Page: page, Limit: limit}
		if raw := strings.TrimSpace(r.URL.Query().Get("kind")); raw != "" {
			k := domain.ReportKind(raw)
			if k != domain.KindPaymentsSummary && k != domain.KindReservationsSummary && k != domain.KindSpaceOccupancy {
				writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "kind no es válido", []map[string]string{{"field": "kind", "message": "Debe ser un kind de informe conocido"}})
				return
			}
			filter.Kind = &k
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("state")); raw != "" {
			st := domain.ReportState(raw)
			if st != domain.ReportPending && st != domain.ReportReady && st != domain.ReportFailed {
				writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "El estado no es válido", []map[string]string{{"field": "state", "message": "Debe ser un estado de informe conocido"}})
				return
			}
			filter.State = &st
		}
		got, err := svc.List(r.Context(), filter)
		if err != nil {
			writeReportErr(w, r, err)
			return
		}
		data := make([]reportDTO, 0, len(got.Items))
		for _, item := range got.Items {
			data = append(data, toReportDTO(item))
		}
		writeJSON(w, http.StatusOK, reportListEnvelope{
			Data: data,
			Meta: metaDTO{Page: got.Page, Limit: got.Limit, Total: got.Total, TotalPages: got.TotalPages},
		})
	}
}

func GetReport(svc app.Reports) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r); err != nil {
			writeReportAuth(w, r, err)
			return
		}
		id := r.PathValue("reportId")
		if !canonicalUUID.MatchString(id) {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "El identificador no es un UUID canónico", []map[string]string{{"field": "reportId", "message": "Debe ser un UUID en minúsculas con guiones"}})
			return
		}
		got, err := svc.Get(r.Context(), principalFrom(r), id)
		if err != nil {
			writeReportErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, toReportDTO(got))
	}
}

type createReportBody struct {
	Kind        string `json:"kind"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	SpaceID     string `json:"spaceId"`
}

func CreateReport(svc app.Reports) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r); err != nil {
			writeReportAuth(w, r, err)
			return
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key es obligatoria", []map[string]string{{"field": "Idempotency-Key", "message": "Obligatoria, 8 a 128 caracteres"}})
			return
		}
		if len(key) < 8 || len(key) > 128 {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Idempotency-Key inválida", []map[string]string{{"field": "Idempotency-Key", "message": "Debe tener entre 8 y 128 caracteres"}})
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Datos de entrada inválidos", nil)
			return
		}
		var body createReportBody
		if err := json.Unmarshal(raw, &body); err != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "JSON malformado", nil)
			return
		}
		start, berr := parseInstant("periodStart", body.PeriodStart)
		if berr != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", berr.Error(), berr.details)
			return
		}
		end, berr := parseInstant("periodEnd", body.PeriodEnd)
		if berr != nil {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", berr.Error(), berr.details)
			return
		}
		got, replay, err := svc.Create(r.Context(), app.CreateReportCommand{
			Actor:          principalFrom(r),
			Kind:           body.Kind,
			PeriodStart:    start,
			PeriodEnd:      end,
			SpaceID:        strings.TrimSpace(body.SpaceID),
			IdempotencyKey: key,
			RawBody:        string(raw),
			Bearer:         r.Header.Get("Authorization"),
		})
		if err != nil {
			writeReportErr(w, r, err)
			return
		}
		status := http.StatusCreated
		if replay {
			status = http.StatusOK
		}
		writeJSON(w, status, toReportDTO(got))
	}
}

func ExportReport(svc app.Reports) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(r); err != nil {
			writeReportAuth(w, r, err)
			return
		}
		id := r.PathValue("reportId")
		if !canonicalUUID.MatchString(id) {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "El identificador no es un UUID canónico", []map[string]string{{"field": "reportId", "message": "Debe ser un UUID en minúsculas con guiones"}})
			return
		}
		format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = "csv"
		}
		if format != "csv" && format != "json" {
			writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "format debe ser csv o json", []map[string]string{{"field": "format", "message": "csv o json"}})
			return
		}
		got, err := svc.Get(r.Context(), principalFrom(r), id)
		if err != nil {
			writeReportErr(w, r, err)
			return
		}
		if got.State != domain.ReportReady {
			writeErr(w, r, http.StatusUnprocessableEntity, "INVALID_STATUS_TRANSITION", "El informe no está en estado READY", nil)
			return
		}
		if format == "json" {
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="report-%s.json"`, got.ID))
			writeJSON(w, http.StatusOK, toReportDTO(got))
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="report-%s.csv"`, got.ID))
		w.WriteHeader(http.StatusOK)
		cw := csv.NewWriter(w)
		headers := csvHeaders(got.Rows)
		_ = cw.Write(headers)
		for _, row := range got.Rows {
			rec := make([]string, 0, len(headers))
			for _, h := range headers {
				rec = append(rec, fmt.Sprint(row[h]))
			}
			_ = cw.Write(rec)
		}
		cw.Flush()
	}
}

func csvHeaders(rows []map[string]any) []string {
	if len(rows) == 0 {
		return []string{"count"}
	}
	out := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		out = append(out, k)
	}
	return out
}

func parseInstant(field, raw string) (time.Time, *bindError) {
	if !strings.HasSuffix(raw, "Z") {
		return time.Time{}, &bindError{msg: field + " debe ser RFC 3339 UTC con Z", details: []map[string]string{{"field": field, "message": "Debe terminar en Z"}}}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, &bindError{msg: field + " no es RFC 3339", details: []map[string]string{{"field": field, "message": "Debe ser RFC 3339 en UTC"}}}
	}
	return t.UTC(), nil
}

func requireAdmin(r *http.Request) error {
	p := principalFrom(r)
	if p.Subject == "" {
		return app.ErrUnauthorized
	}
	if !p.Admin() {
		return app.ErrForbidden
	}
	return nil
}

func writeReportAuth(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, app.ErrForbidden) {
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "Se requiere rol ADMIN", nil)
		return
	}
	writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token de autenticación requerido", nil)
}

func writeReportErr(w http.ResponseWriter, r *http.Request, err error) {
	var up app.UpstreamError
	if errors.As(err, &up) {
		msg := up.Service + " no respondió"
		writeErr(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", msg, nil)
		return
	}
	switch {
	case errors.Is(err, app.ErrForbidden):
		writeErr(w, r, http.StatusForbidden, "FORBIDDEN", "Se requiere rol ADMIN", nil)
	case errors.Is(err, app.ErrNotFound):
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "El informe no existe", nil)
	case errors.Is(err, app.ErrConflict):
		writeErr(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "La misma Idempotency-Key se usó con otro cuerpo", nil)
	case errors.Is(err, app.ErrBusiness):
		writeErr(w, r, http.StatusUnprocessableEntity, "BUSINESS_RULE_VIOLATION", "La solicitud de informe no cumple las reglas de periodo o spaceId", nil)
	case errors.Is(err, app.ErrValidation):
		writeErr(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Datos de entrada inválidos", nil)
	default:
		writeErr(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Error interno", nil)
	}
}
