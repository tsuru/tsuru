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

// NewDeleteDockerNodecontainersNodecontainerParams creates a new DeleteDockerNodecontainersNodecontainerParams object
// with the default values initialized.
func NewDeleteDockerNodecontainersNodecontainerParams() *DeleteDockerNodecontainersNodecontainerParams {
	var ()
	return &DeleteDockerNodecontainersNodecontainerParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDeleteDockerNodecontainersNodecontainerParamsWithTimeout creates a new DeleteDockerNodecontainersNodecontainerParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDeleteDockerNodecontainersNodecontainerParamsWithTimeout(timeout time.Duration) *DeleteDockerNodecontainersNodecontainerParams {
	var ()
	return &DeleteDockerNodecontainersNodecontainerParams{

		timeout: timeout,
	}
}

// NewDeleteDockerNodecontainersNodecontainerParamsWithContext creates a new DeleteDockerNodecontainersNodecontainerParams object
// with the default values initialized, and the ability to set a context for a request
func NewDeleteDockerNodecontainersNodecontainerParamsWithContext(ctx context.Context) *DeleteDockerNodecontainersNodecontainerParams {
	var ()
	return &DeleteDockerNodecontainersNodecontainerParams{

		Context: ctx,
	}
}

// NewDeleteDockerNodecontainersNodecontainerParamsWithHTTPClient creates a new DeleteDockerNodecontainersNodecontainerParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDeleteDockerNodecontainersNodecontainerParamsWithHTTPClient(client *http.Client) *DeleteDockerNodecontainersNodecontainerParams {
	var ()
	return &DeleteDockerNodecontainersNodecontainerParams{
		HTTPClient: client,
	}
}

/*DeleteDockerNodecontainersNodecontainerParams contains all the parameters to send to the API endpoint
for the delete docker nodecontainers nodecontainer operation typically these are written to a http.Request
*/
type DeleteDockerNodecontainersNodecontainerParams struct {

	/*Nodecontainer
	  Nodecontainer name.

	*/
	Nodecontainer string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) WithTimeout(timeout time.Duration) *DeleteDockerNodecontainersNodecontainerParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) WithContext(ctx context.Context) *DeleteDockerNodecontainersNodecontainerParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) WithHTTPClient(client *http.Client) *DeleteDockerNodecontainersNodecontainerParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithNodecontainer adds the nodecontainer to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) WithNodecontainer(nodecontainer string) *DeleteDockerNodecontainersNodecontainerParams {
	o.SetNodecontainer(nodecontainer)
	return o
}

// SetNodecontainer adds the nodecontainer to the delete docker nodecontainers nodecontainer params
func (o *DeleteDockerNodecontainersNodecontainerParams) SetNodecontainer(nodecontainer string) {
	o.Nodecontainer = nodecontainer
}

// WriteToRequest writes these params to a swagger request
func (o *DeleteDockerNodecontainersNodecontainerParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param nodecontainer
	if err := r.SetPathParam("nodecontainer", o.Nodecontainer); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
