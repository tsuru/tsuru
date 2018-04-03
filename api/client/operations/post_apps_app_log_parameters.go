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

// NewPostAppsAppLogParams creates a new PostAppsAppLogParams object
// with the default values initialized.
func NewPostAppsAppLogParams() *PostAppsAppLogParams {
	var ()
	return &PostAppsAppLogParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewPostAppsAppLogParamsWithTimeout creates a new PostAppsAppLogParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewPostAppsAppLogParamsWithTimeout(timeout time.Duration) *PostAppsAppLogParams {
	var ()
	return &PostAppsAppLogParams{

		timeout: timeout,
	}
}

// NewPostAppsAppLogParamsWithContext creates a new PostAppsAppLogParams object
// with the default values initialized, and the ability to set a context for a request
func NewPostAppsAppLogParamsWithContext(ctx context.Context) *PostAppsAppLogParams {
	var ()
	return &PostAppsAppLogParams{

		Context: ctx,
	}
}

// NewPostAppsAppLogParamsWithHTTPClient creates a new PostAppsAppLogParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewPostAppsAppLogParamsWithHTTPClient(client *http.Client) *PostAppsAppLogParams {
	var ()
	return &PostAppsAppLogParams{
		HTTPClient: client,
	}
}

/*PostAppsAppLogParams contains all the parameters to send to the API endpoint
for the post apps app log operation typically these are written to a http.Request
*/
type PostAppsAppLogParams struct {

	/*App
	  App name.

	*/
	App string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the post apps app log params
func (o *PostAppsAppLogParams) WithTimeout(timeout time.Duration) *PostAppsAppLogParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the post apps app log params
func (o *PostAppsAppLogParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the post apps app log params
func (o *PostAppsAppLogParams) WithContext(ctx context.Context) *PostAppsAppLogParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the post apps app log params
func (o *PostAppsAppLogParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the post apps app log params
func (o *PostAppsAppLogParams) WithHTTPClient(client *http.Client) *PostAppsAppLogParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the post apps app log params
func (o *PostAppsAppLogParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithApp adds the app to the post apps app log params
func (o *PostAppsAppLogParams) WithApp(app string) *PostAppsAppLogParams {
	o.SetApp(app)
	return o
}

// SetApp adds the app to the post apps app log params
func (o *PostAppsAppLogParams) SetApp(app string) {
	o.App = app
}

// WriteToRequest writes these params to a swagger request
func (o *PostAppsAppLogParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param app
	if err := r.SetPathParam("app", o.App); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
