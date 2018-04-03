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

// NewGetNodeAddressParams creates a new GetNodeAddressParams object
// with the default values initialized.
func NewGetNodeAddressParams() *GetNodeAddressParams {
	var ()
	return &GetNodeAddressParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetNodeAddressParamsWithTimeout creates a new GetNodeAddressParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetNodeAddressParamsWithTimeout(timeout time.Duration) *GetNodeAddressParams {
	var ()
	return &GetNodeAddressParams{

		timeout: timeout,
	}
}

// NewGetNodeAddressParamsWithContext creates a new GetNodeAddressParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetNodeAddressParamsWithContext(ctx context.Context) *GetNodeAddressParams {
	var ()
	return &GetNodeAddressParams{

		Context: ctx,
	}
}

// NewGetNodeAddressParamsWithHTTPClient creates a new GetNodeAddressParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetNodeAddressParamsWithHTTPClient(client *http.Client) *GetNodeAddressParams {
	var ()
	return &GetNodeAddressParams{
		HTTPClient: client,
	}
}

/*GetNodeAddressParams contains all the parameters to send to the API endpoint
for the get node address operation typically these are written to a http.Request
*/
type GetNodeAddressParams struct {

	/*Address
	  Node address.

	*/
	Address string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get node address params
func (o *GetNodeAddressParams) WithTimeout(timeout time.Duration) *GetNodeAddressParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get node address params
func (o *GetNodeAddressParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get node address params
func (o *GetNodeAddressParams) WithContext(ctx context.Context) *GetNodeAddressParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get node address params
func (o *GetNodeAddressParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get node address params
func (o *GetNodeAddressParams) WithHTTPClient(client *http.Client) *GetNodeAddressParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get node address params
func (o *GetNodeAddressParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithAddress adds the address to the get node address params
func (o *GetNodeAddressParams) WithAddress(address string) *GetNodeAddressParams {
	o.SetAddress(address)
	return o
}

// SetAddress adds the address to the get node address params
func (o *GetNodeAddressParams) SetAddress(address string) {
	o.Address = address
}

// WriteToRequest writes these params to a swagger request
func (o *GetNodeAddressParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param address
	if err := r.SetPathParam("address", o.Address); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
