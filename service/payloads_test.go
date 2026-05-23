package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormEncode(t *testing.T) {
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
