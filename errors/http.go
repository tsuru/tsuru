// Package errors provides facilities with error handling.
package errors

// Http represents an HTTP error. It implements the error interface.
//
// Each HTTP error has a Code and a message explaining what went wrong.
type Http struct {
	// Status code.
	Code int

	// Message explaining what went wrong.
	Message string
}

func (e *Http) Error() string {
	return e.Message
}
