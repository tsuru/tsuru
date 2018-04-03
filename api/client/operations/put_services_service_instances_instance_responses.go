// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PutServicesServiceInstancesInstanceReader is a Reader for the PutServicesServiceInstancesInstance structure.
type PutServicesServiceInstancesInstanceReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PutServicesServiceInstancesInstanceReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPutServicesServiceInstancesInstanceOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPutServicesServiceInstancesInstanceBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 401:
		result := NewPutServicesServiceInstancesInstanceUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewPutServicesServiceInstancesInstanceNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPutServicesServiceInstancesInstanceOK creates a PutServicesServiceInstancesInstanceOK with default headers values
func NewPutServicesServiceInstancesInstanceOK() *PutServicesServiceInstancesInstanceOK {
	return &PutServicesServiceInstancesInstanceOK{}
}

/*PutServicesServiceInstancesInstanceOK handles this case with default header values.

Service instance updated
*/
type PutServicesServiceInstancesInstanceOK struct {
}

func (o *PutServicesServiceInstancesInstanceOK) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}][%d] putServicesServiceInstancesInstanceOK ", 200)
}

func (o *PutServicesServiceInstancesInstanceOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutServicesServiceInstancesInstanceBadRequest creates a PutServicesServiceInstancesInstanceBadRequest with default headers values
func NewPutServicesServiceInstancesInstanceBadRequest() *PutServicesServiceInstancesInstanceBadRequest {
	return &PutServicesServiceInstancesInstanceBadRequest{}
}

/*PutServicesServiceInstancesInstanceBadRequest handles this case with default header values.

Invalid data
*/
type PutServicesServiceInstancesInstanceBadRequest struct {
}

func (o *PutServicesServiceInstancesInstanceBadRequest) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}][%d] putServicesServiceInstancesInstanceBadRequest ", 400)
}

func (o *PutServicesServiceInstancesInstanceBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutServicesServiceInstancesInstanceUnauthorized creates a PutServicesServiceInstancesInstanceUnauthorized with default headers values
func NewPutServicesServiceInstancesInstanceUnauthorized() *PutServicesServiceInstancesInstanceUnauthorized {
	return &PutServicesServiceInstancesInstanceUnauthorized{}
}

/*PutServicesServiceInstancesInstanceUnauthorized handles this case with default header values.

Unauthorized
*/
type PutServicesServiceInstancesInstanceUnauthorized struct {
}

func (o *PutServicesServiceInstancesInstanceUnauthorized) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}][%d] putServicesServiceInstancesInstanceUnauthorized ", 401)
}

func (o *PutServicesServiceInstancesInstanceUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutServicesServiceInstancesInstanceNotFound creates a PutServicesServiceInstancesInstanceNotFound with default headers values
func NewPutServicesServiceInstancesInstanceNotFound() *PutServicesServiceInstancesInstanceNotFound {
	return &PutServicesServiceInstancesInstanceNotFound{}
}

/*PutServicesServiceInstancesInstanceNotFound handles this case with default header values.

Service instance not found
*/
type PutServicesServiceInstancesInstanceNotFound struct {
}

func (o *PutServicesServiceInstancesInstanceNotFound) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}][%d] putServicesServiceInstancesInstanceNotFound ", 404)
}

func (o *PutServicesServiceInstancesInstanceNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
