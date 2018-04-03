// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PostDockerNodecontainersReader is a Reader for the PostDockerNodecontainers structure.
type PostDockerNodecontainersReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PostDockerNodecontainersReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPostDockerNodecontainersOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPostDockerNodecontainersBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 401:
		result := NewPostDockerNodecontainersUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPostDockerNodecontainersOK creates a PostDockerNodecontainersOK with default headers values
func NewPostDockerNodecontainersOK() *PostDockerNodecontainersOK {
	return &PostDockerNodecontainersOK{}
}

/*PostDockerNodecontainersOK handles this case with default header values.

Ok
*/
type PostDockerNodecontainersOK struct {
}

func (o *PostDockerNodecontainersOK) Error() string {
	return fmt.Sprintf("[POST /docker/nodecontainers][%d] postDockerNodecontainersOK ", 200)
}

func (o *PostDockerNodecontainersOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostDockerNodecontainersBadRequest creates a PostDockerNodecontainersBadRequest with default headers values
func NewPostDockerNodecontainersBadRequest() *PostDockerNodecontainersBadRequest {
	return &PostDockerNodecontainersBadRequest{}
}

/*PostDockerNodecontainersBadRequest handles this case with default header values.

Invald data
*/
type PostDockerNodecontainersBadRequest struct {
}

func (o *PostDockerNodecontainersBadRequest) Error() string {
	return fmt.Sprintf("[POST /docker/nodecontainers][%d] postDockerNodecontainersBadRequest ", 400)
}

func (o *PostDockerNodecontainersBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostDockerNodecontainersUnauthorized creates a PostDockerNodecontainersUnauthorized with default headers values
func NewPostDockerNodecontainersUnauthorized() *PostDockerNodecontainersUnauthorized {
	return &PostDockerNodecontainersUnauthorized{}
}

/*PostDockerNodecontainersUnauthorized handles this case with default header values.

Unauthorized
*/
type PostDockerNodecontainersUnauthorized struct {
}

func (o *PostDockerNodecontainersUnauthorized) Error() string {
	return fmt.Sprintf("[POST /docker/nodecontainers][%d] postDockerNodecontainersUnauthorized ", 401)
}

func (o *PostDockerNodecontainersUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
