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

// NewGetDebugPprofSymbolParams creates a new GetDebugPprofSymbolParams object
// with the default values initialized.
func NewGetDebugPprofSymbolParams() *GetDebugPprofSymbolParams {

	return &GetDebugPprofSymbolParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetDebugPprofSymbolParamsWithTimeout creates a new GetDebugPprofSymbolParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetDebugPprofSymbolParamsWithTimeout(timeout time.Duration) *GetDebugPprofSymbolParams {

	return &GetDebugPprofSymbolParams{

		timeout: timeout,
	}
}

// NewGetDebugPprofSymbolParamsWithContext creates a new GetDebugPprofSymbolParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetDebugPprofSymbolParamsWithContext(ctx context.Context) *GetDebugPprofSymbolParams {

	return &GetDebugPprofSymbolParams{

		Context: ctx,
	}
}

// NewGetDebugPprofSymbolParamsWithHTTPClient creates a new GetDebugPprofSymbolParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetDebugPprofSymbolParamsWithHTTPClient(client *http.Client) *GetDebugPprofSymbolParams {

	return &GetDebugPprofSymbolParams{
		HTTPClient: client,
	}
}

/*GetDebugPprofSymbolParams contains all the parameters to send to the API endpoint
for the get debug pprof symbol operation typically these are written to a http.Request
*/
type GetDebugPprofSymbolParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get debug pprof symbol params
func (o *GetDebugPprofSymbolParams) WithTimeout(timeout time.Duration) *GetDebugPprofSymbolParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get debug pprof symbol params
func (o *GetDebugPprofSymbolParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get debug pprof symbol params
func (o *GetDebugPprofSymbolParams) WithContext(ctx context.Context) *GetDebugPprofSymbolParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get debug pprof symbol params
func (o *GetDebugPprofSymbolParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get debug pprof symbol params
func (o *GetDebugPprofSymbolParams) WithHTTPClient(client *http.Client) *GetDebugPprofSymbolParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get debug pprof symbol params
func (o *GetDebugPprofSymbolParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetDebugPprofSymbolParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
