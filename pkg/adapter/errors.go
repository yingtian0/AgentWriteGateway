package adapter

import (
	"errors"
	"fmt"
)

type ErrorClass string

const (
	ErrorRetryable ErrorClass = "RETRYABLE"
	ErrorTerminal  ErrorClass = "TERMINAL"
	ErrorUnknown   ErrorClass = "UNKNOWN_EXTERNAL_STATE"
)

type Error struct {
	Class         ErrorClass
	Operation     string
	CorrelationID string
	Err           error
}

func (e *Error) Error() string {
	if e.CorrelationID == "" {
		return fmt.Sprintf("%s %s: %v", e.Class, e.Operation, e.Err)
	}
	return fmt.Sprintf("%s %s correlation=%s: %v", e.Class, e.Operation, e.CorrelationID, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func IsClass(err error, class ErrorClass) bool {
	var classified *Error
	return errors.As(err, &classified) && classified.Class == class
}
