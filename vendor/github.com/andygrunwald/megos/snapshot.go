package megos

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
)

// GetMetricsSnapshot will return the snapshot of pid
func (c *Client) GetMetricsSnapshot(pid *Pid) (*MetricsSnapshot, error) {
	u := c.GetURLForMetricsSnapshotPid(*pid)
	resp, err := c.GetHTTPResponse(&u)
	return c.parseMetricsSnapshotResponse(resp, err)
}

// GetURLForMetricsSnapshotPid will return the URL for the snapshot
// based on a PID
func (c *Client) GetURLForMetricsSnapshotPid(pid Pid) url.URL {
	return c.getBaseURLForFilePid(pid, "metrics/snapshot")
}

// parseMetricsSnapshotResponse will transform a http.Response to snapshot map
func (c *Client) parseMetricsSnapshotResponse(resp *http.Response, err error) (*MetricsSnapshot, error) {
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	var snapshot MetricsSnapshot
	err = json.Unmarshal(body, &snapshot)
	if err != nil {
		return nil, err
	}

	c.Lock()
	c.MetricsSnapshot = &snapshot
	c.Unlock()

	return c.MetricsSnapshot, nil
}
