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

// NewPostAppsAppnameBuildParams creates a new PostAppsAppnameBuildParams object
// with the default values initialized.
func NewPostAppsAppnameBuildParams() *PostAppsAppnameBuildParams {

	return &PostAppsAppnameBuildParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewPostAppsAppnameBuildParamsWithTimeout creates a new PostAppsAppnameBuildParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewPostAppsAppnameBuildParamsWithTimeout(timeout time.Duration) *PostAppsAppnameBuildParams {

	return &PostAppsAppnameBuildParams{

		timeout: timeout,
	}
}

// NewPostAppsAppnameBuildParamsWithContext creates a new PostAppsAppnameBuildParams object
// with the default values initialized, and the ability to set a context for a request
func NewPostAppsAppnameBuildParamsWithContext(ctx context.Context) *PostAppsAppnameBuildParams {

	return &PostAppsAppnameBuildParams{

		Context: ctx,
	}
}

// NewPostAppsAppnameBuildParamsWithHTTPClient creates a new PostAppsAppnameBuildParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewPostAppsAppnameBuildParamsWithHTTPClient(client *http.Client) *PostAppsAppnameBuildParams {

	return &PostAppsAppnameBuildParams{
		HTTPClient: client,
	}
}

/*PostAppsAppnameBuildParams contains all the parameters to send to the API endpoint
for the post apps appname build operation typically these are written to a http.Request
*/
type PostAppsAppnameBuildParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the post apps appname build params
func (o *PostAppsAppnameBuildParams) WithTimeout(timeout time.Duration) *PostAppsAppnameBuildParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the post apps appname build params
func (o *PostAppsAppnameBuildParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the post apps appname build params
func (o *PostAppsAppnameBuildParams) WithContext(ctx context.Context) *PostAppsAppnameBuildParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the post apps appname build params
func (o *PostAppsAppnameBuildParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the post apps appname build params
func (o *PostAppsAppnameBuildParams) WithHTTPClient(client *http.Client) *PostAppsAppnameBuildParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the post apps appname build params
func (o *PostAppsAppnameBuildParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *PostAppsAppnameBuildParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
