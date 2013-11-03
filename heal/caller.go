// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heal

import (
	"encoding/json"
	"fmt"
	"github.com/globocom/tsuru/log"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	mut        sync.Mutex
	healerUrls = make(map[string]*healer)
)

type healer struct {
	url string
}

func setHealers(h map[string]*healer) {
	mut.Lock()
	healerUrls = h
	mut.Unlock()
}

func getHealers() map[string]*healer {
	mut.Lock()
	defer mut.Unlock()
	return healerUrls
}

func (h *healer) heal() error {
	log.Debugf("healing tsuru healer with endpoint %s...", h.url)
	r, err := request("GET", h.url, nil)
	if err == nil {
		r.Body.Close()
	}
	return err
}

// healersFromResource returns healers registered in tsuru.
func healersFromResource(endpoint string) (map[string]*healer, error) {
	url := fmt.Sprintf("%s/healers", endpoint)
	response, err := request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		return nil, err
	}
	var h map[string]*healer
	data := map[string]string{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}
	h = make(map[string]*healer, len(data))
	for name, url := range data {
		h[name] = &healer{url: fmt.Sprintf("%s%s", endpoint, url)}
	}
	return h, nil
}

func request(method, url string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if token := os.Getenv("TSURU_TOKEN"); token != "" {
		request.Header.Add("Authorization", fmt.Sprintf("bearer %s", token))
	}
	resp, err := (&http.Client{}).Do(request)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// HealTicker execute the registered healers registered by RegisterHealerTicker.
func HealTicker(ticker <-chan time.Time) {
	log.Debug("running heal ticker")
	var wg sync.WaitGroup
	for _ = range ticker {
		healers := getHealers()
		wg.Add(len(healers))
		for name, h := range healers {
			log.Debugf("running verification/heal for %s", name)
			go func(healer *healer) {
				err := healer.heal()
				if err != nil {
					log.Debug(err.Error())
				}
				wg.Done()
			}(h)
		}
		wg.Wait()
	}
}

// RegisterHealerTicker register healers from resource.
func RegisterHealerTicker(ticker <-chan time.Time, endpoint string) {
	var registerHealer = func() {
		log.Debug("running register ticker")
		if healers, err := healersFromResource(endpoint); err == nil {
			setHealers(healers)
		}
	}
	registerHealer()
	go func() {
		for _ = range ticker {
			registerHealer()
		}
	}()
}
