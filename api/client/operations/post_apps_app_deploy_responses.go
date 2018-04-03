// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PostAppsAppDeployReader is a Reader for the PostAppsAppDeploy structure.
type PostAppsAppDeployReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PostAppsAppDeployReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPostAppsAppDeployOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPostAppsAppDeployBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 403:
		result := NewPostAppsAppDeployForbidden()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewPostAppsAppDeployNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPostAppsAppDeployOK creates a PostAppsAppDeployOK with default headers values
func NewPostAppsAppDeployOK() *PostAppsAppDeployOK {
	return &PostAppsAppDeployOK{}
}

/*PostAppsAppDeployOK handles this case with default header values.

OK
*/
type PostAppsAppDeployOK struct {
}

func (o *PostAppsAppDeployOK) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/deploy][%d] postAppsAppDeployOK ", 200)
}

func (o *PostAppsAppDeployOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppDeployBadRequest creates a PostAppsAppDeployBadRequest with default headers values
func NewPostAppsAppDeployBadRequest() *PostAppsAppDeployBadRequest {
	return &PostAppsAppDeployBadRequest{}
}

/*PostAppsAppDeployBadRequest handles this case with default header values.

Invalid data
*/
type PostAppsAppDeployBadRequest struct {
}

func (o *PostAppsAppDeployBadRequest) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/deploy][%d] postAppsAppDeployBadRequest ", 400)
}

func (o *PostAppsAppDeployBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppDeployForbidden creates a PostAppsAppDeployForbidden with default headers values
func NewPostAppsAppDeployForbidden() *PostAppsAppDeployForbidden {
	return &PostAppsAppDeployForbidden{}
}

/*PostAppsAppDeployForbidden handles this case with default header values.

Forbidden
*/
type PostAppsAppDeployForbidden struct {
}

func (o *PostAppsAppDeployForbidden) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/deploy][%d] postAppsAppDeployForbidden ", 403)
}

func (o *PostAppsAppDeployForbidden) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAppsAppDeployNotFound creates a PostAppsAppDeployNotFound with default headers values
func NewPostAppsAppDeployNotFound() *PostAppsAppDeployNotFound {
	return &PostAppsAppDeployNotFound{}
}

/*PostAppsAppDeployNotFound handles this case with default header values.

Not found
*/
type PostAppsAppDeployNotFound struct {
}

func (o *PostAppsAppDeployNotFound) Error() string {
	return fmt.Sprintf("[POST /apps/{app}/deploy][%d] postAppsAppDeployNotFound ", 404)
}

func (o *PostAppsAppDeployNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
