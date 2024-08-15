package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	eventTypes "github.com/tsuru/tsuru/types/event"
	"github.com/tsuru/tsuru/types/service"
)

// title: service broker list
// path: /brokers
// method: GET
// produce: application/json
// responses:
//
//	200: List service brokers
//	204: No content
//	401: Unauthorized
func serviceBrokerList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermServiceBrokerRead) {
		return permission.ErrUnauthorized
	}
	brokers, err := servicemanager.ServiceBroker.List(ctx)
	if err != nil {
		return err
	}
	if len(brokers) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return json.NewEncoder(w).Encode(map[string]interface{}{
		"brokers": brokers,
	})
}

// title: Add service broker
// path: /brokers
// method: POST
// responses:
//
//	201: Service broker created
//	401: Unauthorized
//	409: Broker already exists
func serviceBrokerAdd(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermServiceBrokerCreate) {
		return permission.ErrUnauthorized
	}
	broker, err := decodeServiceBroker(r)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeServiceBroker, Value: broker.Name},
		Kind:       permission.PermServiceBrokerCreate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceBrokerReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	if err = servicemanager.ServiceBroker.Create(ctx, *broker); err != nil {
		if err == service.ErrServiceBrokerAlreadyExists {
			return &errors.HTTP{Code: http.StatusConflict, Message: "Broker already exists."}
		}
		return err
	}
	w.WriteHeader(http.StatusCreated)
	return nil
}

// title: Update service broker
// path: /brokers/{broker}
// method: PUT
// responses:
//
//	200: Service broker updated
//	401: Unauthorized
//	404: Not Found
func serviceBrokerUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermServiceBrokerUpdate) {
		return permission.ErrUnauthorized
	}
	brokerName := r.URL.Query().Get(":broker")
	if brokerName == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Empty broker name."}
	}
	broker, err := decodeServiceBroker(r)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeServiceBroker, Value: broker.Name},
		Kind:       permission.PermServiceBrokerUpdate,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceBrokerReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	if err = servicemanager.ServiceBroker.Update(ctx, brokerName, *broker); err == service.ErrServiceBrokerNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

// title: Delete service broker
// path: /brokers/{broker}
// method: DELETE
// responses:
//
//	200: Service broker deleted
//	401: Unauthorized
//	404: Not Found
func serviceBrokerDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	ctx := r.Context()
	if !permission.Check(ctx, t, permission.PermServiceBrokerDelete) {
		return permission.ErrUnauthorized
	}
	brokerName := r.URL.Query().Get(":broker")
	if brokerName == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Empty broker name."}
	}
	evt, err := event.New(ctx, &event.Opts{
		Target:     eventTypes.Target{Type: eventTypes.TargetTypeServiceBroker, Value: brokerName},
		Kind:       permission.PermServiceBrokerDelete,
		Owner:      t,
		RemoteAddr: r.RemoteAddr,
		CustomData: event.FormToCustomData(InputFields(r)),
		Allowed:    event.Allowed(permission.PermServiceBrokerReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(ctx, err) }()
	if err = servicemanager.ServiceBroker.Delete(ctx, brokerName); err == service.ErrServiceBrokerNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

func decodeServiceBroker(request *http.Request) (*service.Broker, error) {
	var broker service.Broker
	if err := ParseInput(request, &broker); err != nil {
		return nil, fmt.Errorf("unable to parse broker: %v", err)
	}
	return &broker, nil
}
