package app

import "errors"

var (
	ErrValidation      = errors.New("validation")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("idempotency conflict")
	ErrBusiness        = errors.New("business rule")
	ErrImmutable       = errors.New("report not ready")
	ErrJWKSUnavailable = errors.New("jwks unavailable")
)

type UpstreamError struct {
	Service string
	Status  int
}

func (e UpstreamError) Error() string {
	if e.Service == "" {
		return "upstream unavailable"
	}
	return e.Service + " no respondió"
}
