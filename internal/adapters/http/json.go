package httpadapter

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, msg string, details []map[string]string) {
	body := map[string]any{"error": code, "message": msg, "traceId": TraceID(r)}
	if details != nil {
		body["details"] = details
	}
	writeJSON(w, status, body)
}
