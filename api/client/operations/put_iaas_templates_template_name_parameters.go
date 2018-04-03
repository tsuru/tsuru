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

// NewPutIaasTemplatesTemplateNameParams creates a new PutIaasTemplatesTemplateNameParams object
// with the default values initialized.
func NewPutIaasTemplatesTemplateNameParams() *PutIaasTemplatesTemplateNameParams {

	return &PutIaasTemplatesTemplateNameParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewPutIaasTemplatesTemplateNameParamsWithTimeout creates a new PutIaasTemplatesTemplateNameParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewPutIaasTemplatesTemplateNameParamsWithTimeout(timeout time.Duration) *PutIaasTemplatesTemplateNameParams {

	return &PutIaasTemplatesTemplateNameParams{

		timeout: timeout,
	}
}

// NewPutIaasTemplatesTemplateNameParamsWithContext creates a new PutIaasTemplatesTemplateNameParams object
// with the default values initialized, and the ability to set a context for a request
func NewPutIaasTemplatesTemplateNameParamsWithContext(ctx context.Context) *PutIaasTemplatesTemplateNameParams {

	return &PutIaasTemplatesTemplateNameParams{

		Context: ctx,
	}
}

// NewPutIaasTemplatesTemplateNameParamsWithHTTPClient creates a new PutIaasTemplatesTemplateNameParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewPutIaasTemplatesTemplateNameParamsWithHTTPClient(client *http.Client) *PutIaasTemplatesTemplateNameParams {

	return &PutIaasTemplatesTemplateNameParams{
		HTTPClient: client,
	}
}

/*PutIaasTemplatesTemplateNameParams contains all the parameters to send to the API endpoint
for the put iaas templates template name operation typically these are written to a http.Request
*/
type PutIaasTemplatesTemplateNameParams struct {
	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the put iaas templates template name params
func (o *PutIaasTemplatesTemplateNameParams) WithTimeout(timeout time.Duration) *PutIaasTemplatesTemplateNameParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the put iaas templates template name params
func (o *PutIaasTemplatesTemplateNameParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the put iaas templates template name params
func (o *PutIaasTemplatesTemplateNameParams) WithContext(ctx context.Context) *PutIaasTemplatesTemplateNameParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the put iaas templates template name params
func (o *PutIaasTemplatesTemplateNameParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the put iaas templates template name params
func (o *PutIaasTemplatesTemplateNameParams) WithHTTPClient(client *http.Client) *PutIaasTemplatesTemplateNameParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the put iaas templates template name params
func (o *PutIaasTemplatesTemplateNameParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WriteToRequest writes these params to a swagger request
func (o *PutIaasTemplatesTemplateNameParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
