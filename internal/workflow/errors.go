package workflow

import (
	"errors"
	"fmt"
)

type ErrorClass string

const (
	ErrorRetryable            ErrorClass = "RETRYABLE"
	ErrorTerminal             ErrorClass = "TERMINAL"
	ErrorUnknownExternalState ErrorClass = "UNKNOWN_EXTERNAL_STATE"
)

type ClassifiedError struct {
	Class     ErrorClass
	Operation string
	Err       error
}

func (e *ClassifiedError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.Class, e.Operation, e.Err)
}

func (e *ClassifiedError) Unwrap() error { return e.Err }

func IsClass(err error, class ErrorClass) bool {
	var classified *ClassifiedError
	return errors.As(err, &classified) && classified.Class == class
}
