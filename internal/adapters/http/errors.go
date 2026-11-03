package httpadapter

import (
	"net/http"
)

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	TraceID string `json:"traceId,omitempty"`
}

func NotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{
			Error:   "NOT_FOUND",
			Message: "Ruta no encontrada",
			TraceID: TraceID(r),
		})
	})
}
