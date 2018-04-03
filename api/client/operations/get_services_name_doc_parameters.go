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

// NewGetServicesNameDocParams creates a new GetServicesNameDocParams object
// with the default values initialized.
func NewGetServicesNameDocParams() *GetServicesNameDocParams {

	return &GetServicesNameDocParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetServicesNameDocParamsWithTimeout creates a new GetServicesNameDocParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetServicesNameDocParamsWithTimeout(timeout time.Duration) *GetServicesNameDocParams {

	return &GetServicesNameDocParams{

		timeout: timeout,
	}
}

// NewGetServicesNameDocParamsWithContext creates a new GetServicesNameDocParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetServicesNameDocParamsWithContext(ctx context.Context) *GetServicesNameDocParams {

	return &GetServicesNameDocParams{

		Context: ctx,
	}
}

// NewGetServicesNameDocParamsWithHTTPClient creates a new GetServicesNameDocParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetServicesNameDocParamsWithHTTPClient(client *http.Client) *GetServicesNameDocParams {

	return &GetServicesNameDocParams{
		HTTPClient: client,
	}
}

/*GetServicesNameDocParams contains all the parameters to send to the API endpoint
for the get services name doc operation typically these are written to a http.Request
*/
type GetServicesNameDocParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get services name doc params
func (o *GetServicesNameDocParams) WithTimeout(timeout time.Duration) *GetServicesNameDocParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get services name doc params
func (o *GetServicesNameDocParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get services name doc params
func (o *GetServicesNameDocParams) WithContext(ctx context.Context) *GetServicesNameDocParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get services name doc params
func (o *GetServicesNameDocParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get services name doc params
func (o *GetServicesNameDocParams) WithHTTPClient(client *http.Client) *GetServicesNameDocParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get services name doc params
func (o *GetServicesNameDocParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetServicesNameDocParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
