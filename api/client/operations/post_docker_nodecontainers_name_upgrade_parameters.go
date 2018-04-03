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

// NewPostDockerNodecontainersNameUpgradeParams creates a new PostDockerNodecontainersNameUpgradeParams object
// with the default values initialized.
func NewPostDockerNodecontainersNameUpgradeParams() *PostDockerNodecontainersNameUpgradeParams {

	return &PostDockerNodecontainersNameUpgradeParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewPostDockerNodecontainersNameUpgradeParamsWithTimeout creates a new PostDockerNodecontainersNameUpgradeParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewPostDockerNodecontainersNameUpgradeParamsWithTimeout(timeout time.Duration) *PostDockerNodecontainersNameUpgradeParams {

	return &PostDockerNodecontainersNameUpgradeParams{

		timeout: timeout,
	}
}

// NewPostDockerNodecontainersNameUpgradeParamsWithContext creates a new PostDockerNodecontainersNameUpgradeParams object
// with the default values initialized, and the ability to set a context for a request
func NewPostDockerNodecontainersNameUpgradeParamsWithContext(ctx context.Context) *PostDockerNodecontainersNameUpgradeParams {

	return &PostDockerNodecontainersNameUpgradeParams{

		Context: ctx,
	}
}

// NewPostDockerNodecontainersNameUpgradeParamsWithHTTPClient creates a new PostDockerNodecontainersNameUpgradeParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewPostDockerNodecontainersNameUpgradeParamsWithHTTPClient(client *http.Client) *PostDockerNodecontainersNameUpgradeParams {

	return &PostDockerNodecontainersNameUpgradeParams{
		HTTPClient: client,
	}
}

/*PostDockerNodecontainersNameUpgradeParams contains all the parameters to send to the API endpoint
for the post docker nodecontainers name upgrade operation typically these are written to a http.Request
*/
type PostDockerNodecontainersNameUpgradeParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the post docker nodecontainers name upgrade params
func (o *PostDockerNodecontainersNameUpgradeParams) WithTimeout(timeout time.Duration) *PostDockerNodecontainersNameUpgradeParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the post docker nodecontainers name upgrade params
func (o *PostDockerNodecontainersNameUpgradeParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the post docker nodecontainers name upgrade params
func (o *PostDockerNodecontainersNameUpgradeParams) WithContext(ctx context.Context) *PostDockerNodecontainersNameUpgradeParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the post docker nodecontainers name upgrade params
func (o *PostDockerNodecontainersNameUpgradeParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the post docker nodecontainers name upgrade params
func (o *PostDockerNodecontainersNameUpgradeParams) WithHTTPClient(client *http.Client) *PostDockerNodecontainersNameUpgradeParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the post docker nodecontainers name upgrade params
func (o *PostDockerNodecontainersNameUpgradeParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *PostDockerNodecontainersNameUpgradeParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
