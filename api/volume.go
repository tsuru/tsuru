// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision/pool"
	"github.com/tsuru/tsuru/servicemanager"
	eventTypes "github.com/tsuru/tsuru/types/event"
	permTypes "github.com/tsuru/tsuru/types/permission"
	volumeTypes "github.com/tsuru/tsuru/types/volume"
)

func volumeFilterByContext(contexts []permTypes.PermissionContext) *volumeTypes.Filter {
	filter := &volumeTypes.Filter{}
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

func contextsForVolume(v *volumeTypes.Volume) []permTypes.PermissionContext {
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
//
//	200: List volumes
//	204: No content
//	401: Unauthorized
func volumesList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	contexts := permission.ContextsForPermission(ctx, t, permission.PermVolumeRead)
	if len(contexts) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	volumes, err := servicemanager.Volume.ListByFilter(ctx, volumeFilterByContext(contexts))
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
//
//	200: Show volume
//	401: Unauthorized
//	404: Volume not found
func volumeInfo(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	v, err := servicemanager.Volume.Get(ctx, r.URL.Query().Get(":name"))
	if err != nil {
		if err == volumeTypes.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canRead := permission.Check(ctx, t, permission.PermVolumeRead, contextsForVolume(v)...)
	if !canRead {
		return permission.ErrUnauthorized
	}
	v.Binds, err = servicemanager.Volume.Binds(ctx, v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&v)
}

// title: volume create
// path: /volumes
// method: POST
// produce: application/json
// responses:
//
//	201: Volume created
//	401: Unauthorized
//	409: Volume already exists
func volumeCreate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var inputVolume volumeTypes.Volume
	err = ParseInput(r, &inputVolume)
	if err != nil {
		return err
	}
	inputVolume.Plan.Opts = nil
	inputVolume.Status = ""
	canCreate := permission.Check(ctx, t, permission.PermVolumeCreate,
		permission.Context(permTypes.CtxTeam, inputVolume.TeamOwner),
		permission.Context(permTypes.CtxPool, inputVolume.Pool),
	)
	if !canCreate {
		return permission.ErrUnauthorized
	}

	err = servicemanager.Volume.CheckPoolVolumeConstraints(ctx, inputVolume)
	if err == volumeTypes.ErrVolumePlanNotFound || err == pool.ErrPoolHasNoVolumePlan {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	if err != nil {
		return err
	}

	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeVolume, Value: inputVolume.Name},
		Kind:       permission.PermVolumeCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(&inputVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	_, err = servicemanager.Volume.Get(ctx, inputVolume.Name)
	if err == nil {
		return &errors.HTTP{Code: http.StatusConflict, Message: "volume already exists"}
	}
	err = servicemanager.Volume.Create(ctx, &inputVolume)
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
//
//	200: Volume updated
//	401: Unauthorized
//	404: Volume not found
func volumeUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var inputVolume volumeTypes.Volume
	err = ParseInput(r, &inputVolume)
	if err != nil {
		return err
	}
	inputVolume.Plan.Opts = nil
	inputVolume.Status = ""
	inputVolume.Name = r.URL.Query().Get(":name")
	dbVolume, err := servicemanager.Volume.Get(ctx, inputVolume.Name)
	if err != nil {
		if err == volumeTypes.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canUpdate := permission.Check(ctx, t, permission.PermVolumeUpdate, contextsForVolume(dbVolume)...)
	if !canUpdate {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeVolume, Value: inputVolume.Name},
		Kind:       permission.PermVolumeUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	return servicemanager.Volume.Update(ctx, &inputVolume)
}

// title: volume plan list
// path: /volumeplans
// method: GET
// produce: application/json
// responses:
//
//	200: List volume plans
//	401: Unauthorized
func volumePlansList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	contexts := permission.ContextsForPermission(ctx, t, permission.PermVolumeCreate)
	if len(contexts) == 0 {
		return permission.ErrUnauthorized
	}
	plansProvisioners, err := servicemanager.Volume.ListPlans(ctx)
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
//
//	200: Volume deleted
//	401: Unauthorized
//	404: Volume not found
func volumeDelete(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	volumeName := r.URL.Query().Get(":name")
	dbVolume, err := servicemanager.Volume.Get(ctx, volumeName)
	if err != nil {
		if err == volumeTypes.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canDelete := permission.Check(ctx, t, permission.PermVolumeDelete, contextsForVolume(dbVolume)...)
	if !canDelete {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeVolume, Value: volumeName},
		Kind:       permission.PermVolumeDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	return servicemanager.Volume.Delete(ctx, dbVolume)
}

// title: volume bind
// path: /volumes/{name}/bind
// method: POST
// produce: application/json
// responses:
//
//	200: Volume binded
//	401: Unauthorized
//	404: Volume not found
//	409: Volume bind already exists
func volumeBind(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var bindInfo struct {
		App        string
		MountPoint string
		ReadOnly   bool
		NoRestart  bool
	}
	err = ParseInput(r, &bindInfo)
	if err != nil {
		return err
	}
	dbVolume, err := servicemanager.Volume.Get(ctx, r.URL.Query().Get(":name"))
	if err != nil {
		if err == volumeTypes.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canBindVolume := permission.Check(ctx, t, permission.PermVolumeUpdateBind, contextsForVolume(dbVolume)...)
	if !canBindVolume {
		return permission.ErrUnauthorized
	}
	a, err := getAppFromContext(bindInfo.App, r)
	if err != nil {
		return err
	}
	canBindApp := permission.Check(ctx, t, permission.PermAppUpdateBindVolume, contextsForApp(a)...)
	if !canBindApp {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeVolume, Value: dbVolume.Name},
		Kind:       permission.PermVolumeUpdateBind,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = servicemanager.Volume.BindApp(ctx, &volumeTypes.BindOpts{
		Volume:     dbVolume,
		AppName:    bindInfo.App,
		MountPoint: bindInfo.MountPoint,
		ReadOnly:   bindInfo.ReadOnly,
	})
	if err != nil || bindInfo.NoRestart {
		if err == volumeTypes.ErrVolumeAlreadyBound {
			return &errors.HTTP{Code: http.StatusConflict, Message: err.Error()}
		}
		return err
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	return app.Restart(ctx, a, "", "", evt)
}

// title: volume unbind
// path: /volumes/{name}/bind
// method: DELETE
// produce: application/json
// responses:
//
//	200: Volume unbinded
//	401: Unauthorized
//	404: Volume not found
func volumeUnbind(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	ctx := r.Context()
	var bindInfo struct {
		App        string
		MountPoint string
		NoRestart  bool
	}
	err = ParseInput(r, &bindInfo)
	if err != nil {
		return err
	}
	dbVolume, err := servicemanager.Volume.Get(ctx, r.URL.Query().Get(":name"))
	if err != nil {
		if err == volumeTypes.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	canUnbind := permission.Check(ctx, t, permission.PermVolumeUpdateUnbind, contextsForVolume(dbVolume)...)
	if !canUnbind {
		return permission.ErrUnauthorized
	}
	a, err := getAppFromContext(bindInfo.App, r)
	if err != nil {
		return err
	}
	canUnbindApp := permission.Check(ctx, t, permission.PermAppUpdateUnbindVolume, contextsForApp(a)...)
	if !canUnbindApp {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeVolume, Value: dbVolume.Name},
		Kind:       permission.PermVolumeUpdateUnbind,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(dbVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	err = servicemanager.Volume.UnbindApp(ctx, &volumeTypes.BindOpts{
		Volume:     dbVolume,
		AppName:    bindInfo.App,
		MountPoint: bindInfo.MountPoint,
	})
	if err != nil || bindInfo.NoRestart {
		if err == volumeTypes.ErrVolumeBindNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	w.Header().Set("Content-Type", "application/x-json-stream")
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 30*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{Encoder: json.NewEncoder(keepAliveWriter)}
	evt.SetLogWriter(writer)
	return app.Restart(ctx, a, "", "", evt)
}
