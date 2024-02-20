// Copyright 2012 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io"
	"net/http"

	"github.com/pkg/errors"
	tsuruio "github.com/tsuru/tsuru/io"
)

const (
	VerbosityHeader = "X-Tsuru-Verbosity"
)

// StreamJSONResponse supports the JSON streaming format from the tsuru API.
func StreamJSONResponse(w io.Writer, response *http.Response) error {
	if response == nil {
		return errors.New("response cannot be nil")
	}
	defer response.Body.Close()
	var err error
	output := tsuruio.NewStreamWriter(w, nil)
	for n := int64(1); n > 0 && err == nil; n, err = io.Copy(output, response.Body) {
	}
	if err != nil {
		return err
	}
	unparsed := output.Remaining()
	if len(unparsed) > 0 {
		return errors.Errorf("unparsed message error: %s", string(unparsed))
	}
	return nil
}
