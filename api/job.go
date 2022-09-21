// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

// title: job create
// path: /jobs
// method: POST
// consume: application/x-www-form-urlencoded
// produce: application/json
// responses:
//
//	201: Job created
//	400: Invalid data
//	401: Unauthorized
//	403: Quota exceeded
//	409: Job already exists
// func createJob(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
// 	ctx := r.Context()
// 	var ia inputApp
// 	err = ParseInput(r, &ia)
// 	if err != nil {
// 		return err
// 	}
// 	a := app.App{
// 		TeamOwner:   ia.TeamOwner,
// 		Platform:    ia.Platform,
// 		Plan:        appTypes.Plan{Name: ia.Plan},
// 		Name:        ia.Name,
// 		Description: ia.Description,
// 		Pool:        ia.Pool,
// 		RouterOpts:  ia.RouterOpts,
// 		Router:      ia.Router,
// 		Tags:        ia.Tags,
// 		Metadata:    ia.Metadata,
// 		Quota:       quota.UnlimitedQuota,
// 	}
// 	tags, _ := InputValues(r, "tag")
// 	a.Tags = append(a.Tags, tags...) // for compatibility
// 	if a.TeamOwner == "" {
// 		a.TeamOwner, err = autoTeamOwner(ctx, t, permission.PermAppCreate)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	canCreate := permission.Check(t, permission.PermAppCreate,
// 		permission.Context(permTypes.CtxTeam, a.TeamOwner),
// 	)
// 	if !canCreate {
// 		return permission.ErrUnauthorized
// 	}
// 	u, err := auth.ConvertNewUser(t.User())
// 	if err != nil {
// 		return err
// 	}
// 	if a.Platform != "" {
// 		repo, _ := image.SplitImageName(a.Platform)
// 		platform, errPlat := servicemanager.Platform.FindByName(ctx, repo)
// 		if errPlat != nil {
// 			return errPlat
// 		}
// 		if platform.Disabled {
// 			canUsePlat := permission.Check(t, permission.PermPlatformUpdate) ||
// 				permission.Check(t, permission.PermPlatformCreate)
// 			if !canUsePlat {
// 				return &errors.HTTP{Code: http.StatusBadRequest, Message: appTypes.ErrInvalidPlatform.Error()}
// 			}
// 		}
// 	}
// 	evt, err := event.New(&event.Opts{
// 		Target:     appTarget(a.Name),
// 		Kind:       permission.PermAppCreate,
// 		Owner:      t,
// 		CustomData: event.FormToCustomData(InputFields(r)),
// 		Allowed:    event.Allowed(permission.PermAppReadEvents, contextsForApp(&a)...),
// 	})
// 	if err != nil {
// 		return err
// 	}
// 	defer func() { evt.Done(err) }()
// 	err = app.CreateApp(ctx, &a, u)
// 	if err != nil {
// 		log.Errorf("Got error while creating app: %s", err)
// 		if _, ok := err.(appTypes.NoTeamsError); ok {
// 			return &errors.HTTP{
// 				Code:    http.StatusBadRequest,
// 				Message: "In order to create an app, you should be member of at least one team",
// 			}
// 		}
// 		if e, ok := err.(*appTypes.AppCreationError); ok {
// 			if e.Err == app.ErrAppAlreadyExists {
// 				return &errors.HTTP{Code: http.StatusConflict, Message: e.Error()}
// 			}
// 			if _, ok := pkgErrors.Cause(e.Err).(*quota.QuotaExceededError); ok {
// 				return &errors.HTTP{
// 					Code:    http.StatusForbidden,
// 					Message: "Quota exceeded",
// 				}
// 			}
// 		}
// 		if err == appTypes.ErrInvalidPlatform {
// 			return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
// 		}
// 		return err
// 	}
// 	msg := map[string]interface{}{
// 		"status": "success",
// 	}
// 	addrs, err := a.GetAddresses()
// 	if err != nil {
// 		return err
// 	}
// 	if len(addrs) > 0 {
// 		msg["ip"] = addrs[0]
// 	}
// 	jsonMsg, err := json.Marshal(msg)
// 	if err != nil {
// 		return err
// 	}
// 	w.Header().Set("Content-Type", "application/json")
// 	w.WriteHeader(http.StatusCreated)
// 	w.Write(jsonMsg)
// 	return nil
// }
