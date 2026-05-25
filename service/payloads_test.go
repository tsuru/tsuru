package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateServicePayloadForm(t *testing.T) {
	tests := []struct {
		name     string
		req      createServicePayload
		expected string
	}{
		{
			name:     "empty request",
			req:      createServicePayload{},
			expected: "eventid=&name=&team=&user=",
		},
		{
			name: "required fields only",
			req: createServicePayload{
				Name:    "my-service",
				Team:    "my-team",
				EventID: "event-1",
				User:    "user@example.com",
			},
			expected: "eventid=event-1&name=my-service&team=my-team&user=user%40example.com",
		},
		{
			name: "with description and plan",
			req: createServicePayload{
				Name:        "my-service",
				Team:        "my-team",
				Description: "a service",
				Plan:        "small",
				EventID:     "event-1",
				User:        "user",
			},
			expected: "description=a+service&eventid=event-1&name=my-service&plan=small&team=my-team&user=user",
		},
		{
			name: "with tags",
			req: createServicePayload{
				Name:    "my-service",
				Team:    "my-team",
				Tags:    []string{"foo", "bar"},
				EventID: "event-1",
				User:    "user",
			},
			expected: "eventid=event-1&name=my-service&tags=foo&tags=bar&team=my-team&user=user",
		},
		{
			name: "with parameters",
			req: createServicePayload{
				Name:       "my-service",
				Team:       "my-team",
				Parameters: map[string]any{"key": "value"},
				EventID:    "event-1",
				User:       "user",
			},
			expected: "eventid=event-1&name=my-service&parameters.key=value&team=my-team&user=user",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := tt.req.Form()
			assert.Equal(t, tt.expected, values.Encode())
		})
	}
}

func TestUpdateServicePayloadForm(t *testing.T) {
	payload := &updateServicePayload{
		Description: "updated",
		Team:        "my-team",
		Plan:        "large",
		Tags:        []string{"a", "b"},
		EventID:     "evt-1",
		User:        "user",
		Parameters:  map[string]any{"k": "v"},
	}
	v := payload.Form()
	assert.Equal(t, "updated", v.Get("description"))
	assert.Equal(t, "my-team", v.Get("team"))
	assert.Equal(t, "large", v.Get("plan"))
	assert.Equal(t, []string{"a", "b"}, v["tags"])
	assert.Equal(t, "evt-1", v.Get("eventid"))
	assert.Equal(t, "user", v.Get("user"))
	assert.Equal(t, "v", v.Get("parameters.k"))
}

func TestDestroyServicePayloadForm(t *testing.T) {
	payload := &destroyServicePayload{
		User:    "user",
		EventID: "evt-1",
	}
	v := payload.Form()
	assert.Equal(t, "user", v.Get("user"))
	assert.Equal(t, "evt-1", v.Get("eventid"))
}

func TestUnbindAppPayloadForm(t *testing.T) {
	payload := &unbindAppPayload{
		AppHosts: []string{"host1", "host2"},
		AppName:  "myapp",
		User:     "user",
		EventID:  "evt-1",
	}
	v := payload.Form()
	assert.Equal(t, "myapp", v.Get("app-name"))
	assert.Equal(t, "user", v.Get("user"))
	assert.Equal(t, "evt-1", v.Get("eventid"))
	assert.Equal(t, []string{"host1", "host2"}, v["app-hosts"])
	assert.Equal(t, "host1", v.Get("app-host"))
}

func TestUnbindAppPayloadFormNoHosts(t *testing.T) {
	payload := &unbindAppPayload{
		AppName: "myapp",
		User:    "user",
		EventID: "evt-1",
	}
	v := payload.Form()
	_, hasAppHost := v["app-host"]
	assert.False(t, hasAppHost)
}

func TestBindAppPayloadForm(t *testing.T) {
	payload := &bindAppPayload{
		AppName:               "myapp",
		Parameters:            map[string]any{"k": "v"},
		User:                  "user",
		EventID:               "evt-1",
		AppHosts:              []string{"host1"},
		AppInternalHosts:      []string{"internal1"},
		AppPoolName:           "pool1",
		AppPoolProvisioner:    "kubernetes",
		AppClusterName:        "cluster1",
		AppClusterProvisioner: "kubernetes",
		AppClusterAddresses:   []string{"addr1", "addr2"},
	}
	v := payload.Form()
	assert.Equal(t, "myapp", v.Get("app-name"))
	assert.Equal(t, "v", v.Get("parameters.k"))
	assert.Equal(t, "user", v.Get("user"))
	assert.Equal(t, "evt-1", v.Get("eventid"))
	assert.Equal(t, []string{"host1"}, v["app-hosts"])
	assert.Equal(t, "host1", v.Get("app-host"))
	assert.Equal(t, []string{"internal1"}, v["app-internal-hosts"])
	assert.Equal(t, "pool1", v.Get("app-pool-name"))
	assert.Equal(t, "kubernetes", v.Get("app-pool-provisioner"))
	assert.Equal(t, "cluster1", v.Get("app-cluster-name"))
	assert.Equal(t, "kubernetes", v.Get("app-cluster-provisioner"))
	assert.Equal(t, []string{"addr1", "addr2"}, v["app-cluster-addresses"])
}

func TestBindJobPayloadForm(t *testing.T) {
	payload := &bindJobPayload{
		JobName:        "myjob",
		User:           "user",
		EventID:        "evt-1",
		JobPoolName:    "pool1",
		JobClusterName: "cluster1",
	}
	v := payload.Form()
	assert.Equal(t, "myjob", v.Get("job-name"))
	assert.Equal(t, "user", v.Get("user"))
	assert.Equal(t, "evt-1", v.Get("eventid"))
	assert.Equal(t, "pool1", v.Get("job-pool-name"))
	assert.Equal(t, "cluster1", v.Get("job-cluster-name"))
}

func TestUnbindJobPayloadForm(t *testing.T) {
	payload := &unbindJobPayload{
		User:    "user",
		EventID: "evt-1",
	}
	v := payload.Form()
	assert.Equal(t, "user", v.Get("user"))
	assert.Equal(t, "evt-1", v.Get("eventid"))
}

func TestCreateServicePayloadJSON(t *testing.T) {
	payload := &createServicePayload{
		Name:        "my-service",
		Team:        "my-team",
		Description: "a service",
		Plan:        "small",
		Tags:        []string{"foo", "bar"},
		EventID:     "event-1",
		User:        "user@example.com",
		Parameters:  map[string]any{"key": "value"},
	}
	data, err := json.Marshal(payload)
	assert.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)
	assert.Equal(t, "my-service", result["name"])
	assert.Equal(t, "my-team", result["team"])
	assert.Equal(t, "a service", result["description"])
	assert.Equal(t, "small", result["plan"])
	assert.Equal(t, []any{"foo", "bar"}, result["tags"])
	assert.Equal(t, "event-1", result["eventID"])
	assert.Equal(t, "user@example.com", result["user"])
	assert.Equal(t, map[string]any{"key": "value"}, result["parameters"])
}

func TestCreateServicePayloadJSONOmitsEmpty(t *testing.T) {
	payload := &createServicePayload{
		Name:    "my-service",
		Team:    "my-team",
		EventID: "event-1",
		User:    "user",
	}
	data, err := json.Marshal(payload)
	assert.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)
	_, hasDescription := result["description"]
	_, hasPlan := result["plan"]
	_, hasTags := result["tags"]
	_, hasParameters := result["parameters"]
	assert.False(t, hasDescription)
	assert.False(t, hasPlan)
	assert.False(t, hasTags)
	assert.False(t, hasParameters)
}

func TestBindAppPayloadJSON(t *testing.T) {
	payload := &bindAppPayload{
		AppName:               "myapp",
		Parameters:            map[string]any{"p1": "v1"},
		User:                  "user",
		EventID:               "evt-1",
		AppHosts:              []string{"host1"},
		AppInternalHosts:      []string{"internal1"},
		AppPoolName:           "pool1",
		AppPoolProvisioner:    "kubernetes",
		AppClusterName:        "cluster1",
		AppClusterProvisioner: "kubernetes",
		AppClusterAddresses:   []string{"addr1"},
	}
	data, err := json.Marshal(payload)
	assert.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)
	assert.Equal(t, "myapp", result["appName"])
	assert.Equal(t, map[string]any{"p1": "v1"}, result["parameters"])
	assert.Equal(t, "user", result["user"])
	assert.Equal(t, "evt-1", result["eventID"])
	assert.Equal(t, "pool1", result["appPoolName"])
	assert.Equal(t, "kubernetes", result["appPoolProvisioner"])
	assert.Equal(t, "cluster1", result["appClusterName"])
	assert.Equal(t, "kubernetes", result["appClusterProvisioner"])
}
