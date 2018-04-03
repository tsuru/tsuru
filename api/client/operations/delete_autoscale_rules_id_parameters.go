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

// NewDeleteAutoscaleRulesIDParams creates a new DeleteAutoscaleRulesIDParams object
// with the default values initialized.
func NewDeleteAutoscaleRulesIDParams() *DeleteAutoscaleRulesIDParams {

	return &DeleteAutoscaleRulesIDParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDeleteAutoscaleRulesIDParamsWithTimeout creates a new DeleteAutoscaleRulesIDParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDeleteAutoscaleRulesIDParamsWithTimeout(timeout time.Duration) *DeleteAutoscaleRulesIDParams {

	return &DeleteAutoscaleRulesIDParams{

		timeout: timeout,
	}
}

// NewDeleteAutoscaleRulesIDParamsWithContext creates a new DeleteAutoscaleRulesIDParams object
// with the default values initialized, and the ability to set a context for a request
func NewDeleteAutoscaleRulesIDParamsWithContext(ctx context.Context) *DeleteAutoscaleRulesIDParams {

	return &DeleteAutoscaleRulesIDParams{

		Context: ctx,
	}
}

// NewDeleteAutoscaleRulesIDParamsWithHTTPClient creates a new DeleteAutoscaleRulesIDParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDeleteAutoscaleRulesIDParamsWithHTTPClient(client *http.Client) *DeleteAutoscaleRulesIDParams {

	return &DeleteAutoscaleRulesIDParams{
		HTTPClient: client,
	}
}

/*DeleteAutoscaleRulesIDParams contains all the parameters to send to the API endpoint
for the delete autoscale rules ID operation typically these are written to a http.Request
*/
type DeleteAutoscaleRulesIDParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the delete autoscale rules ID params
func (o *DeleteAutoscaleRulesIDParams) WithTimeout(timeout time.Duration) *DeleteAutoscaleRulesIDParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the delete autoscale rules ID params
func (o *DeleteAutoscaleRulesIDParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the delete autoscale rules ID params
func (o *DeleteAutoscaleRulesIDParams) WithContext(ctx context.Context) *DeleteAutoscaleRulesIDParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the delete autoscale rules ID params
func (o *DeleteAutoscaleRulesIDParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the delete autoscale rules ID params
func (o *DeleteAutoscaleRulesIDParams) WithHTTPClient(client *http.Client) *DeleteAutoscaleRulesIDParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the delete autoscale rules ID params
func (o *DeleteAutoscaleRulesIDParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *DeleteAutoscaleRulesIDParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
