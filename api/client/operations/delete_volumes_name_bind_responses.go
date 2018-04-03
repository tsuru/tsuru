// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// DeleteVolumesNameBindReader is a Reader for the DeleteVolumesNameBind structure.
type DeleteVolumesNameBindReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteVolumesNameBindReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteVolumesNameBindOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewDeleteVolumesNameBindUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewDeleteVolumesNameBindNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewDeleteVolumesNameBindOK creates a DeleteVolumesNameBindOK with default headers values
func NewDeleteVolumesNameBindOK() *DeleteVolumesNameBindOK {
	return &DeleteVolumesNameBindOK{}
}

/*DeleteVolumesNameBindOK handles this case with default header values.

Volume unbinded
*/
type DeleteVolumesNameBindOK struct {
}

func (o *DeleteVolumesNameBindOK) Error() string {
	return fmt.Sprintf("[DELETE /volumes/{name}/bind][%d] deleteVolumesNameBindOK ", 200)
}

func (o *DeleteVolumesNameBindOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteVolumesNameBindUnauthorized creates a DeleteVolumesNameBindUnauthorized with default headers values
func NewDeleteVolumesNameBindUnauthorized() *DeleteVolumesNameBindUnauthorized {
	return &DeleteVolumesNameBindUnauthorized{}
}

/*DeleteVolumesNameBindUnauthorized handles this case with default header values.

Unauthorized
*/
type DeleteVolumesNameBindUnauthorized struct {
}

func (o *DeleteVolumesNameBindUnauthorized) Error() string {
	return fmt.Sprintf("[DELETE /volumes/{name}/bind][%d] deleteVolumesNameBindUnauthorized ", 401)
}

func (o *DeleteVolumesNameBindUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteVolumesNameBindNotFound creates a DeleteVolumesNameBindNotFound with default headers values
func NewDeleteVolumesNameBindNotFound() *DeleteVolumesNameBindNotFound {
	return &DeleteVolumesNameBindNotFound{}
}

/*DeleteVolumesNameBindNotFound handles this case with default header values.

Volume not found
*/
type DeleteVolumesNameBindNotFound struct {
}

func (o *DeleteVolumesNameBindNotFound) Error() string {
	return fmt.Sprintf("[DELETE /volumes/{name}/bind][%d] deleteVolumesNameBindNotFound ", 404)
}

func (o *DeleteVolumesNameBindNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
