// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"html/template"
	"net/http"
	"path"

	"github.com/tsuru/config"
)

func index(w http.ResponseWriter, r *http.Request) error {
	host, _ := config.GetString("host")
	userCreate, _ := config.GetBool("auth:user-registration")
	scheme, _ := config.GetString("auth:scheme")
	repoManager, _ := config.GetString("repo-manager")
	data := map[string]interface{}{
		"tsuruTarget": host,
		"userCreate":  userCreate,
		"nativeLogin": scheme == "" || scheme == "native",
		"keysEnabled": repoManager == "" || repoManager == "gandalf",
	}
	template, err := getTemplate()
	if err != nil {
		return err
	}
	return template.Execute(w, data)
}

func getTemplate() (*template.Template, error) {
	templateFile, _ := config.GetString("index-page-template")
	if templateFile == "" {
		return indexTemplate, nil
	}
	tmpl, err := template.New("index").Funcs(funcMap).ParseFiles(templateFile)
	if err != nil {
		return nil, err
	}
	return tmpl.Lookup(path.Base(templateFile)), nil
}
