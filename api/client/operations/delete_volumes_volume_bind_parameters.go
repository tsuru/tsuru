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

// NewDeleteVolumesVolumeBindParams creates a new DeleteVolumesVolumeBindParams object
// with the default values initialized.
func NewDeleteVolumesVolumeBindParams() *DeleteVolumesVolumeBindParams {
	var ()
	return &DeleteVolumesVolumeBindParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewDeleteVolumesVolumeBindParamsWithTimeout creates a new DeleteVolumesVolumeBindParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewDeleteVolumesVolumeBindParamsWithTimeout(timeout time.Duration) *DeleteVolumesVolumeBindParams {
	var ()
	return &DeleteVolumesVolumeBindParams{

		timeout: timeout,
	}
}

// NewDeleteVolumesVolumeBindParamsWithContext creates a new DeleteVolumesVolumeBindParams object
// with the default values initialized, and the ability to set a context for a request
func NewDeleteVolumesVolumeBindParamsWithContext(ctx context.Context) *DeleteVolumesVolumeBindParams {
	var ()
	return &DeleteVolumesVolumeBindParams{

		Context: ctx,
	}
}

// NewDeleteVolumesVolumeBindParamsWithHTTPClient creates a new DeleteVolumesVolumeBindParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewDeleteVolumesVolumeBindParamsWithHTTPClient(client *http.Client) *DeleteVolumesVolumeBindParams {
	var ()
	return &DeleteVolumesVolumeBindParams{
		HTTPClient: client,
	}
}

/*DeleteVolumesVolumeBindParams contains all the parameters to send to the API endpoint
for the delete volumes volume bind operation typically these are written to a http.Request
*/
type DeleteVolumesVolumeBindParams struct {

	/*Volume
	  Volume name.

	*/
	Volume string

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) WithTimeout(timeout time.Duration) *DeleteVolumesVolumeBindParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) WithContext(ctx context.Context) *DeleteVolumesVolumeBindParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) WithHTTPClient(client *http.Client) *DeleteVolumesVolumeBindParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithVolume adds the volume to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) WithVolume(volume string) *DeleteVolumesVolumeBindParams {
	o.SetVolume(volume)
	return o
}

// SetVolume adds the volume to the delete volumes volume bind params
func (o *DeleteVolumesVolumeBindParams) SetVolume(volume string) {
	o.Volume = volume
}

// WriteToRequest writes these params to a swagger request
func (o *DeleteVolumesVolumeBindParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param volume
	if err := r.SetPathParam("volume", o.Volume); err != nil {
		return err
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
