package httpadapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type ctxKey string

const (
	correlationKey ctxKey = "traceId"
	subjectKey     ctxKey = "sub"
)

func WithCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-Id")
		if id == "" {
			id = newTraceID()
		}
		w.Header().Set("X-Correlation-Id", id)
		ctx := context.WithValue(r.Context(), correlationKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TraceID(r *http.Request) string {
	if v, ok := r.Context().Value(correlationKey).(string); ok {
		return v
	}
	return ""
}

func stripSpoofedIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Clone()
		for k := range h {
			if strings.HasPrefix(strings.ToLower(k), "x-user") {
				h.Del(k)
			}
		}
		r.Header = h
		next.ServeHTTP(w, r)
	})
}

func newTraceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
