package megos

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
)

// GetSystemFromPid will return the system stats of node
func (c *Client) GetSystemFromPid(pid *Pid) (*System, error) {
	u := c.GetURLForSystemFilePid(*pid)
	resp, err := c.GetHTTPResponse(&u)
	return c.parseSystemResponse(resp, err)
}

// GetURLForSystemFilePid will return the URL for the system stats of a node
// based on a PID
func (c *Client) GetURLForSystemFilePid(pid Pid) url.URL {
	return c.getBaseURLForFilePid(pid, "system/stats.json")
}

// parseSystemResponse will transform a http.Response into a System object
func (c *Client) parseSystemResponse(resp *http.Response, err error) (*System, error) {
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	var system System
	err = json.Unmarshal(body, &system)
	if err != nil {
		return nil, err
	}

	c.Lock()
	c.System = &system
	c.Unlock()

	return c.System, nil
}

func (c *Client) getBaseURLForFilePid(pid Pid, filename string) url.URL {
	host := pid.Host + ":" + strconv.Itoa(pid.Port)

	u := url.URL{
		Scheme: "http",
		Host:   host,
		Path:   "/" + filename,
	}

	return u
}
