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

// NewGetInstallHostsNameParams creates a new GetInstallHostsNameParams object
// with the default values initialized.
func NewGetInstallHostsNameParams() *GetInstallHostsNameParams {

	return &GetInstallHostsNameParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetInstallHostsNameParamsWithTimeout creates a new GetInstallHostsNameParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetInstallHostsNameParamsWithTimeout(timeout time.Duration) *GetInstallHostsNameParams {

	return &GetInstallHostsNameParams{

		timeout: timeout,
	}
}

// NewGetInstallHostsNameParamsWithContext creates a new GetInstallHostsNameParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetInstallHostsNameParamsWithContext(ctx context.Context) *GetInstallHostsNameParams {

	return &GetInstallHostsNameParams{

		Context: ctx,
	}
}

// NewGetInstallHostsNameParamsWithHTTPClient creates a new GetInstallHostsNameParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetInstallHostsNameParamsWithHTTPClient(client *http.Client) *GetInstallHostsNameParams {

	return &GetInstallHostsNameParams{
		HTTPClient: client,
	}
}

/*GetInstallHostsNameParams contains all the parameters to send to the API endpoint
for the get install hosts name operation typically these are written to a http.Request
*/
type GetInstallHostsNameParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get install hosts name params
func (o *GetInstallHostsNameParams) WithTimeout(timeout time.Duration) *GetInstallHostsNameParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get install hosts name params
func (o *GetInstallHostsNameParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get install hosts name params
func (o *GetInstallHostsNameParams) WithContext(ctx context.Context) *GetInstallHostsNameParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get install hosts name params
func (o *GetInstallHostsNameParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get install hosts name params
func (o *GetInstallHostsNameParams) WithHTTPClient(client *http.Client) *GetInstallHostsNameParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get install hosts name params
func (o *GetInstallHostsNameParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetInstallHostsNameParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
