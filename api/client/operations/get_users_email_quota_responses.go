// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// GetUsersEmailQuotaReader is a Reader for the GetUsersEmailQuota structure.
type GetUsersEmailQuotaReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetUsersEmailQuotaReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetUsersEmailQuotaOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewGetUsersEmailQuotaUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewGetUsersEmailQuotaNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewGetUsersEmailQuotaOK creates a GetUsersEmailQuotaOK with default headers values
func NewGetUsersEmailQuotaOK() *GetUsersEmailQuotaOK {
	return &GetUsersEmailQuotaOK{}
}

/*GetUsersEmailQuotaOK handles this case with default header values.

OK
*/
type GetUsersEmailQuotaOK struct {
}

func (o *GetUsersEmailQuotaOK) Error() string {
	return fmt.Sprintf("[GET /users/{email}/quota][%d] getUsersEmailQuotaOK ", 200)
}

func (o *GetUsersEmailQuotaOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetUsersEmailQuotaUnauthorized creates a GetUsersEmailQuotaUnauthorized with default headers values
func NewGetUsersEmailQuotaUnauthorized() *GetUsersEmailQuotaUnauthorized {
	return &GetUsersEmailQuotaUnauthorized{}
}

/*GetUsersEmailQuotaUnauthorized handles this case with default header values.

Unauthorized
*/
type GetUsersEmailQuotaUnauthorized struct {
}

func (o *GetUsersEmailQuotaUnauthorized) Error() string {
	return fmt.Sprintf("[GET /users/{email}/quota][%d] getUsersEmailQuotaUnauthorized ", 401)
}

func (o *GetUsersEmailQuotaUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetUsersEmailQuotaNotFound creates a GetUsersEmailQuotaNotFound with default headers values
func NewGetUsersEmailQuotaNotFound() *GetUsersEmailQuotaNotFound {
	return &GetUsersEmailQuotaNotFound{}
}

/*GetUsersEmailQuotaNotFound handles this case with default header values.

User not found
*/
type GetUsersEmailQuotaNotFound struct {
}

func (o *GetUsersEmailQuotaNotFound) Error() string {
	return fmt.Sprintf("[GET /users/{email}/quota][%d] getUsersEmailQuotaNotFound ", 404)
}

func (o *GetUsersEmailQuotaNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
