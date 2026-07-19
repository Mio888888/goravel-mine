package apperror

import "errors"

var ErrBusinessRule = errors.New("business rule violation")

type BusinessError struct {
	Message string
}

func (e BusinessError) Error() string {
	return e.Message
}

func (e BusinessError) Unwrap() error {
	return ErrBusinessRule
}
