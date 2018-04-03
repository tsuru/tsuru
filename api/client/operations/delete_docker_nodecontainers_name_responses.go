// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// DeleteDockerNodecontainersNameReader is a Reader for the DeleteDockerNodecontainersName structure.
type DeleteDockerNodecontainersNameReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteDockerNodecontainersNameReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteDockerNodecontainersNameOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewDeleteDockerNodecontainersNameUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewDeleteDockerNodecontainersNameNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewDeleteDockerNodecontainersNameOK creates a DeleteDockerNodecontainersNameOK with default headers values
func NewDeleteDockerNodecontainersNameOK() *DeleteDockerNodecontainersNameOK {
	return &DeleteDockerNodecontainersNameOK{}
}

/*DeleteDockerNodecontainersNameOK handles this case with default header values.

Ok
*/
type DeleteDockerNodecontainersNameOK struct {
}

func (o *DeleteDockerNodecontainersNameOK) Error() string {
	return fmt.Sprintf("[DELETE /docker/nodecontainers/{name}][%d] deleteDockerNodecontainersNameOK ", 200)
}

func (o *DeleteDockerNodecontainersNameOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteDockerNodecontainersNameUnauthorized creates a DeleteDockerNodecontainersNameUnauthorized with default headers values
func NewDeleteDockerNodecontainersNameUnauthorized() *DeleteDockerNodecontainersNameUnauthorized {
	return &DeleteDockerNodecontainersNameUnauthorized{}
}

/*DeleteDockerNodecontainersNameUnauthorized handles this case with default header values.

Unauthorized
*/
type DeleteDockerNodecontainersNameUnauthorized struct {
}

func (o *DeleteDockerNodecontainersNameUnauthorized) Error() string {
	return fmt.Sprintf("[DELETE /docker/nodecontainers/{name}][%d] deleteDockerNodecontainersNameUnauthorized ", 401)
}

func (o *DeleteDockerNodecontainersNameUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteDockerNodecontainersNameNotFound creates a DeleteDockerNodecontainersNameNotFound with default headers values
func NewDeleteDockerNodecontainersNameNotFound() *DeleteDockerNodecontainersNameNotFound {
	return &DeleteDockerNodecontainersNameNotFound{}
}

/*DeleteDockerNodecontainersNameNotFound handles this case with default header values.

Not found
*/
type DeleteDockerNodecontainersNameNotFound struct {
}

func (o *DeleteDockerNodecontainersNameNotFound) Error() string {
	return fmt.Sprintf("[DELETE /docker/nodecontainers/{name}][%d] deleteDockerNodecontainersNameNotFound ", 404)
}

func (o *DeleteDockerNodecontainersNameNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
