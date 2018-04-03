// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// GetNodeAddressContainersReader is a Reader for the GetNodeAddressContainers structure.
type GetNodeAddressContainersReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *GetNodeAddressContainersReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewGetNodeAddressContainersOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 204:
		result := NewGetNodeAddressContainersNoContent()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewGetNodeAddressContainersUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewGetNodeAddressContainersNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewGetNodeAddressContainersOK creates a GetNodeAddressContainersOK with default headers values
func NewGetNodeAddressContainersOK() *GetNodeAddressContainersOK {
	return &GetNodeAddressContainersOK{}
}

/*GetNodeAddressContainersOK handles this case with default header values.

Ok
*/
type GetNodeAddressContainersOK struct {
}

func (o *GetNodeAddressContainersOK) Error() string {
	return fmt.Sprintf("[GET /node/{address}/containers][%d] getNodeAddressContainersOK ", 200)
}

func (o *GetNodeAddressContainersOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetNodeAddressContainersNoContent creates a GetNodeAddressContainersNoContent with default headers values
func NewGetNodeAddressContainersNoContent() *GetNodeAddressContainersNoContent {
	return &GetNodeAddressContainersNoContent{}
}

/*GetNodeAddressContainersNoContent handles this case with default header values.

No content
*/
type GetNodeAddressContainersNoContent struct {
}

func (o *GetNodeAddressContainersNoContent) Error() string {
	return fmt.Sprintf("[GET /node/{address}/containers][%d] getNodeAddressContainersNoContent ", 204)
}

func (o *GetNodeAddressContainersNoContent) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetNodeAddressContainersUnauthorized creates a GetNodeAddressContainersUnauthorized with default headers values
func NewGetNodeAddressContainersUnauthorized() *GetNodeAddressContainersUnauthorized {
	return &GetNodeAddressContainersUnauthorized{}
}

/*GetNodeAddressContainersUnauthorized handles this case with default header values.

Unauthorized
*/
type GetNodeAddressContainersUnauthorized struct {
}

func (o *GetNodeAddressContainersUnauthorized) Error() string {
	return fmt.Sprintf("[GET /node/{address}/containers][%d] getNodeAddressContainersUnauthorized ", 401)
}

func (o *GetNodeAddressContainersUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewGetNodeAddressContainersNotFound creates a GetNodeAddressContainersNotFound with default headers values
func NewGetNodeAddressContainersNotFound() *GetNodeAddressContainersNotFound {
	return &GetNodeAddressContainersNotFound{}
}

/*GetNodeAddressContainersNotFound handles this case with default header values.

Not found
*/
type GetNodeAddressContainersNotFound struct {
}

func (o *GetNodeAddressContainersNotFound) Error() string {
	return fmt.Sprintf("[GET /node/{address}/containers][%d] getNodeAddressContainersNotFound ", 404)
}

func (o *GetNodeAddressContainersNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
