package messagebus

import (
	"errors"
	"strings"
)

const (
	FailureClassRetryable    = "RETRYABLE"
	FailureClassNonRetryable = "NON_RETRYABLE"
	FailureClassUnknown      = "UNKNOWN_RESULT"
)

type MessageFailure struct {
	Class string
	Err   error
}

func (e MessageFailure) Error() string {
	if e.Err == nil {
		return e.Class
	}
	return e.Err.Error()
}

func (e MessageFailure) Unwrap() error {
	return e.Err
}

func (e MessageFailure) Retryable() bool {
	return strings.ToUpper(strings.TrimSpace(e.Class)) != FailureClassNonRetryable
}

func RetryableMessageError(err error) error {
	return MessageFailure{Class: FailureClassRetryable, Err: err}
}

func NonRetryableMessageError(err error) error {
	return MessageFailure{Class: FailureClassNonRetryable, Err: err}
}

func UnknownResultMessageError(err error) error {
	return MessageFailure{Class: FailureClassUnknown, Err: err}
}

func classifyMessageFailure(err error) string {
	if err == nil {
		return ""
	}
	var failure MessageFailure
	if errors.As(err, &failure) {
		class := strings.ToUpper(strings.TrimSpace(failure.Class))
		switch class {
		case FailureClassRetryable, FailureClassNonRetryable, FailureClassUnknown:
			return class
		}
	}
	return FailureClassRetryable
}
