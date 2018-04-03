// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PostAppsAppnameBuildReader is a Reader for the PostAppsAppnameBuild structure.
type PostAppsAppnameBuildReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PostAppsAppnameBuildReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPostAppsAppnameBuildOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPostAppsAppnameBuildBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 403:
		result := NewPostAppsAppnameBuildForbidden()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewPostAppsAppnameBuildNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPostAppsAppnameBuildOK creates a PostAppsAppnameBuildOK with default headers values
func NewPostAppsAppnameBuildOK() *PostAppsAppnameBuildOK {
	return &PostAppsAppnameBuildOK{}
}

/*PostAppsAppnameBuildOK handles this case with default header values.

OK
*/
type PostAppsAppnameBuildOK struct {
}

func (o *PostAppsAppnameBuildOK) Error() string {
	return fmt.Sprintf("[POST /apps/{appname}/build][%d] postAppsAppnameBuildOK ", 200)
}

func (o *PostAppsAppnameBuildOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppnameBuildBadRequest creates a PostAppsAppnameBuildBadRequest with default headers values
func NewPostAppsAppnameBuildBadRequest() *PostAppsAppnameBuildBadRequest {
	return &PostAppsAppnameBuildBadRequest{}
}

/*PostAppsAppnameBuildBadRequest handles this case with default header values.

Invalid data
*/
type PostAppsAppnameBuildBadRequest struct {
}

func (o *PostAppsAppnameBuildBadRequest) Error() string {
	return fmt.Sprintf("[POST /apps/{appname}/build][%d] postAppsAppnameBuildBadRequest ", 400)
}

func (o *PostAppsAppnameBuildBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppnameBuildForbidden creates a PostAppsAppnameBuildForbidden with default headers values
func NewPostAppsAppnameBuildForbidden() *PostAppsAppnameBuildForbidden {
	return &PostAppsAppnameBuildForbidden{}
}

/*PostAppsAppnameBuildForbidden handles this case with default header values.

Forbidden
*/
type PostAppsAppnameBuildForbidden struct {
}

func (o *PostAppsAppnameBuildForbidden) Error() string {
	return fmt.Sprintf("[POST /apps/{appname}/build][%d] postAppsAppnameBuildForbidden ", 403)
}

func (o *PostAppsAppnameBuildForbidden) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppnameBuildNotFound creates a PostAppsAppnameBuildNotFound with default headers values
func NewPostAppsAppnameBuildNotFound() *PostAppsAppnameBuildNotFound {
	return &PostAppsAppnameBuildNotFound{}
}

/*PostAppsAppnameBuildNotFound handles this case with default header values.

Not found
*/
type PostAppsAppnameBuildNotFound struct {
}

func (o *PostAppsAppnameBuildNotFound) Error() string {
	return fmt.Sprintf("[POST /apps/{appname}/build][%d] postAppsAppnameBuildNotFound ", 404)
}

func (o *PostAppsAppnameBuildNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
