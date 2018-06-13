package api

import (
	"encoding/json"
	"net/http"

	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/servicemanager"
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
