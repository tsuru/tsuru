// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PutAppAppRoutersNameReader is a Reader for the PutAppAppRoutersName structure.
type PutAppAppRoutersNameReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PutAppAppRoutersNameReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPutAppAppRoutersNameOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPutAppAppRoutersNameBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewPutAppAppRoutersNameNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPutAppAppRoutersNameOK creates a PutAppAppRoutersNameOK with default headers values
func NewPutAppAppRoutersNameOK() *PutAppAppRoutersNameOK {
	return &PutAppAppRoutersNameOK{}
}

/*PutAppAppRoutersNameOK handles this case with default header values.

OK
*/
type PutAppAppRoutersNameOK struct {
}

func (o *PutAppAppRoutersNameOK) Error() string {
	return fmt.Sprintf("[PUT /app/{app}/routers/{name}][%d] putAppAppRoutersNameOK ", 200)
}

func (o *PutAppAppRoutersNameOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutAppAppRoutersNameBadRequest creates a PutAppAppRoutersNameBadRequest with default headers values
func NewPutAppAppRoutersNameBadRequest() *PutAppAppRoutersNameBadRequest {
	return &PutAppAppRoutersNameBadRequest{}
}

/*PutAppAppRoutersNameBadRequest handles this case with default header values.

Invalid request
*/
type PutAppAppRoutersNameBadRequest struct {
}

func (o *PutAppAppRoutersNameBadRequest) Error() string {
	return fmt.Sprintf("[PUT /app/{app}/routers/{name}][%d] putAppAppRoutersNameBadRequest ", 400)
}

func (o *PutAppAppRoutersNameBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPutAppAppRoutersNameNotFound creates a PutAppAppRoutersNameNotFound with default headers values
func NewPutAppAppRoutersNameNotFound() *PutAppAppRoutersNameNotFound {
	return &PutAppAppRoutersNameNotFound{}
}

/*PutAppAppRoutersNameNotFound handles this case with default header values.

App or router not found
*/
type PutAppAppRoutersNameNotFound struct {
}

func (o *PutAppAppRoutersNameNotFound) Error() string {
	return fmt.Sprintf("[PUT /app/{app}/routers/{name}][%d] putAppAppRoutersNameNotFound ", 404)
}

func (o *PutAppAppRoutersNameNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
