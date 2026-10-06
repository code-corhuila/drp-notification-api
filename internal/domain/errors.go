package domain

import "errors"

var (
	ErrInvalidInput      = errors.New("invalid input")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrImmutableReport   = errors.New("report already generated")
)
