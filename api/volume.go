// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/volume"
)

func volumeFilterByContext(contexts []permission.PermissionContext) *volume.Filter {
	filter := &volume.Filter{}
contextsLoop:
	for _, c := range contexts {
		switch c.CtxType {
		case permission.CtxGlobal:
			filter = nil
			break contextsLoop
		case permission.CtxTeam:
			filter.Teams = append(filter.Teams, c.Value)
		case permission.CtxVolume:
			filter.Names = append(filter.Names, c.Value)
		case permission.CtxPool:
			filter.Pools = append(filter.Pools, c.Value)
		}
	}
	return filter
}

func contextsForVolume(v *volume.Volume) []permission.PermissionContext {
	return []permission.PermissionContext{
		permission.Context(permission.CtxVolume, v.Name),
		permission.Context(permission.CtxTeam, v.TeamOwner),
		permission.Context(permission.CtxPool, v.Pool),
	}
}

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
		permission.Context(permission.CtxTeam, inputVolume.TeamOwner),
		permission.Context(permission.CtxPool, inputVolume.Pool),
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
	err = inputVolume.Save()
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

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
	canUpdate := permission.Check(t, permission.PermVolumeCreate, contextsForVolume(&inputVolume)...)
	if !canUpdate {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeVolume, Value: inputVolume.Name},
		Kind:       permission.PermVolumeUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermVolumeReadEvents, contextsForVolume(&inputVolume)...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	_, err = volume.Load(inputVolume.Name)
	if err != nil {
		if err == volume.ErrVolumeNotFound {
			return &errors.HTTP{Code: http.StatusNotFound, Message: err.Error()}
		}
		return err
	}
	return inputVolume.Save()
}

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
