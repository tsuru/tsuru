// Code generated by go-swagger; DO NOT EDIT.

package app

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// AppCreateReader is a Reader for the AppCreate structure.
type AppCreateReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *AppCreateReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 201:
		result := NewAppCreateCreated()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewAppCreateBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 401:
		result := NewAppCreateUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 403:
		result := NewAppCreateForbidden()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 409:
		result := NewAppCreateConflict()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewAppCreateCreated creates a AppCreateCreated with default headers values
func NewAppCreateCreated() *AppCreateCreated {
	return &AppCreateCreated{}
}

/*AppCreateCreated handles this case with default header values.

App created
*/
type AppCreateCreated struct {
}

func (o *AppCreateCreated) Error() string {
	return fmt.Sprintf("[POST /apps][%d] appCreateCreated ", 201)
}

func (o *AppCreateCreated) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewAppCreateBadRequest creates a AppCreateBadRequest with default headers values
func NewAppCreateBadRequest() *AppCreateBadRequest {
	return &AppCreateBadRequest{}
}

/*AppCreateBadRequest handles this case with default header values.

Invalid data
*/
type AppCreateBadRequest struct {
}

func (o *AppCreateBadRequest) Error() string {
	return fmt.Sprintf("[POST /apps][%d] appCreateBadRequest ", 400)
}

func (o *AppCreateBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewAppCreateUnauthorized creates a AppCreateUnauthorized with default headers values
func NewAppCreateUnauthorized() *AppCreateUnauthorized {
	return &AppCreateUnauthorized{}
}

/*AppCreateUnauthorized handles this case with default header values.

Unauthorized
*/
type AppCreateUnauthorized struct {
}

func (o *AppCreateUnauthorized) Error() string {
	return fmt.Sprintf("[POST /apps][%d] appCreateUnauthorized ", 401)
}

func (o *AppCreateUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewAppCreateForbidden creates a AppCreateForbidden with default headers values
func NewAppCreateForbidden() *AppCreateForbidden {
	return &AppCreateForbidden{}
}

/*AppCreateForbidden handles this case with default header values.

Quota exceeded
*/
type AppCreateForbidden struct {
}

func (o *AppCreateForbidden) Error() string {
	return fmt.Sprintf("[POST /apps][%d] appCreateForbidden ", 403)
}

func (o *AppCreateForbidden) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewAppCreateConflict creates a AppCreateConflict with default headers values
func NewAppCreateConflict() *AppCreateConflict {
	return &AppCreateConflict{}
}

/*AppCreateConflict handles this case with default header values.

App already exists
*/
type AppCreateConflict struct {
}

func (o *AppCreateConflict) Error() string {
	return fmt.Sprintf("[POST /apps][%d] appCreateConflict ", 409)
}

func (o *AppCreateConflict) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
