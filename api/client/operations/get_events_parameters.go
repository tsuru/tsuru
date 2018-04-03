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

// NewGetEventsParams creates a new GetEventsParams object
// with the default values initialized.
func NewGetEventsParams() *GetEventsParams {

	return &GetEventsParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetEventsParamsWithTimeout creates a new GetEventsParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetEventsParamsWithTimeout(timeout time.Duration) *GetEventsParams {

	return &GetEventsParams{

		timeout: timeout,
	}
}

// NewGetEventsParamsWithContext creates a new GetEventsParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetEventsParamsWithContext(ctx context.Context) *GetEventsParams {

	return &GetEventsParams{

		Context: ctx,
	}
}

// NewGetEventsParamsWithHTTPClient creates a new GetEventsParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetEventsParamsWithHTTPClient(client *http.Client) *GetEventsParams {

	return &GetEventsParams{
		HTTPClient: client,
	}
}

/*GetEventsParams contains all the parameters to send to the API endpoint
for the get events operation typically these are written to a http.Request
*/
type GetEventsParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get events params
func (o *GetEventsParams) WithTimeout(timeout time.Duration) *GetEventsParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get events params
func (o *GetEventsParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get events params
func (o *GetEventsParams) WithContext(ctx context.Context) *GetEventsParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get events params
func (o *GetEventsParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get events params
func (o *GetEventsParams) WithHTTPClient(client *http.Client) *GetEventsParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get events params
func (o *GetEventsParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *GetEventsParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
