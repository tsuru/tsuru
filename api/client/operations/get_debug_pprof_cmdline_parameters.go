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

// NewGetDebugPprofCmdlineParams creates a new GetDebugPprofCmdlineParams object
// with the default values initialized.
func NewGetDebugPprofCmdlineParams() *GetDebugPprofCmdlineParams {

	return &GetDebugPprofCmdlineParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetDebugPprofCmdlineParamsWithTimeout creates a new GetDebugPprofCmdlineParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetDebugPprofCmdlineParamsWithTimeout(timeout time.Duration) *GetDebugPprofCmdlineParams {

	return &GetDebugPprofCmdlineParams{

		timeout: timeout,
	}
}

// NewGetDebugPprofCmdlineParamsWithContext creates a new GetDebugPprofCmdlineParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetDebugPprofCmdlineParamsWithContext(ctx context.Context) *GetDebugPprofCmdlineParams {

	return &GetDebugPprofCmdlineParams{

		Context: ctx,
	}
}

// NewGetDebugPprofCmdlineParamsWithHTTPClient creates a new GetDebugPprofCmdlineParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetDebugPprofCmdlineParamsWithHTTPClient(client *http.Client) *GetDebugPprofCmdlineParams {

	return &GetDebugPprofCmdlineParams{
		HTTPClient: client,
	}
}

/*GetDebugPprofCmdlineParams contains all the parameters to send to the API endpoint
for the get debug pprof cmdline operation typically these are written to a http.Request
*/
type GetDebugPprofCmdlineParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get debug pprof cmdline params
func (o *GetDebugPprofCmdlineParams) WithTimeout(timeout time.Duration) *GetDebugPprofCmdlineParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get debug pprof cmdline params
func (o *GetDebugPprofCmdlineParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get debug pprof cmdline params
func (o *GetDebugPprofCmdlineParams) WithContext(ctx context.Context) *GetDebugPprofCmdlineParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get debug pprof cmdline params
func (o *GetDebugPprofCmdlineParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get debug pprof cmdline params
func (o *GetDebugPprofCmdlineParams) WithHTTPClient(client *http.Client) *GetDebugPprofCmdlineParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get debug pprof cmdline params
func (o *GetDebugPprofCmdlineParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetDebugPprofCmdlineParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
