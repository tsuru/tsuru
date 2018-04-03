// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// DeleteUsersKeysKeyReader is a Reader for the DeleteUsersKeysKey structure.
type DeleteUsersKeysKeyReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteUsersKeysKeyReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteUsersKeysKeyOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewDeleteUsersKeysKeyBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 401:
		result := NewDeleteUsersKeysKeyUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewDeleteUsersKeysKeyNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewDeleteUsersKeysKeyOK creates a DeleteUsersKeysKeyOK with default headers values
func NewDeleteUsersKeysKeyOK() *DeleteUsersKeysKeyOK {
	return &DeleteUsersKeysKeyOK{}
}

/*DeleteUsersKeysKeyOK handles this case with default header values.

Ok
*/
type DeleteUsersKeysKeyOK struct {
}

func (o *DeleteUsersKeysKeyOK) Error() string {
	return fmt.Sprintf("[DELETE /users/keys/{key}][%d] deleteUsersKeysKeyOK ", 200)
}

func (o *DeleteUsersKeysKeyOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteUsersKeysKeyBadRequest creates a DeleteUsersKeysKeyBadRequest with default headers values
func NewDeleteUsersKeysKeyBadRequest() *DeleteUsersKeysKeyBadRequest {
	return &DeleteUsersKeysKeyBadRequest{}
}

/*DeleteUsersKeysKeyBadRequest handles this case with default header values.

Invalid data
*/
type DeleteUsersKeysKeyBadRequest struct {
}

func (o *DeleteUsersKeysKeyBadRequest) Error() string {
	return fmt.Sprintf("[DELETE /users/keys/{key}][%d] deleteUsersKeysKeyBadRequest ", 400)
}

func (o *DeleteUsersKeysKeyBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteUsersKeysKeyUnauthorized creates a DeleteUsersKeysKeyUnauthorized with default headers values
func NewDeleteUsersKeysKeyUnauthorized() *DeleteUsersKeysKeyUnauthorized {
	return &DeleteUsersKeysKeyUnauthorized{}
}

/*DeleteUsersKeysKeyUnauthorized handles this case with default header values.

Unauthorized
*/
type DeleteUsersKeysKeyUnauthorized struct {
}

func (o *DeleteUsersKeysKeyUnauthorized) Error() string {
	return fmt.Sprintf("[DELETE /users/keys/{key}][%d] deleteUsersKeysKeyUnauthorized ", 401)
}

func (o *DeleteUsersKeysKeyUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteUsersKeysKeyNotFound creates a DeleteUsersKeysKeyNotFound with default headers values
func NewDeleteUsersKeysKeyNotFound() *DeleteUsersKeysKeyNotFound {
	return &DeleteUsersKeysKeyNotFound{}
}

/*DeleteUsersKeysKeyNotFound handles this case with default header values.

Not found
*/
type DeleteUsersKeysKeyNotFound struct {
}

func (o *DeleteUsersKeysKeyNotFound) Error() string {
	return fmt.Sprintf("[DELETE /users/keys/{key}][%d] deleteUsersKeysKeyNotFound ", 404)
}

func (o *DeleteUsersKeysKeyNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
