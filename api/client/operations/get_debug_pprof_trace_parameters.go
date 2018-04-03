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

// NewGetDebugPprofTraceParams creates a new GetDebugPprofTraceParams object
// with the default values initialized.
func NewGetDebugPprofTraceParams() *GetDebugPprofTraceParams {

	return &GetDebugPprofTraceParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetDebugPprofTraceParamsWithTimeout creates a new GetDebugPprofTraceParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetDebugPprofTraceParamsWithTimeout(timeout time.Duration) *GetDebugPprofTraceParams {

	return &GetDebugPprofTraceParams{

		timeout: timeout,
	}
}

// NewGetDebugPprofTraceParamsWithContext creates a new GetDebugPprofTraceParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetDebugPprofTraceParamsWithContext(ctx context.Context) *GetDebugPprofTraceParams {

	return &GetDebugPprofTraceParams{

		Context: ctx,
	}
}

// NewGetDebugPprofTraceParamsWithHTTPClient creates a new GetDebugPprofTraceParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetDebugPprofTraceParamsWithHTTPClient(client *http.Client) *GetDebugPprofTraceParams {

	return &GetDebugPprofTraceParams{
		HTTPClient: client,
	}
}

/*GetDebugPprofTraceParams contains all the parameters to send to the API endpoint
for the get debug pprof trace operation typically these are written to a http.Request
*/
type GetDebugPprofTraceParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get debug pprof trace params
func (o *GetDebugPprofTraceParams) WithTimeout(timeout time.Duration) *GetDebugPprofTraceParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get debug pprof trace params
func (o *GetDebugPprofTraceParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get debug pprof trace params
func (o *GetDebugPprofTraceParams) WithContext(ctx context.Context) *GetDebugPprofTraceParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get debug pprof trace params
func (o *GetDebugPprofTraceParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get debug pprof trace params
func (o *GetDebugPprofTraceParams) WithHTTPClient(client *http.Client) *GetDebugPprofTraceParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get debug pprof trace params
func (o *GetDebugPprofTraceParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetDebugPprofTraceParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
