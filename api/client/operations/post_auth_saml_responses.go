// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// PostAuthSamlReader is a Reader for the PostAuthSaml structure.
type PostAuthSamlReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *PostAuthSamlReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewPostAuthSamlOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 400:
		result := NewPostAuthSamlBadRequest()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewPostAuthSamlOK creates a PostAuthSamlOK with default headers values
func NewPostAuthSamlOK() *PostAuthSamlOK {
	return &PostAuthSamlOK{}
}

/*PostAuthSamlOK handles this case with default header values.

Ok
*/
type PostAuthSamlOK struct {
}

func (o *PostAuthSamlOK) Error() string {
	return fmt.Sprintf("[POST /auth/saml][%d] postAuthSamlOK ", 200)
}

func (o *PostAuthSamlOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewPostAuthSamlBadRequest creates a PostAuthSamlBadRequest with default headers values
func NewPostAuthSamlBadRequest() *PostAuthSamlBadRequest {
	return &PostAuthSamlBadRequest{}
}

/*PostAuthSamlBadRequest handles this case with default header values.

Invalid data
*/
type PostAuthSamlBadRequest struct {
}

func (o *PostAuthSamlBadRequest) Error() string {
	return fmt.Sprintf("[POST /auth/saml][%d] postAuthSamlBadRequest ", 400)
}

func (o *PostAuthSamlBadRequest) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
