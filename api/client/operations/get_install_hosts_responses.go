// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// GetInstallHostsReader is a Reader for the GetInstallHosts structure.
type GetInstallHostsReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetInstallHostsReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetInstallHostsOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewGetInstallHostsUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewGetInstallHostsOK creates a GetInstallHostsOK with default headers values
func NewGetInstallHostsOK() *GetInstallHostsOK {
	return &GetInstallHostsOK{}
}

/*GetInstallHostsOK handles this case with default header values.

OK
*/
type GetInstallHostsOK struct {
}

func (o *GetInstallHostsOK) Error() string {
	return fmt.Sprintf("[GET /install/hosts][%d] getInstallHostsOK ", 200)
}

func (o *GetInstallHostsOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetInstallHostsUnauthorized creates a GetInstallHostsUnauthorized with default headers values
func NewGetInstallHostsUnauthorized() *GetInstallHostsUnauthorized {
	return &GetInstallHostsUnauthorized{}
}

/*GetInstallHostsUnauthorized handles this case with default header values.

Unauthorized
*/
type GetInstallHostsUnauthorized struct {
}

func (o *GetInstallHostsUnauthorized) Error() string {
	return fmt.Sprintf("[GET /install/hosts][%d] getInstallHostsUnauthorized ", 401)
}

func (o *GetInstallHostsUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
