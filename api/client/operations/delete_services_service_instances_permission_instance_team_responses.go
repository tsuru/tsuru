// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"fmt"

	"github.com/go-openapi/runtime"

	strfmt "github.com/go-openapi/strfmt"
)

// DeleteServicesServiceInstancesPermissionInstanceTeamReader is a Reader for the DeleteServicesServiceInstancesPermissionInstanceTeam structure.
type DeleteServicesServiceInstancesPermissionInstanceTeamReader struct {
	formats strfmt.Registry
}

// ReadResponse reads a server response into the received o.
func (o *DeleteServicesServiceInstancesPermissionInstanceTeamReader) ReadResponse(response runtime.ClientResponse, consumer runtime.Consumer) (interface{}, error) {
	switch response.Code() {

	case 200:
		result := NewDeleteServicesServiceInstancesPermissionInstanceTeamOK()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return result, nil

	case 401:
		result := NewDeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	case 404:
		result := NewDeleteServicesServiceInstancesPermissionInstanceTeamNotFound()
		if err := result.readResponse(response, consumer, o.formats); err != nil {
			return nil, err
		}
		return nil, result

	default:
		return nil, runtime.NewAPIError("unknown error", response, response.Code())
	}
}

// NewDeleteServicesServiceInstancesPermissionInstanceTeamOK creates a DeleteServicesServiceInstancesPermissionInstanceTeamOK with default headers values
func NewDeleteServicesServiceInstancesPermissionInstanceTeamOK() *DeleteServicesServiceInstancesPermissionInstanceTeamOK {
	return &DeleteServicesServiceInstancesPermissionInstanceTeamOK{}
}

/*DeleteServicesServiceInstancesPermissionInstanceTeamOK handles this case with default header values.

Access revoked
*/
type DeleteServicesServiceInstancesPermissionInstanceTeamOK struct {
}

func (o *DeleteServicesServiceInstancesPermissionInstanceTeamOK) Error() string {
	return fmt.Sprintf("[DELETE /services/{service}/instances/permission/{instance}/{team}][%d] deleteServicesServiceInstancesPermissionInstanceTeamOK ", 200)
}

func (o *DeleteServicesServiceInstancesPermissionInstanceTeamOK) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized creates a DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized with default headers values
func NewDeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized() *DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized {
	return &DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized{}
}

/*DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized handles this case with default header values.

Unauthorized
*/
type DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized struct {
}

func (o *DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized) Error() string {
	return fmt.Sprintf("[DELETE /services/{service}/instances/permission/{instance}/{team}][%d] deleteServicesServiceInstancesPermissionInstanceTeamUnauthorized ", 401)
}

func (o *DeleteServicesServiceInstancesPermissionInstanceTeamUnauthorized) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}

// NewDeleteServicesServiceInstancesPermissionInstanceTeamNotFound creates a DeleteServicesServiceInstancesPermissionInstanceTeamNotFound with default headers values
func NewDeleteServicesServiceInstancesPermissionInstanceTeamNotFound() *DeleteServicesServiceInstancesPermissionInstanceTeamNotFound {
	return &DeleteServicesServiceInstancesPermissionInstanceTeamNotFound{}
}

/*DeleteServicesServiceInstancesPermissionInstanceTeamNotFound handles this case with default header values.

Service instance not found
*/
type DeleteServicesServiceInstancesPermissionInstanceTeamNotFound struct {
}

func (o *DeleteServicesServiceInstancesPermissionInstanceTeamNotFound) Error() string {
	return fmt.Sprintf("[DELETE /services/{service}/instances/permission/{instance}/{team}][%d] deleteServicesServiceInstancesPermissionInstanceTeamNotFound ", 404)
}

func (o *DeleteServicesServiceInstancesPermissionInstanceTeamNotFound) readResponse(response runtime.ClientResponse, consumer runtime.Consumer, formats strfmt.Registry) error {

	return nil
}
