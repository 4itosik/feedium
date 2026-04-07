package post

import "errors"

var ErrNotFound = errors.New("post not found")

type ValidationError struct {
	Msg string
}

func (e ValidationError) Error() string { return e.Msg }

func validationError(msg string) error { return ValidationError{Msg: msg} }

func NewValidationError(msg string) error { return ValidationError{Msg: msg} }
