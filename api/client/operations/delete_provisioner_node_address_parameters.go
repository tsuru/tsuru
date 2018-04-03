// Code generated by go-swagger; DO NOT EDIT.

package operations

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"

	strfmt "github.com/go-openapi/strfmt"
)

// NewDeleteProvisionerNodeAddressParams creates a new DeleteProvisionerNodeAddressParams object
// with the default values initialized.
func NewDeleteProvisionerNodeAddressParams() *DeleteProvisionerNodeAddressParams {

	return &DeleteProvisionerNodeAddressParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDeleteProvisionerNodeAddressParamsWithTimeout creates a new DeleteProvisionerNodeAddressParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDeleteProvisionerNodeAddressParamsWithTimeout(timeout time.Duration) *DeleteProvisionerNodeAddressParams {

	return &DeleteProvisionerNodeAddressParams{

		timeout: timeout,
	}
}

// NewDeleteProvisionerNodeAddressParamsWithContext creates a new DeleteProvisionerNodeAddressParams object
// with the default values initialized, and the ability to set a context for a request
func NewDeleteProvisionerNodeAddressParamsWithContext(ctx context.Context) *DeleteProvisionerNodeAddressParams {

	return &DeleteProvisionerNodeAddressParams{

		Context: ctx,
	}
}

// NewDeleteProvisionerNodeAddressParamsWithHTTPClient creates a new DeleteProvisionerNodeAddressParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDeleteProvisionerNodeAddressParamsWithHTTPClient(client *http.Client) *DeleteProvisionerNodeAddressParams {

	return &DeleteProvisionerNodeAddressParams{
		HTTPClient: client,
	}
}

/*DeleteProvisionerNodeAddressParams contains all the parameters to send to the API endpoint
for the delete provisioner node address operation typically these are written to a http.Request
*/
type DeleteProvisionerNodeAddressParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the delete provisioner node address params
func (o *DeleteProvisionerNodeAddressParams) WithTimeout(timeout time.Duration) *DeleteProvisionerNodeAddressParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the delete provisioner node address params
func (o *DeleteProvisionerNodeAddressParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the delete provisioner node address params
func (o *DeleteProvisionerNodeAddressParams) WithContext(ctx context.Context) *DeleteProvisionerNodeAddressParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the delete provisioner node address params
func (o *DeleteProvisionerNodeAddressParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the delete provisioner node address params
func (o *DeleteProvisionerNodeAddressParams) WithHTTPClient(client *http.Client) *DeleteProvisionerNodeAddressParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the delete provisioner node address params
func (o *DeleteProvisionerNodeAddressParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *DeleteProvisionerNodeAddressParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
