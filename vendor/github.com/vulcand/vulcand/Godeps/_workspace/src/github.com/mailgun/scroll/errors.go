package scroll

import (
	"fmt"
	"net/http"
)

type GenericAPIError struct {
	Reason string
}

func (e GenericAPIError) Error() string {
	return e.Reason
}

type MissingFieldError struct {
	Field string
}

func (e MissingFieldError) Error() string {
	return fmt.Sprintf("Missing mandatory parameter: %v", e.Field)
}

type InvalidFormatError struct {
	Field string
	Value string
}

func (e InvalidFormatError) Error() string {
	return fmt.Sprintf("Invalid format for parameter %v: %v", e.Field, e.Value)
}

type InvalidParameterError struct {
	Field string
	Value string
}

func (e InvalidParameterError) Error() string {
	return fmt.Sprintf("Invalid parameter: %v %v", e.Field, e.Value)
}

type NotFoundError struct {
	Description string
}

func (e NotFoundError) Error() string {
	return e.Description
}

type ConflictError struct {
	Description string
}

func (e ConflictError) Error() string {
	return e.Description
}

type UnsafeFieldError struct {
	Field       string
	Description string
}

func (e UnsafeFieldError) Error() string {
	return fmt.Sprintf("field %q is unsafe: %v", e.Field, e.Description)
}

func responseAndStatusFor(err error) (Response, int) {
	switch err.(type) {
	case GenericAPIError, MissingFieldError, InvalidFormatError, InvalidParameterError, UnsafeFieldError:
		return Response{"message": err.Error()}, http.StatusBadRequest
	case NotFoundError:
		return Response{"message": err.Error()}, http.StatusNotFound
	case ConflictError:
		return Response{"message": err.Error()}, http.StatusConflict
	default:
		return Response{"message": "Internal Server Error"}, http.StatusInternalServerError
	}
}
