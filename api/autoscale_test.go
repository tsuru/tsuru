package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/ajg/form"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/autoscale"
	"github.com/tsuru/tsuru/event"
	"github.com/tsuru/tsuru/event/eventtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/provision"
	check "gopkg.in/check.v1"
)

func (s *S) TestAutoScaleHistoryNoContent(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/node/autoscale", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent)
}

func (s *S) TestAutoScaleHistoryHandler(c *check.C) {
	evt1, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: provision.PoolMetadataName, Value: "poolx"},
		InternalKind: autoscale.EventKind,
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt1.Logf("my evt1")
	err = evt1.DoneCustomData(nil, autoscale.EventCustomData{
		Result: &autoscale.ScalerResult{ToAdd: 1, Reason: "r1"},
	})
	c.Assert(err, check.IsNil)
	time.Sleep(100 * time.Millisecond)
	evt2, err := event.NewInternal(&event.Opts{
		Target:       event.Target{Type: provision.PoolMetadataName, Value: "pooly"},
		InternalKind: autoscale.EventKind,
		Allowed:      event.Allowed(permission.PermPool),
	})
	c.Assert(err, check.IsNil)
	evt2.Logf("my evt2")
	err = evt2.DoneCustomData(nil, autoscale.EventCustomData{
		Result: &autoscale.ScalerResult{ToRebalance: true, Reason: "r2"},
	})
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/node/autoscale", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	body := recorder.Body.Bytes()
	history := []autoscale.Event{}
	err = json.Unmarshal(body, &history)
	c.Assert(err, check.IsNil)
	c.Assert(history, check.HasLen, 2)
	c.Assert(evt1.StartTime.Sub(history[1].StartTime) < time.Second, check.Equals, true)
	c.Assert(evt2.StartTime.Sub(history[0].StartTime) < time.Second, check.Equals, true)
	c.Assert(history[1].MetadataValue, check.Equals, "poolx")
	c.Assert(history[0].MetadataValue, check.Equals, "pooly")
	c.Assert(history[1].Action, check.Equals, "add")
	c.Assert(history[0].Action, check.Equals, "rebalance")
	c.Assert(history[1].Reason, check.Equals, "r1")
	c.Assert(history[0].Reason, check.Equals, "r2")
	c.Assert(history[1].Log, check.Matches, ".*my evt1\n")
	c.Assert(history[0].Log, check.Matches, ".*my evt2\n")
}

func (s *S) TestAutoScaleRunHandler(c *check.C) {
	s.provisioner.AddNode(provision.AddNodeOptions{
		Address: "localhost:1999",
		Pool:    "pool1",
	})
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale:enabled")
	config.Set("docker:auto-scale:max-container-count", 2)
	defer config.Unset("docker:auto-scale:max-container-count")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/node/autoscale/run", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/x-json-stream")
	body := recorder.Body.String()
	c.Assert(body, check.Matches,
		`{"Message":".*running scaler \*autoscale.countScaler for \\"pool\\": \\"pool1\\"\\n","Timestamp":".*"}`+"\n"+
			`{"Message":".*rebalancing - dry: false, force: false\\n","Timestamp":".*"}`+"\n"+
			`{"Message":".*filtering pool: pool1\\n","Timestamp":".*"}`+"\n"+
			`{"Message":".*nothing to do for \\"pool\\": \\"pool1\\"\\n","Timestamp":".*"}`+"\n",
	)
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool},
		Owner:  s.token.GetUserName(),
		Kind:   "node.autoscale.update.run",
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleConfigHandler(c *check.C) {
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale:enabled")
	err := autoscale.Initialize()
	c.Assert(err, check.IsNil)
	base, err := autoscale.CurrentConfig()
	c.Assert(err, check.IsNil)
	expected, err := json.Marshal(base)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/node/autoscale/config", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	c.Assert(recorder.Header().Get("Content-Type"), check.Equals, "application/json")
	c.Assert(recorder.Body.String(), check.Equals, string(expected)+"\n")
}

func (s *S) TestAutoScaleListRulesEmpty(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/node/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var rules []autoscale.Rule
	err = json.Unmarshal(recorder.Body.Bytes(), &rules)
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoscale.Rule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
	})
}

func (s *S) TestAutoScaleListRulesWithLegacyConfig(c *check.C) {
	config.Set("docker:auto-scale:enabled", true)
	config.Set("docker:auto-scale:metadata-filter", "mypool")
	config.Set("docker:auto-scale:max-container-count", 4)
	config.Set("docker:auto-scale:scale-down-ratio", 1.5)
	config.Set("docker:auto-scale:prevent-rebalance", true)
	config.Set("docker:scheduler:max-used-memory", 0.9)
	defer config.Unset("docker:auto-scale")
	defer config.Unset("docker:scheduler:max-used-memory")
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/node/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var rules []autoscale.Rule
	err = json.Unmarshal(recorder.Body.Bytes(), &rules)
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoscale.Rule{
		{MetadataFilter: "mypool", Enabled: true, MaxContainerCount: 4, ScaleDownRatio: 1.5, PreventRebalance: true, MaxMemoryRatio: 0.9},
	})
}

func (s *S) TestAutoScaleListRulesWithDBConfig(c *check.C) {
	config.Set("docker:auto-scale:scale-down-ratio", 2.0)
	defer config.Unset("docker:auto-scale:max-container-count")
	config.Set("docker:scheduler:total-memory-metadata", "maxmemory")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	rules := []autoscale.Rule{
		{MetadataFilter: "", Enabled: true, MaxContainerCount: 10, ScaleDownRatio: 1.2},
		{MetadataFilter: "pool1", Enabled: true, ScaleDownRatio: 1.1, MaxMemoryRatio: 2.0},
	}
	for _, r := range rules {
		err := r.Update()
		c.Assert(err, check.IsNil)
	}
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("GET", "/node/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	var reqRules []autoscale.Rule
	err = json.Unmarshal(recorder.Body.Bytes(), &reqRules)
	c.Assert(err, check.IsNil)
	c.Assert(reqRules, check.DeepEquals, rules)
}

func (s *S) TestAutoScaleSetRule(c *check.C) {
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale-enabled")
	config.Set("docker:scheduler:total-memory-metadata", "maxmemory")
	defer config.Unset("docker:scheduler:total-memory-metadata")
	rule := autoscale.Rule{
		MetadataFilter: "pool1",
		Enabled:        true,
		ScaleDownRatio: 1.1,
		MaxMemoryRatio: 2.0,
	}
	v, err := form.EncodeToValues(&rule)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	request, err := http.NewRequest("POST", "/node/autoscale/rules", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := autoscale.ListRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoscale.Rule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
		rule,
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.autoscale.update",
		StartCustomData: []map[string]interface{}{
			{"name": "MetadataFilter", "value": "pool1"},
			{"name": "Enabled", "value": "true"},
			{"name": "ScaleDownRatio", "value": "1.1"},
			{"name": "MaxMemoryRatio", "value": "2"},
		},
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleSetRuleInvalidRule(c *check.C) {
	rule := autoscale.Rule{MetadataFilter: "pool1", Enabled: true, ScaleDownRatio: 0.9, MaxMemoryRatio: 2.0}
	v, err := form.EncodeToValues(&rule)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/node/autoscale/rules", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Matches, "(?s).*invalid rule, scale down ratio needs to be greater than 1.0, got 0.9.*")
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: "pool1"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.autoscale.update",
		StartCustomData: []map[string]interface{}{
			{"name": "MetadataFilter", "value": "pool1"},
			{"name": "Enabled", "value": "true"},
			{"name": "ScaleDownRatio", "value": "0.9"},
			{"name": "MaxMemoryRatio", "value": "2"},
		},
		ErrorMatches: `.*invalid rule, scale down ratio needs to be greater than 1.0, got 0.9.*`,
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleSetRuleExisting(c *check.C) {
	rule := autoscale.Rule{MetadataFilter: "", Enabled: true, ScaleDownRatio: 1.1, MaxContainerCount: 5}
	err := rule.Update()
	c.Assert(err, check.IsNil)
	rule.MaxContainerCount = 9
	v, err := form.EncodeToValues(&rule)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(v.Encode())
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("POST", "/node/autoscale/rules", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := autoscale.ListRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoscale.Rule{rule})
}

func (s *S) TestAutoScaleDeleteRule(c *check.C) {
	config.Set("docker:auto-scale:enabled", true)
	defer config.Unset("docker:auto-scale-enabled")
	rule := autoscale.Rule{MetadataFilter: "", Enabled: true, ScaleDownRatio: 1.1, MaxContainerCount: 5}
	err := rule.Update()
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/node/autoscale/rules", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := autoscale.ListRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoscale.Rule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: ""},
		Owner:  s.token.GetUserName(),
		Kind:   "node.autoscale.delete",
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleDeleteRuleNonDefault(c *check.C) {
	rule := autoscale.Rule{MetadataFilter: "mypool", Enabled: true, ScaleDownRatio: 1.1, MaxContainerCount: 5}
	err := rule.Update()
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/node/autoscale/rules/mypool", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK)
	rules, err := autoscale.ListRules()
	c.Assert(err, check.IsNil)
	c.Assert(rules, check.DeepEquals, []autoscale.Rule{
		{Enabled: true, ScaleDownRatio: 1.333, Error: "invalid rule, either memory information or max container count must be set"},
	})
	c.Assert(eventtest.EventDesc{
		Target: event.Target{Type: event.TargetTypePool, Value: "mypool"},
		Owner:  s.token.GetUserName(),
		Kind:   "node.autoscale.delete",
	}, eventtest.HasEvent)
}

func (s *S) TestAutoScaleDeleteRuleNotFound(c *check.C) {
	recorder := httptest.NewRecorder()
	request, err := http.NewRequest("DELETE", "/node/autoscale/rules/mypool", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	server := RunServer(true)
	server.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound)
	c.Assert(recorder.Body.String(), check.Equals, "rule not found\n")
	c.Assert(eventtest.EventDesc{
		Target:       event.Target{Type: event.TargetTypePool, Value: "mypool"},
		Owner:        s.token.GetUserName(),
		Kind:         "node.autoscale.delete",
		ErrorMatches: `rule not found`,
	}, eventtest.HasEvent)
}
