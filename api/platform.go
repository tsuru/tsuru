// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/auth"
	tErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	appTypes "github.com/tsuru/tsuru/types/app"
)

// title: add platform
// path: /platforms
// method: POST
// consume: multipart/form-data
// produce: application/x-json-stream
// responses:
//   200: Platform created
//   400: Invalid data
//   401: Unauthorized
func platformAdd(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	name := InputValue(r, "name")
	file, _, err := r.FormFile("dockerfile_content")
	if err != nil {
		return &tErrors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return &tErrors.HTTP{Code: http.StatusBadRequest, Message: appTypes.ErrMissingFileContent.Error()}
	}
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	canCreatePlatform := permission.Check(t, permission.PermPlatformCreate)
	if !canCreatePlatform {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePlatform, Value: name},
		Kind:       permission.PermPlatformCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlatformReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	evt.SetLogWriter(writer)
	ctx, cancel := evt.CancelableContext(context.Background())
	err = servicemanager.Platform.Create(appTypes.PlatformOptions{
		Name:   name,
		Args:   args,
		Data:   data,
		Output: evt,
		Ctx:    ctx,
	})
	cancel()
	if err != nil {
		return err
	}
	writer.Write([]byte("Platform successfully added!\n"))
	return nil
}

// title: update platform
// path: /platforms/{name}
// method: PUT
// produce: application/x-json-stream
// responses:
//   200: Platform updated
//   401: Unauthorized
//   404: Not found
func platformUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	name := r.URL.Query().Get(":name")
	file, _, _ := r.FormFile("dockerfile_content")
	if file != nil {
		defer file.Close()
	}
	args := make(map[string]string)
	for key, values := range r.Form {
		args[key] = values[0]
	}
	canUpdatePlatform := permission.Check(t, permission.PermPlatformUpdate)
	if !canUpdatePlatform {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePlatform, Value: name},
		Kind:       permission.PermPlatformUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlatformReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	evt.SetLogWriter(writer)
	ctx, cancel := evt.CancelableContext(context.Background())
	err = servicemanager.Platform.Update(appTypes.PlatformOptions{
		Name:   name,
		Args:   args,
		Input:  file,
		Output: evt,
		Ctx:    ctx,
	})
	cancel()
	if err == appTypes.ErrPlatformNotFound {
		return &tErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err != nil {
		return err
	}
	writer.Write([]byte("Platform successfully updated!\n"))
	return nil
}

// title: remove platform
// path: /platforms/{name}
// method: DELETE
// responses:
//   200: Platform removed
//   401: Unauthorized
//   404: Not found
func platformRemove(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	canDeletePlatform := permission.Check(t, permission.PermPlatformDelete)
	if !canDeletePlatform {
		return permission.ErrUnauthorized
	}
	name := r.URL.Query().Get(":name")
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePlatform, Value: name},
		Kind:       permission.PermPlatformDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlatformReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = servicemanager.Platform.Remove(name)
	if err == appTypes.ErrPlatformNotFound {
		return &tErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	return err
}

// title: platform list
// path: /platforms
// method: GET
// produce: application/json
// responses:
//   200: List platforms
//   204: No content
//   401: Unauthorized
func platformList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
		permission.Check(t, permission.PermPlatformCreate)
	platforms, err := servicemanager.Platform.List(!canUsePlat)
	if err != nil {
		return err
	}
	if len(platforms) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(platforms)
}

// title: platform info
// path: /platforms/{name}
// method: GET
// produce: application/json
// responses:
//   200: Platform info
//   401: Unauthorized
//   404: NotFound
func platformInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	name := r.URL.Query().Get(":name")
	canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
		permission.Check(t, permission.PermPlatformRead)
	if !canUsePlat {
		return permission.ErrUnauthorized
	}
	platform, err := servicemanager.Platform.FindByName(name)
	if err == appTypes.ErrPlatformNotFound {
		return &tErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err != nil {
		return err
	}
	images, err := servicemanager.PlatformImage.ListImagesOrDefault(name)
	if err != nil {
		return err
	}
	msg := map[string]interface{}{
		"platform": platform,
		"images":   images,
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(msg)
}

// title: rollback platform
// path: /platforms/{name}/rollback
// method: POST
// produce: application/x-json-stream
// responses:
//   200: OK
//   400: BadRequest
//   401: Unauthorized
//   404: Not found
func platformRollback(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	name := r.URL.Query().Get(":name")
	image := InputValue(r, "image")
	if image == "" {
		return &tErrors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "you cannot rollback without an image name",
		}
	}
	canUpdatePlatform := permission.Check(t, permission.PermPlatformUpdate)
	if !canUpdatePlatform {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := io.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &io.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePlatform, Value: name},
		Kind:       permission.PermPlatformUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPlatformReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	evt.SetLogWriter(writer)
	ctx, cancel := evt.CancelableContext(context.Background())
	err = servicemanager.Platform.Rollback(appTypes.PlatformOptions{
		Name:      name,
		ImageName: image,
		Output:    evt,
		Ctx:       ctx,
	})
	cancel()
	if err == appTypes.ErrPlatformNotFound {
		return &tErrors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
	}
	if err != nil {
		return err
	}
	writer.Write([]byte("Platform successfully updated!\n"))
	return nil
}
