// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// GetDebugPprofReader is a Reader for the GetDebugPprof structure.
type GetDebugPprofReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetDebugPprofReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetDebugPprofOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewGetDebugPprofUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewGetDebugPprofOK creates a GetDebugPprofOK with default headers values
func NewGetDebugPprofOK() *GetDebugPprofOK {
	return &GetDebugPprofOK{}
}

/*GetDebugPprofOK handles this case with default header values.

Ok
*/
type GetDebugPprofOK struct {
}

func (o *GetDebugPprofOK) Error() string {
	return fmt.Sprintf("[GET /debug/pprof][%d] getDebugPprofOK ", 200)
}

func (o *GetDebugPprofOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetDebugPprofUnauthorized creates a GetDebugPprofUnauthorized with default headers values
func NewGetDebugPprofUnauthorized() *GetDebugPprofUnauthorized {
	return &GetDebugPprofUnauthorized{}
}

/*GetDebugPprofUnauthorized handles this case with default header values.

Unauthorized
*/
type GetDebugPprofUnauthorized struct {
}

func (o *GetDebugPprofUnauthorized) Error() string {
	return fmt.Sprintf("[GET /debug/pprof][%d] getDebugPprofUnauthorized ", 401)
}

func (o *GetDebugPprofUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
