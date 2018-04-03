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

// NewPostPoolsPoolTeamParams creates a new PostPoolsPoolTeamParams object
// with the default values initialized.
func NewPostPoolsPoolTeamParams() *PostPoolsPoolTeamParams {
	var ()
	return &PostPoolsPoolTeamParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewPostPoolsPoolTeamParamsWithTimeout creates a new PostPoolsPoolTeamParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewPostPoolsPoolTeamParamsWithTimeout(timeout time.Duration) *PostPoolsPoolTeamParams {
	var ()
	return &PostPoolsPoolTeamParams{

		timeout: timeout,
	}
}

// NewPostPoolsPoolTeamParamsWithContext creates a new PostPoolsPoolTeamParams object
// with the default values initialized, and the ability to set a context for a request
func NewPostPoolsPoolTeamParamsWithContext(ctx context.Context) *PostPoolsPoolTeamParams {
	var ()
	return &PostPoolsPoolTeamParams{

		Context: ctx,
	}
}

// NewPostPoolsPoolTeamParamsWithHTTPClient creates a new PostPoolsPoolTeamParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewPostPoolsPoolTeamParamsWithHTTPClient(client *http.Client) *PostPoolsPoolTeamParams {
	var ()
	return &PostPoolsPoolTeamParams{
		HTTPClient: client,
	}
}

/*PostPoolsPoolTeamParams contains all the parameters to send to the API endpoint
for the post pools pool team operation typically these are written to a http.Request
*/
type PostPoolsPoolTeamParams struct {

	/*Pool
	  Pool name.

	*/
	Pool string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the post pools pool team params
func (o *PostPoolsPoolTeamParams) WithTimeout(timeout time.Duration) *PostPoolsPoolTeamParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the post pools pool team params
func (o *PostPoolsPoolTeamParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the post pools pool team params
func (o *PostPoolsPoolTeamParams) WithContext(ctx context.Context) *PostPoolsPoolTeamParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the post pools pool team params
func (o *PostPoolsPoolTeamParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the post pools pool team params
func (o *PostPoolsPoolTeamParams) WithHTTPClient(client *http.Client) *PostPoolsPoolTeamParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the post pools pool team params
func (o *PostPoolsPoolTeamParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithPool adds the pool to the post pools pool team params
func (o *PostPoolsPoolTeamParams) WithPool(pool string) *PostPoolsPoolTeamParams {
	o.SetPool(pool)
	return o
}

// SetPool adds the pool to the post pools pool team params
func (o *PostPoolsPoolTeamParams) SetPool(pool string) {
	o.Pool = pool
}

// WriteToRequest writes these params to a swagger request
func (o *PostPoolsPoolTeamParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param pool
	if err := r.SetPathParam("pool", o.Pool); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
