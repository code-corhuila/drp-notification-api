package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/code-corhuila/drp-notification-api/internal/app"
)

const principalKey ctxKey = "principal"

func RequireBearer(tokens app.TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if header == "" || len(header) < 7 || !strings.EqualFold(header[:7], "Bearer ") {
				writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token de autenticación requerido", nil)
				return
			}
			raw := strings.TrimSpace(header[7:])
			if raw == "" {
				writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token de autenticación requerido", nil)
				return
			}
			p, err := tokens.Parse(r.Context(), raw)
			if errors.Is(err, app.ErrJWKSUnavailable) {
				writeErr(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "No se pudo validar el token", nil)
				return
			}
			if err != nil || p.Subject == "" {
				writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Token de autenticación requerido", nil)
				return
			}
			ctx := context.WithValue(r.Context(), principalKey, p)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func principalFrom(r *http.Request) app.Principal {
	if v, ok := r.Context().Value(principalKey).(app.Principal); ok {
		return v
	}
	return app.Principal{}
}
