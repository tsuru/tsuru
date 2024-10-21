// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"go/format"
	"log"
	"os"
	"sort"
	"text/template"
	"time"

	"github.com/tsuru/tsuru/permission"
	permTypes "github.com/tsuru/tsuru/types/permission"
)

var fileTpl = `// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright {{.Time.Year}} tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

var (
{{range .Permissions}} \
    Perm{{.Identifier}} = PermissionRegistry.get("{{.FullName}}") // {{.AllowedContexts}}
{{end}} \
)
`

type context struct {
	Time        time.Time
	Permissions permTypes.PermissionSchemeList
}

func main() {
	out := flag.String("o", "", "output file")
	flag.Parse()
	tmpl, err := template.New("tpl").Parse(fileTpl)
	if err != nil {
		log.Fatal(err)
	}
	lst := permission.PermissionRegistry.Permissions()
	sort.Sort(lst)
	data := context{
		Time:        time.Now(),
		Permissions: lst,
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		log.Fatal(err)
	}
	rawFile := buf.Bytes()
	rawFile = bytes.Replace(rawFile, []byte("\\\n"), []byte{}, -1)
	formatedFile, err := format.Source(rawFile)
	if err != nil {
		log.Fatalf("unable to format code: %s\n%s", err, rawFile)
	}
	file, err := os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0660)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	file.Write(formatedFile)
}
