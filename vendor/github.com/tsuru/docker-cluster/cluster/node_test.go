// Copyright 2015 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"testing"
	"time"

	dtesting "github.com/fsouza/go-dockerclient/testing"
)

func TestNodeStatus(t *testing.T) {
	node := Node{}
	if node.Status() != NodeStatusWaiting {
		t.Fatalf("Expected status NodeStatusWaiting, got %s", node.Status())
	}
	node.CreationStatus = NodeCreationStatusCreated
	if node.Status() != NodeStatusWaiting {
		t.Fatalf("Expected status NodeStatusWaiting, got %s", node.Status())
	}
	node.CreationStatus = NodeCreationStatusError
	if node.Status() != NodeCreationStatusError {
		t.Fatalf("Expected status NodeCreationStatusError, got %s", node.Status())
	}
	node.CreationStatus = NodeCreationStatusPending
	if node.Status() != NodeCreationStatusPending {
		t.Fatalf("Expected status NodeCreationStatusPending, got %s", node.Status())
	}
	node = Node{Metadata: map[string]string{
		"Failures": "1",
	}}
	if node.Status() != NodeStatusRetry {
		t.Fatalf("Expected status NodeStatusRetry, got %s", node.Status())
	}
	node = Node{Metadata: map[string]string{
		"LastSuccess": "xxx",
	}}
	if node.Status() != NodeStatusReady {
		t.Fatalf("Expected status NodeStatusReady, got %s", node.Status())
	}
	node = Node{Metadata: map[string]string{
		"DisabledUntil": time.Now().Add(1 * time.Minute).Format(time.RFC3339),
		"Failures":      "1",
	}}
	if node.Status() != NodeStatusTemporarilyDisabled {
		t.Fatalf("Expected status NodeStatusTemporarilyDisabled, got %s", node.Status())
	}
	future := time.Now().UTC().Add(5 * time.Second)
	node = Node{Healing: HealingData{LockedUntil: future}, Metadata: map[string]string{
		"LastSuccess": "date",
	}}
	if node.Status() != NodeStatusReady {
		t.Fatalf("Expected status NodeStatusReady got %s", node.Status())
	}
	node = Node{Healing: HealingData{LockedUntil: future, IsFailure: true}, Metadata: map[string]string{
		"DisabledUntil": time.Now().Add(1 * time.Minute).Format(time.RFC3339),
		"Failures":      "1",
	}}
	if node.Status() != NodeStatusHealing {
		t.Fatalf("Expected status NodeStatusHealing got %s", node.Status())
	}
	node = Node{CreationStatus: NodeCreationStatusDisabled}
	if node.Status() != NodeCreationStatusDisabled {
		t.Fatalf("Expected status NodeStatusDisabled got %s", node.Status())
	}
}

func TestNodeMarshalJSON(t *testing.T) {
	dt := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	node := Node{Address: "addr1", Metadata: map[string]string{
		"DisabledUntil": dt,
		"Failures":      "1",
	}}
	bytes, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var val map[string]interface{}
	expected := map[string]interface{}{
		"Address": "addr1",
		"Metadata": map[string]interface{}{
			"DisabledUntil": dt,
			"Failures":      "1",
		},
		"Status": NodeStatusTemporarilyDisabled,
	}
	err = json.Unmarshal(bytes, &val)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(val, expected) {
		t.Fatalf("Expected marshaled json to equal %#v, got: %#v", expected, val)
	}
}

func TestNodeUpdateError(t *testing.T) {
	node := Node{}
	expectedErr := "some error"
	node.updateError(errors.New(expectedErr), true)
	if node.Metadata["Failures"] != "1" {
		t.Fatalf("Expected failures counter 1, got: %s", node.Metadata["Failures"])
	}
	if node.Metadata["LastError"] != expectedErr {
		t.Fatalf("Expected last error %q, got %q", expectedErr, node.Metadata["LastError"])
	}
	node.updateError(errors.New(expectedErr), true)
	if node.Metadata["Failures"] != "2" {
		t.Fatalf("Expected failures counter 2, got: %s", node.Metadata["Failures"])
	}
	nonIncrementErr := errors.New("non incrementing")
	node.updateError(nonIncrementErr, false)
	if node.Metadata["Failures"] != "2" {
		t.Fatalf("Expected failures counter 2, got: %s", node.Metadata["Failures"])
	}
	if node.Metadata["LastError"] != nonIncrementErr.Error() {
		t.Fatalf("Expected last error %q, got %q", nonIncrementErr.Error(), node.Metadata["LastError"])
	}
}

func TestNodeUpdateTemporarilyDisabled(t *testing.T) {
	node := Node{}
	disabledUntil := time.Now().Add(5 * time.Minute)
	node.updateDisabled(disabledUntil)
	if node.Metadata["DisabledUntil"] != disabledUntil.Format(time.RFC3339) {
		t.Fatalf("Expected disabled until %q, got: %q",
			disabledUntil.Format(time.RFC3339), node.Metadata["DisabledUntil"])
	}
}

func TestNodeUpdateSuccess(t *testing.T) {
	node := Node{Metadata: map[string]string{
		"Failures":      "9",
		"DisabledUntil": "something",
		"LastError":     "some error",
	}}
	now := time.Now().Format(time.RFC3339)
	node.updateSuccess()
	_, ok := node.Metadata["Failures"]
	if ok {
		t.Fatal("Node shouldn't have Failures")
	}
	_, ok = node.Metadata["DisabledUntil"]
	if ok {
		t.Fatal("Node shouldn't have DisabledUntil")
	}
	_, ok = node.Metadata["LastError"]
	if ok {
		t.Fatal("Node shouldn't have LastError")
	}
	re := regexp.MustCompile(`(.*T\d{2}:\d{2}).*`)
	lastSuccess := node.Metadata["LastSuccess"]
	lastSuccess = re.ReplaceAllString(lastSuccess, "$1")
	now = re.ReplaceAllString(now, "$1")
	if lastSuccess != now {
		t.Fatalf("Expected LastSuccess %q, got: %q", now, lastSuccess)
	}
}

func TestNodeResetFailures(t *testing.T) {
	node := Node{Metadata: map[string]string{
		"Failures":      "9",
		"DisabledUntil": "something",
		"LastError":     "some error",
		"LastSuccess":   "something",
	}}
	node.ResetFailures()
	_, ok := node.Metadata["Failures"]
	if ok {
		t.Fatal("Node shouldn't have Failures")
	}
	_, ok = node.Metadata["DisabledUntil"]
	if ok {
		t.Fatal("Node shouldn't have DisabledUntil")
	}
	_, ok = node.Metadata["LastError"]
	if ok {
		t.Fatal("Node shouldn't have LastError")
	}
	lastSuccess := node.Metadata["LastSuccess"]
	if lastSuccess != "something" {
		t.Fatalf("Node should have preserved LastSuccess, got %s", lastSuccess)
	}
}

func TestNodeIsEnabled(t *testing.T) {
	node := Node{}
	if !node.isEnabled() {
		t.Fatal("node should be enabled")
	}
	node.CreationStatus = NodeCreationStatusCreated
	if !node.isEnabled() {
		t.Error("node should be enabled when CreationStatus=NodeCreationStatusCreated")
	}
	node.CreationStatus = NodeCreationStatusPending
	if node.isEnabled() {
		t.Error("node should not be enabled when CreationStatus=NodeCreationStatusPending")
	}
	node.CreationStatus = NodeCreationStatusError
	if node.isEnabled() {
		t.Error("node should not be enabled when CreationStatus=NodeCreationStatusError")
	}
	node = Node{Metadata: map[string]string{
		"DisabledUntil": time.Now().Add(-1 * time.Minute).Format(time.RFC3339),
	}}
	if !node.isEnabled() {
		t.Fatal("node should be enabled")
	}
	node = Node{Metadata: map[string]string{
		"DisabledUntil": time.Now().Add(1 * time.Minute).Format(time.RFC3339),
	}}
	if node.isEnabled() {
		t.Fatal("node should be disabled")
	}
	future := time.Now().UTC().Add(5 * time.Second)
	node = Node{Healing: HealingData{LockedUntil: future}}
	if !node.isEnabled() {
		t.Fatal("node should be enabled")
	}
	node = Node{Healing: HealingData{LockedUntil: future, IsFailure: true}}
	if node.isEnabled() {
		t.Fatal("node should be disabled")
	}
	node = Node{CreationStatus: NodeCreationStatusDisabled}
	if node.isEnabled() {
		t.Fatal("node should be disabled")
	}
}

func TestNodeFailureCount(t *testing.T) {
	node := Node{}
	if node.FailureCount() != 0 {
		t.Fatalf("Expected failure count 0, got: %d", node.FailureCount())
	}
	node = Node{Metadata: map[string]string{"Failures": "3"}}
	if node.FailureCount() != 3 {
		t.Fatalf("Expected failure count 3, got: %d", node.FailureCount())
	}
}

func TestNodeListFilterDisabledAndHealing(t *testing.T) {
	nodes := []Node{{Address: "a1"}, {Address: "a2"}, {Address: "a3"}, {Address: "a4"}}
	until := time.Now().Add(1 * time.Minute).Format(time.RFC3339)
	nodes[1].Metadata = map[string]string{"DisabledUntil": until}
	future := time.Now().UTC().Add(5 * time.Second)
	nodes[3].Healing = HealingData{LockedUntil: future, IsFailure: true}
	filtered := NodeList(nodes).filterDisabled()
	if len(filtered) != 2 {
		t.Fatalf("Expected filtered nodes len = 2, got %d", len(filtered))
	}
	if filtered[0].Address != "a1" || filtered[1].Address != "a3" {
		t.Fatalf("Expected filtered nodes to be %#v", filtered)
	}
}

func TestNodeClient(t *testing.T) {
	server, err := dtesting.NewServer("127.0.0.1:0", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	node := Node{Address: server.URL()}
	client, err := node.Client()
	if err != nil {
		t.Error(err)
	}
	err = client.Ping()
	if err != nil {
		t.Error(err)
	}
}

func TestNodeDefTLSClient(t *testing.T) {
	dockerTLS := dtesting.TLSConfig{
		CertPath:    "./testdata/server-cert.pem",
		CertKeyPath: "./testdata/server-key.pem",
		RootCAPath:  "./testdata/ca.pem",
	}
	var req string
	server, err := dtesting.NewTLSServer("127.0.0.1:0", nil, func(r *http.Request) {
		req = r.URL.Path
	}, dockerTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	tlsConfig, err := readTLSConfig("./testdata")
	if err != nil {
		t.Fatal(err)
	}
	url, err := url.Parse(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	url.Scheme = "https"
	node := Node{Address: url.String(), defTLSConfig: tlsConfig}
	client, err := node.Client()
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(client.TLSConfig, tlsConfig) {
		t.Fatal("docker client TLS not configured correctly.")
	}
	client.Info()
	if req != "/info" {
		t.Fatal("unable to ping docker server using tls")
	}
}
func TestNodeLocalTLSClient(t *testing.T) {
	dockerTLS := dtesting.TLSConfig{
		CertPath:    "./testdata/server-cert.pem",
		CertKeyPath: "./testdata/server-key.pem",
		RootCAPath:  "./testdata/ca.pem",
	}
	var req string
	server, err := dtesting.NewTLSServer("127.0.0.1:0", nil, func(r *http.Request) {
		req = r.URL.Path
	}, dockerTLS)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop()
	url, err := url.Parse(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	url.Scheme = "https"
	ca, err := ioutil.ReadFile("./testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	cert, err := ioutil.ReadFile("./testdata/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	key, err := ioutil.ReadFile("./testdata/key.pem")
	if err != nil {
		t.Fatal(err)
	}
	node := Node{
		Address:    url.String(),
		CaCert:     ca,
		ClientCert: cert,
		ClientKey:  key,
	}
	client, err := node.Client()
	if err != nil {
		t.Error(err)
	}
	if client.TLSConfig == nil {
		t.Fatal("docker client TLS not configured correctly.")
	}
	client.Info()
	if req != "/info" {
		t.Fatal("unable to ping docker server using tls")
	}
}

func TestNodeCleanMetadata(t *testing.T) {
	node := Node{
		Metadata: map[string]string{
			"m1":            "v1",
			"Failures":      "f1",
			"m2":            "v2",
			"DisabledUntil": "d1",
		},
	}
	expectedClean := map[string]string{
		"m1": "v1",
		"m2": "v2",
	}
	cleanMetadata := node.CleanMetadata()
	if !reflect.DeepEqual(cleanMetadata, expectedClean) {
		t.Fatalf("expected node.CleanMetadata() == %#v. got %#v", expectedClean, cleanMetadata)
	}
	expectedExtra := map[string]string{
		"Failures":      "f1",
		"DisabledUntil": "d1",
	}
	extraMetadata := node.ExtraMetadata()
	if !reflect.DeepEqual(extraMetadata, expectedExtra) {
		t.Fatalf("expected node.ExtraMetadata() == %#v. got %#v", expectedExtra, extraMetadata)
	}
}
