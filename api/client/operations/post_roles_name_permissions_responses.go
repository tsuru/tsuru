// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PostRolesNamePermissionsReader is a Reader for the PostRolesNamePermissions structure.
type PostRolesNamePermissionsReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PostRolesNamePermissionsReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPostRolesNamePermissionsOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPostRolesNamePermissionsBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 401:
		result := NewPostRolesNamePermissionsUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 409:
		result := NewPostRolesNamePermissionsConflict()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPostRolesNamePermissionsOK creates a PostRolesNamePermissionsOK with default headers values
func NewPostRolesNamePermissionsOK() *PostRolesNamePermissionsOK {
	return &PostRolesNamePermissionsOK{}
}

/*PostRolesNamePermissionsOK handles this case with default header values.

Ok
*/
type PostRolesNamePermissionsOK struct {
}

func (o *PostRolesNamePermissionsOK) Error() string {
	return fmt.Sprintf("[POST /roles/{name}/permissions][%d] postRolesNamePermissionsOK ", 200)
}

func (o *PostRolesNamePermissionsOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostRolesNamePermissionsBadRequest creates a PostRolesNamePermissionsBadRequest with default headers values
func NewPostRolesNamePermissionsBadRequest() *PostRolesNamePermissionsBadRequest {
	return &PostRolesNamePermissionsBadRequest{}
}

/*PostRolesNamePermissionsBadRequest handles this case with default header values.

Invalid data
*/
type PostRolesNamePermissionsBadRequest struct {
}

func (o *PostRolesNamePermissionsBadRequest) Error() string {
	return fmt.Sprintf("[POST /roles/{name}/permissions][%d] postRolesNamePermissionsBadRequest ", 400)
}

func (o *PostRolesNamePermissionsBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostRolesNamePermissionsUnauthorized creates a PostRolesNamePermissionsUnauthorized with default headers values
func NewPostRolesNamePermissionsUnauthorized() *PostRolesNamePermissionsUnauthorized {
	return &PostRolesNamePermissionsUnauthorized{}
}

/*PostRolesNamePermissionsUnauthorized handles this case with default header values.

Unauthorized
*/
type PostRolesNamePermissionsUnauthorized struct {
}

func (o *PostRolesNamePermissionsUnauthorized) Error() string {
	return fmt.Sprintf("[POST /roles/{name}/permissions][%d] postRolesNamePermissionsUnauthorized ", 401)
}

func (o *PostRolesNamePermissionsUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostRolesNamePermissionsConflict creates a PostRolesNamePermissionsConflict with default headers values
func NewPostRolesNamePermissionsConflict() *PostRolesNamePermissionsConflict {
	return &PostRolesNamePermissionsConflict{}
}

/*PostRolesNamePermissionsConflict handles this case with default header values.

Permission not allowed
*/
type PostRolesNamePermissionsConflict struct {
}

func (o *PostRolesNamePermissionsConflict) Error() string {
	return fmt.Sprintf("[POST /roles/{name}/permissions][%d] postRolesNamePermissionsConflict ", 409)
}

func (o *PostRolesNamePermissionsConflict) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
