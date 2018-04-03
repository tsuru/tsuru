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

// NewPostEventsUUIDCancelParams creates a new PostEventsUUIDCancelParams object
// with the default values initialized.
func NewPostEventsUUIDCancelParams() *PostEventsUUIDCancelParams {

	return &PostEventsUUIDCancelParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewPostEventsUUIDCancelParamsWithTimeout creates a new PostEventsUUIDCancelParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewPostEventsUUIDCancelParamsWithTimeout(timeout time.Duration) *PostEventsUUIDCancelParams {

	return &PostEventsUUIDCancelParams{

		timeout: timeout,
	}
}

// NewPostEventsUUIDCancelParamsWithContext creates a new PostEventsUUIDCancelParams object
// with the default values initialized, and the ability to set a context for a request
func NewPostEventsUUIDCancelParamsWithContext(ctx context.Context) *PostEventsUUIDCancelParams {

	return &PostEventsUUIDCancelParams{

		Context: ctx,
	}
}

// NewPostEventsUUIDCancelParamsWithHTTPClient creates a new PostEventsUUIDCancelParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewPostEventsUUIDCancelParamsWithHTTPClient(client *http.Client) *PostEventsUUIDCancelParams {

	return &PostEventsUUIDCancelParams{
		HTTPClient: client,
	}
}

/*PostEventsUUIDCancelParams contains all the parameters to send to the API endpoint
for the post events UUID cancel operation typically these are written to a http.Request
*/
type PostEventsUUIDCancelParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the post events UUID cancel params
func (o *PostEventsUUIDCancelParams) WithTimeout(timeout time.Duration) *PostEventsUUIDCancelParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the post events UUID cancel params
func (o *PostEventsUUIDCancelParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the post events UUID cancel params
func (o *PostEventsUUIDCancelParams) WithContext(ctx context.Context) *PostEventsUUIDCancelParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the post events UUID cancel params
func (o *PostEventsUUIDCancelParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the post events UUID cancel params
func (o *PostEventsUUIDCancelParams) WithHTTPClient(client *http.Client) *PostEventsUUIDCancelParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the post events UUID cancel params
func (o *PostEventsUUIDCancelParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *PostEventsUUIDCancelParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
