// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"fmt"
	"net/http"

	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/saml"
	"github.com/tsuru/tsuru/cmd"
	"github.com/tsuru/tsuru/errors"
)

func samlMetadata(w http.ResponseWriter, r *http.Request) error {
	if app.AuthScheme.Name() != "saml" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "This URL is only supported with saml enabled",
		}
	}
	page, err := saml.Metadata()
	if err != nil {
		return &errors.HTTP{Code: http.StatusInternalServerError, Message: err.Error()}
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(page))
	return nil
}

func samlCallbackLogin(w http.ResponseWriter, r *http.Request) error {
	if app.AuthScheme.Name() != "saml" {
		return &errors.HTTP{
			Code:    http.StatusBadRequest,
			Message: "This URL is only supported with saml enabled",
		}
	}
	params := map[string]string{}
	content := r.PostFormValue("SAMLResponse")
	if content == "" {
		return &errors.HTTP{Code: http.StatusBadRequest, Message: "Empty SAML Response"}
	}
	params["callback"] = "true"
	params["xml"] = content
	//Get saml.SAMLAuthScheme, error already treated on first check
	scheme, _ := auth.GetScheme("saml")
	_, err := scheme.Login(params)
	if err != nil {
		msg := fmt.Sprintf(cmd.SamlCallbackFailureMessage(), err.Error())
		fmt.Fprintf(w, msg)
	} else {
		fmt.Fprintf(w, cmd.SamlCallbackSuccessMessage())
	}
	return nil
}
