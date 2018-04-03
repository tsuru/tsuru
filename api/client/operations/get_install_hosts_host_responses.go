// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// GetInstallHostsHostReader is a Reader for the GetInstallHostsHost structure.
type GetInstallHostsHostReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetInstallHostsHostReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetInstallHostsHostOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewGetInstallHostsHostUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewGetInstallHostsHostNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewGetInstallHostsHostOK creates a GetInstallHostsHostOK with default headers values
func NewGetInstallHostsHostOK() *GetInstallHostsHostOK {
	return &GetInstallHostsHostOK{}
}

/*GetInstallHostsHostOK handles this case with default header values.

OK
*/
type GetInstallHostsHostOK struct {
}

func (o *GetInstallHostsHostOK) Error() string {
	return fmt.Sprintf("[GET /install/hosts/{host}][%d] getInstallHostsHostOK ", 200)
}

func (o *GetInstallHostsHostOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetInstallHostsHostUnauthorized creates a GetInstallHostsHostUnauthorized with default headers values
func NewGetInstallHostsHostUnauthorized() *GetInstallHostsHostUnauthorized {
	return &GetInstallHostsHostUnauthorized{}
}

/*GetInstallHostsHostUnauthorized handles this case with default header values.

Unauthorized
*/
type GetInstallHostsHostUnauthorized struct {
}

func (o *GetInstallHostsHostUnauthorized) Error() string {
	return fmt.Sprintf("[GET /install/hosts/{host}][%d] getInstallHostsHostUnauthorized ", 401)
}

func (o *GetInstallHostsHostUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetInstallHostsHostNotFound creates a GetInstallHostsHostNotFound with default headers values
func NewGetInstallHostsHostNotFound() *GetInstallHostsHostNotFound {
	return &GetInstallHostsHostNotFound{}
}

/*GetInstallHostsHostNotFound handles this case with default header values.

Not Found
*/
type GetInstallHostsHostNotFound struct {
}

func (o *GetInstallHostsHostNotFound) Error() string {
	return fmt.Sprintf("[GET /install/hosts/{host}][%d] getInstallHostsHostNotFound ", 404)
}

func (o *GetInstallHostsHostNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
