// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PostAppsAppStopReader is a Reader for the PostAppsAppStop structure.
type PostAppsAppStopReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PostAppsAppStopReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPostAppsAppStopOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewPostAppsAppStopUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewPostAppsAppStopNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPostAppsAppStopOK creates a PostAppsAppStopOK with default headers values
func NewPostAppsAppStopOK() *PostAppsAppStopOK {
	return &PostAppsAppStopOK{}
}

/*PostAppsAppStopOK handles this case with default header values.

Ok
*/
type PostAppsAppStopOK struct {
}

func (o *PostAppsAppStopOK) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/stop][%d] postAppsAppStopOK ", 200)
}

func (o *PostAppsAppStopOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppStopUnauthorized creates a PostAppsAppStopUnauthorized with default headers values
func NewPostAppsAppStopUnauthorized() *PostAppsAppStopUnauthorized {
	return &PostAppsAppStopUnauthorized{}
}

/*PostAppsAppStopUnauthorized handles this case with default header values.

Unauthorized
*/
type PostAppsAppStopUnauthorized struct {
}

func (o *PostAppsAppStopUnauthorized) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/stop][%d] postAppsAppStopUnauthorized ", 401)
}

func (o *PostAppsAppStopUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppStopNotFound creates a PostAppsAppStopNotFound with default headers values
func NewPostAppsAppStopNotFound() *PostAppsAppStopNotFound {
	return &PostAppsAppStopNotFound{}
}

/*PostAppsAppStopNotFound handles this case with default header values.

App not found
*/
type PostAppsAppStopNotFound struct {
}

func (o *PostAppsAppStopNotFound) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/stop][%d] postAppsAppStopNotFound ", 404)
}

func (o *PostAppsAppStopNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
