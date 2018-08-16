package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
	"github.com/tsuru/tsuru/types/service"
)

// title: service broker list
// path: /brokers
// method: GET
// produce: application/json
// responses:
//   200: List service brokers
//   204: No content
//   401: Unauthorized
func serviceBrokerList(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermServiceBrokerRead) {
		return permission.ErrUnauthorized
	}
	brokers, err := servicemanager.ServiceBroker.List()
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
//   201: Service broker created
//   401: Unauthorized
//   409: Broker already exists
func serviceBrokerAdd(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermServiceBrokerCreate) {
		return permission.ErrUnauthorized
	}
	broker, err := decodeServiceBroker(r)
	if err != nil {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: err.Error()}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeServiceBroker, Value: broker.Name},
		Kind:       permission.PermServiceBrokerCreate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermServiceBrokerReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if err = servicemanager.ServiceBroker.Create(*broker); err != nil {
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
//   200: Service broker updated
//   401: Unauthorized
//	 404: Not Found
func serviceBrokerUpdate(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermServiceBrokerUpdate) {
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
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeServiceBroker, Value: broker.Name},
		Kind:       permission.PermServiceBrokerUpdate,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermServiceBrokerReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if err = servicemanager.ServiceBroker.Update(brokerName, *broker); err == service.ErrServiceBrokerNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

// title: Delete service broker
// path: /brokers/{broker}
// method: DELETE
// responses:
//   200: Service broker deleted
//   401: Unauthorized
//	 404: Not Found
func serviceBrokerDelete(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	if !permission.Check(t, permission.PermServiceBrokerDelete) {
		return permission.ErrUnauthorized
	}
	brokerName := r.URL.Query().Get(":broker")
	if brokerName == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Empty broker name."}
	}
	evt, err := event.New(&event.Opts{
		Target:     event.Target{Type: event.TargetTypeServiceBroker, Value: brokerName},
		Kind:       permission.PermServiceBrokerDelete,
		Owner:      t,
		CustomData: event.FormToCustomData(r.Form),
		Allowed:    event.Allowed(permission.PermServiceBrokerReadEvents),
	})
	if err != nil {
		return err
	}
	defer func() { evt.Done(err) }()
	if err = servicemanager.ServiceBroker.Delete(brokerName); err == service.ErrServiceBrokerNotFound {
		w.WriteHeader(http.StatusNotFound)
	}
	return err
}

func decodeServiceBroker(request *http.Request) (*service.Broker, error) {
	var broker service.Broker
	dec := form.NewDecoder(nil)
	dec.IgnoreCase(true)
	dec.IgnoreUnknownKeys(true)
	if err := request.ParseForm(); err != nil {
		return nil, fmt.Errorf("unable to parse form: %v", err)
	}
	if err := dec.DecodeValues(&broker, request.Form); err != nil {
		return nil, fmt.Errorf("unable to parse broker: %v", err)
	}
	cacheStr := request.FormValue("Config.CacheExpiration")
	if len(cacheStr) > 0 {
		cache, err := time.ParseDuration(cacheStr)
		if err != nil {
			return nil, fmt.Errorf("unable to parse cache expiration: %v", err)
		}
		broker.Config.CacheExpiration = &cache
	} else {
		broker.Config.CacheExpiration = nil
	}
	return &broker, nil
}
