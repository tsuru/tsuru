// Copyright 2015 docker-cluster authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cluster provides types and functions for management of Docker
// clusters, scheduling container operations among hosts running Docker
// (nodes).
package cluster

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/fsouza/go-dockerclient"
)

// Node represents a host running Docker. Each node has an Address
// (in the form <scheme>://<host>:<port>/) and a map with arbritary
// metadata.
type Node struct {
	Address        string `bson:"_id"`
	Healing        HealingData
	Metadata       map[string]string
	CreationStatus string
	CaCert         []byte
	ClientCert     []byte
	ClientKey      []byte
	defTLSConfig   *tls.Config
	nodeTLSConfig  *tls.Config
}

type HealingData struct {
	LockedUntil time.Time
	IsFailure   bool
}

type NodeList []Node

const (
	NodeStatusWaiting             = "waiting"
	NodeStatusReady               = "ready"
	NodeStatusRetry               = "ready for retry"
	NodeStatusTemporarilyDisabled = "temporarily disabled"
	NodeStatusHealing             = "healing"

	NodeCreationStatusCreated  = "created"
	NodeCreationStatusError    = "error"
	NodeCreationStatusPending  = "pending"
	NodeCreationStatusDisabled = "disabled"
)

func (a NodeList) Len() int           { return len(a) }
func (a NodeList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a NodeList) Less(i, j int) bool { return a[i].Address < a[j].Address }

func (n Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"Address":  n.Address,
		"Metadata": n.Metadata,
		"Status":   n.Status(),
	})
}

func (n *Node) HasSuccess() bool {
	_, hasSuccess := n.Metadata["LastSuccess"]
	return hasSuccess
}

func (n *Node) Status() string {
	if n.CreationStatus != "" && n.CreationStatus != NodeCreationStatusCreated {
		return n.CreationStatus
	}
	if n.isHealing() {
		return NodeStatusHealing
	}
	if n.Metadata == nil {
		return NodeStatusWaiting
	}
	if n.isEnabled() {
		_, hasFailures := n.Metadata["Failures"]
		if hasFailures {
			return NodeStatusRetry
		}
		if !n.HasSuccess() {
			return NodeStatusWaiting
		}
		return NodeStatusReady
	}
	return NodeStatusTemporarilyDisabled
}

func (n *Node) FailureCount() int {
	if n.Metadata == nil {
		return 0
	}
	metaFail, _ := n.Metadata["Failures"]
	failures, _ := strconv.Atoi(metaFail)
	return failures
}

func (n *Node) ResetFailures() {
	if n.Metadata == nil {
		n.Metadata = make(map[string]string)
	}
	delete(n.Metadata, "Failures")
	delete(n.Metadata, "DisabledUntil")
	delete(n.Metadata, "LastError")
}

func (n *Node) Client() (*docker.Client, error) {
	client, err := docker.NewClient(n.Address)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := n.getTLSConfig()
	if err != nil {
		return nil, err
	}
	client.TLSConfig = tlsConfig
	client.HTTPClient = clientWithTimeout(defaultDialTimeout, defaultTimeout, tlsConfig)
	client.Dialer = timeout10Dialer
	return client, nil
}

func (n *Node) updateError(lastErr error, incrementFailures bool) {
	if n.Metadata == nil {
		n.Metadata = make(map[string]string)
	}
	if incrementFailures {
		n.Metadata["Failures"] = strconv.Itoa(n.FailureCount() + 1)
	}
	n.Metadata["LastError"] = lastErr.Error()
}

func (n *Node) updateDisabled(disabledUntil time.Time) {
	if n.Metadata == nil {
		n.Metadata = make(map[string]string)
	}
	n.Metadata["DisabledUntil"] = disabledUntil.Format(time.RFC3339)
}

func (n *Node) updateSuccess() {
	n.ResetFailures()
	n.Metadata["LastSuccess"] = time.Now().Format(time.RFC3339)
}

var extraMetadataKeys = []string{
	"Failures", "DisabledUntil", "LastError", "LastSuccess",
}

func isExtra(key string) bool {
	for _, k := range extraMetadataKeys {
		if k == key {
			return true
		}
	}
	return false
}

func (n *Node) CleanMetadata() map[string]string {
	paramsCopy := make(map[string]string)
	for k, v := range n.Metadata {
		if !isExtra(k) {
			paramsCopy[k] = v
		}
	}
	return paramsCopy
}

func (n *Node) ExtraMetadata() map[string]string {
	paramsCopy := make(map[string]string)
	for k, v := range n.Metadata {
		if isExtra(k) {
			paramsCopy[k] = v
		}
	}
	return paramsCopy
}

func (n *Node) isEnabled() bool {
	if n.CreationStatus != "" && n.CreationStatus != NodeCreationStatusCreated {
		return false
	}
	if n.isHealing() {
		return false
	}
	if n.Metadata == nil {
		return true
	}
	disabledStr, _ := n.Metadata["DisabledUntil"]
	t, _ := time.Parse(time.RFC3339, disabledStr)
	return time.Now().After(t)
}

func (n *Node) isHealing() bool {
	return (!n.Healing.LockedUntil.IsZero()) && n.Healing.IsFailure
}

func (nodes NodeList) filterDisabled() NodeList {
	filtered := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if node.isEnabled() {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func (n *Node) getTLSConfig() (*tls.Config, error) {
	if n.nodeTLSConfig != nil {
		return n.nodeTLSConfig, nil
	}
	if len(n.CaCert) > 0 {
		tlsCert, err := tls.X509KeyPair(n.ClientCert, n.ClientKey)
		if err != nil {
			return nil, err
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(n.CaCert) {
			return nil, errors.New("Could not add RootCA pem")
		}
		config := &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			RootCAs:      caPool,
		}
		n.nodeTLSConfig = config
		return config, nil
	}
	return n.defTLSConfig, nil
}
