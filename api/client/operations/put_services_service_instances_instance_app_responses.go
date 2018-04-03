// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PutServicesServiceInstancesInstanceAppReader is a Reader for the PutServicesServiceInstancesInstanceApp structure.
type PutServicesServiceInstancesInstanceAppReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PutServicesServiceInstancesInstanceAppReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPutServicesServiceInstancesInstanceAppOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPutServicesServiceInstancesInstanceAppBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 401:
		result := NewPutServicesServiceInstancesInstanceAppUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewPutServicesServiceInstancesInstanceAppNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPutServicesServiceInstancesInstanceAppOK creates a PutServicesServiceInstancesInstanceAppOK with default headers values
func NewPutServicesServiceInstancesInstanceAppOK() *PutServicesServiceInstancesInstanceAppOK {
	return &PutServicesServiceInstancesInstanceAppOK{}
}

/*PutServicesServiceInstancesInstanceAppOK handles this case with default header values.

Ok
*/
type PutServicesServiceInstancesInstanceAppOK struct {
}

func (o *PutServicesServiceInstancesInstanceAppOK) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}/{app}][%d] putServicesServiceInstancesInstanceAppOK ", 200)
}

func (o *PutServicesServiceInstancesInstanceAppOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutServicesServiceInstancesInstanceAppBadRequest creates a PutServicesServiceInstancesInstanceAppBadRequest with default headers values
func NewPutServicesServiceInstancesInstanceAppBadRequest() *PutServicesServiceInstancesInstanceAppBadRequest {
	return &PutServicesServiceInstancesInstanceAppBadRequest{}
}

/*PutServicesServiceInstancesInstanceAppBadRequest handles this case with default header values.

Invalid data
*/
type PutServicesServiceInstancesInstanceAppBadRequest struct {
}

func (o *PutServicesServiceInstancesInstanceAppBadRequest) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}/{app}][%d] putServicesServiceInstancesInstanceAppBadRequest ", 400)
}

func (o *PutServicesServiceInstancesInstanceAppBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutServicesServiceInstancesInstanceAppUnauthorized creates a PutServicesServiceInstancesInstanceAppUnauthorized with default headers values
func NewPutServicesServiceInstancesInstanceAppUnauthorized() *PutServicesServiceInstancesInstanceAppUnauthorized {
	return &PutServicesServiceInstancesInstanceAppUnauthorized{}
}

/*PutServicesServiceInstancesInstanceAppUnauthorized handles this case with default header values.

Unauthorized
*/
type PutServicesServiceInstancesInstanceAppUnauthorized struct {
}

func (o *PutServicesServiceInstancesInstanceAppUnauthorized) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}/{app}][%d] putServicesServiceInstancesInstanceAppUnauthorized ", 401)
}

func (o *PutServicesServiceInstancesInstanceAppUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutServicesServiceInstancesInstanceAppNotFound creates a PutServicesServiceInstancesInstanceAppNotFound with default headers values
func NewPutServicesServiceInstancesInstanceAppNotFound() *PutServicesServiceInstancesInstanceAppNotFound {
	return &PutServicesServiceInstancesInstanceAppNotFound{}
}

/*PutServicesServiceInstancesInstanceAppNotFound handles this case with default header values.

App not found
*/
type PutServicesServiceInstancesInstanceAppNotFound struct {
}

func (o *PutServicesServiceInstancesInstanceAppNotFound) Error() string {
	return fmt.Sprintf("[PUT /services/{service}/instances/{instance}/{app}][%d] putServicesServiceInstancesInstanceAppNotFound ", 404)
}

func (o *PutServicesServiceInstancesInstanceAppNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
