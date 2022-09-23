package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/globalsign/mgo"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/autoscale"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	tsuruIo "github.com/tsuru/tsuru/io"
	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

// title: get autoscale config
// path: /autoscale/config
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   401: Unauthorized
func autoScaleGetConfig(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedGetConfig := permission.Check(t, permission.PermNodeAutoscaleRead)
	if !allowedGetConfig {
		return permission.ErrUnauthorized
	}
	config, err := autoscale.CurrentConfig()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(config)
}

// title: autoscale rules list
// path: /autoscale/rules
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   401: Unauthorized
func autoScaleListRules(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	allowedListRule := permission.Check(t, permission.PermNodeAutoscaleRead)
	if !allowedListRule {
		return permission.ErrUnauthorized
	}
	rules, err := autoscale.ListRules()
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(&rules)
}

// title: autoscale set rule
// path: /autoscale/rules
// method: POST
// consume: application/x-www-form-urlencoded
// responses:
//   200: Ok
//   400: Invalid data
//   401: Unauthorized
func autoScaleSetRule(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowedSetRule := permission.Check(t, permission.PermNodeAutoscaleUpdate)
	if !allowedSetRule {
		return permission.ErrUnauthorized
	}
	var rule autoscale.Rule
	err = ParseInput(r, &rule)
	if err != nil {
		return err
	}
	var ctxs []permTypes.PermissionContext
	if rule.MetadataFilter != "" {
		ctxs = append(ctxs, permission.Context(permTypes.CtxPool, rule.MetadataFilter))
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: rule.MetadataFilter},
		Kind:       permission.PermNodeAutoscaleUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	return rule.Update()
}

// title: delete autoscale rule
// path: /autoscale/rules/{id}
// method: DELETE
// responses:
//   200: Ok
//   401: Unauthorized
//   404: Not found
func autoScaleDeleteRule(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	allowedDeleteRule := permission.Check(t, permission.PermNodeAutoscale)
	if !allowedDeleteRule {
		return permission.ErrUnauthorized
	}
	rulePool := r.URL.Query().Get(":id")
	var ctxs []permTypes.PermissionContext
	if rulePool != "" {
		ctxs = append(ctxs, permission.Context(permTypes.CtxPool, rulePool))
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypePool, Value: rulePool},
		Kind:       permission.PermNodeAutoscaleDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermPoolReadEvents, ctxs...),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	err = autoscale.DeleteRule(rulePool)
	if err == mgo.ErrNotFound {
		return &tsuruErrors.HTTP{Code: http.StatusNotFound, Message: "rule not found"}
	}
	return nil
}

// title: list autoscale history
// path: /autoscale
// method: GET
// produce: application/json
// responses:
//   200: Ok
//   204: No content
//   401: Unauthorized
func autoScaleHistoryHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermNodeAutoscale) {
		return permission.ErrUnauthorized
	}
	skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	history, err := autoscale.ListAutoScaleEvents(skip, limit)
	if err != nil {
		return err
	}
	if len(history) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(&history)
}

// title: autoscale run
// path: /autoscale/run
// method: POST
// produce: application/x-json-stream
// responses:
//   200: Ok
//   401: Unauthorized
func autoScaleRunHandler(w http.ResponseWriter, r *http.Request, t auth.Token) (err error) {
	if !permission.Check(t, permission.PermNodeAutoscaleUpdateRun) {
		return permission.ErrUnauthorized
	}
	evt, err := event.New(&event.Opts{
		Target:      event.Target{Type: event.TargetTypePool},
		Kind:        permission.PermNodeAutoscaleUpdateRun,
		Owner:       t,
		RemoteAddr:  r.RemoteAddr,
		CustomData:  event.FormToCustomData(InputFields(r)),
		DisableLock: true,
		Allowed:     event.Allowed(permission.PermPoolReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	w.Header().Set("Content-Type", "application/x-json-stream")
	w.WriteHeader(http.StatusOK)
	keepAliveWriter := tsuruIo.NewKeepAliveWriter(w, 15*time.Second, "")
	defer keepAliveWriter.Stop()
	writer := &tsuruIo.SimpleJsonMessageEncoderWriter{
		Encoder: json.NewEncoder(keepAliveWriter),
	}
	return autoscale.RunOnce(writer)
}
