// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
	"github.com/tsuru/tsuru/volume"
)

func volumeFilterByContext(contexts []permTypes.PermissionContext) *volume.Filter {
	filter := &volume.Filter{}
contextsLoop:
	for _, c := range contexts {
		switch c.CtxType {
		case permTypes.CtxGlobal:
			filter = nil
			break contextsLoop
		case permTypes.CtxTeam:
			filter.Teams = append(filter.Teams, c.Value)
		case permTypes.CtxVolume:
			filter.Names = append(filter.Names, c.Value)
		case permTypes.CtxPool:
			filter.Pools = append(filter.Pools, c.Value)
		}
	}
	return filter
}

func contextsForVolume(v *volume.Volume) []permTypes.PermissionContext {
	return []permTypes.PermissionContext{
		permission.Context(permTypes.CtxVolume, v.Name),
		permission.Context(permTypes.CtxTeam, v.TeamOwner),
		permission.Context(permTypes.CtxPool, v.Pool),
	}
}

// title: volume list
// path: /volumes
// method: GET
// produce: application/json
// responses:
//   200: List volumes
//   204: No content
//   401: Unauthorized
func volumesList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	contexts := permission.ContextsForPermission(t, permission.PermVolumeRead)
	if len(contexts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	volumes, err := volume.ListByFilter(volumeFilterByContext(contexts))
	if err != nil {
		return err
	}
	if len(volumes) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(volumes)
}

// title: volume info
// path: /volumes/{name}
// method: GET
// produce: application/json
// responses:
//   200: Show volume
//   401: Unauthorized
//   404: Volume not found
func volumeInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	v, err := volume.Load(r.URL.Query().Get(":name"))
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canRead := permission.Check(t, permission.PermVolumeRead, contextsForVolume(v)...)
	if !canRead {
		return permission.ErrUnauthorized
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&v)
}

// title: volume create
// path: /volumes
// method: POST
// produce: application/json
// responses:
//   201: Volume created
//   401: Unauthorized
//   409: Volume already exists
func volumeCreate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var inputVolume volume.Volume
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&inputVolume, r.Form)
	inputVolume.Plan.Opts = nil
	inputVolume.Status = ""
	canCreate := permission.Check(t, permission.PermVolumeCreate,
		permission.Context(permTypes.CtxTeam, inputVolume.TeamOwner),
		permission.Context(permTypes.CtxPool, inputVolume.Pool),
	)
	if !canCreate {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeVolume, Value: inputVolume.Name},
		Kind:       permission.PermVolumeCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(&inputVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	_, err = volume.Load(inputVolume.Name)
	if err == nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: "volume already exists"}
	}
	err = inputVolume.Create()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

// title: volume update
// path: /volumes/{name}
// method: POST
// produce: application/json
// responses:
//   200: Volume updated
//   401: Unauthorized
//   404: Volume not found
func volumeUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	err = r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var inputVolume volume.Volume
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&inputVolume, r.Form)
	inputVolume.Plan.Opts = nil
	inputVolume.Status = ""
	inputVolume.Name = r.URL.Query().Get(":name")
	dbVolume, err := volume.Load(inputVolume.Name)
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canUpdate := permission.Check(t, permission.PermVolumeUpdate, contextsForVolume(dbVolume)...)
	if !canUpdate {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeVolume, Value: inputVolume.Name},
		Kind:       permission.PermVolumeUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return inputVolume.Update()
}

// title: volume plan list
// path: /volumeplans
// method: GET
// produce: application/json
// responses:
//   200: List volume plans
//   401: Unauthorized
func volumePlansList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	contexts := permission.ContextsForPermission(t, permission.PermVolumeCreate)
	if len(contexts) == 0 {
		return permission.ErrUnauthorized
	}
	plansProvisioners, err := volume.ListPlans()
	if err != nil {
		return err
	}
	if len(plansProvisioners) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(plansProvisioners)
}

// title: volume delete
// path: /volumes/{name}
// method: DELETE
// produce: application/json
// responses:
//   200: Volume deleted
//   401: Unauthorized
//   404: Volume not found
func volumeDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	volumeName := r.URL.Query().Get(":name")
	dbVolume, err := volume.Load(volumeName)
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canDelete := permission.Check(t, permission.PermVolumeDelete, contextsForVolume(dbVolume)...)
	if !canDelete {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeVolume, Value: volumeName},
		Kind:       permission.PermVolumeDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return dbVolume.Delete()
}

// title: volume bind
// path: /volumes/{name}/bind
// method: POST
// produce: application/json
// responses:
//   200: Volume binded
//   401: Unauthorized
//   404: Volume not found
//   409: Volume bind already exists
func volumeBind(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var bindInfo struct {
		App        string
		MountPoint string
		ReadOnly   bool
		NoRestart  bool
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&bindInfo, r.Form)
	dbVolume, err := volume.Load(r.URL.Query().Get(":name"))
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canBindVolume := permission.Check(t, permission.PermVolumeUpdateBind, contextsForVolume(dbVolume)...)
	if !canBindVolume {
		return permission.ErrUnauthorized
	}
	a, err := getAppFromContext(bindInfo.App, r)
	if err != nil {
		return err
	}
	canBindApp := permission.Check(t, permission.PermAppUpdateBindVolume, contextsForApp(&a)...)
	if !canBindApp {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeVolume, Value: dbVolume.Name},
		Kind:       permission.PermVolumeUpdateBind,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = dbVolume.BindApp(bindInfo.App, bindInfo.MountPoint, bindInfo.ReadOnly)
	if err != nil || bindInfo.NoRestart {
		if err == volume.ErrVolumeAlreadyBound {
			return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
		}
		return err
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.Restart("", writer)
}

// title: volume unbind
// path: /volumes/{name}/bind
// method: DELETE
// produce: application/json
// responses:
//   200: Volume unbinded
//   401: Unauthorized
//   404: Volume not found
func volumeUnbind(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	err := r.ParseForm()
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	var bindInfo struct {
		App        string
		MountPoint string
		NoRestart  bool
	}
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	dec.DecodeValues(&bindInfo, r.Form)
	dbVolume, err := volume.Load(r.URL.Query().Get(":name"))
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canUnbind := permission.Check(t, permission.PermVolumeUpdateUnbind, contextsForVolume(dbVolume)...)
	if !canUnbind {
		return permission.ErrUnauthorized
	}
	a, err := getAppFromContext(bindInfo.App, r)
	if err != nil {
		return err
	}
	canUnbindApp := permission.Check(t, permission.PermAppUpdateUnbindVolume, contextsForApp(&a)...)
	if !canUnbindApp {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeVolume, Value: dbVolume.Name},
		Kind:       permission.PermVolumeUpdateUnbind,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = dbVolume.UnbindApp(bindInfo.App, bindInfo.MountPoint)
	if err != nil || bindInfo.NoRestart {
		if err == volume.ErrVolumeBindNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	return a.Restart("", writer)
}
