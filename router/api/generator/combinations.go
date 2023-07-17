// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"go/format"
	"html/template"
	"log"
	"os"
	"sort"
	"time"
)

var capMap = map[string][]string{
	"tls": {"router.TLSRouter", "apiRouterWithTLSSupport"},
}

var fileTpl = `// AUTOMATICALLY GENERATED FILE - DO NOT EDIT!
// Please run 'go generate' to update this file.
//
// Copyright {{.Time.Year}} tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

{{ $enabled := .Enabled }}
{{ $capMap := .CapMap }}
{{ $caps := .OrderedCap }}

package api

import (
	"github.com/tsuru/tsuru/router"
)

func toSupportedInterface(base *apiRouter, supports map[capability]bool) router.Router {
{{ range $caps -}}
	{{ index (index $capMap .) 1 }}Inst := &{{ index (index $capMap .) 1 }}{ base }
{{ end }}

{{ range $idx, $element := .Combinations -}}
	if {{ range $capi, $capv := $caps -}}
		{{ if $capi }} && {{ end }}
		{{- if not (index (index $enabled $idx) $capv) -}} ! {{- end -}} 
		supports["{{ $capv }}"]
	{{- end -}} {
		return &struct {
			router.Router
		{{ range $element -}}
			{{ index (index $capMap (index $caps .)) 0 }}
		{{ end -}}
		}{
			base,
			{{ range $element -}}
				{{ index (index $capMap (index $caps .)) 1 }}Inst,
			{{ end -}}
		}
	}
{{ end -}}

	return nil
}
`

type context struct {
	Time         time.Time
	Combinations [][]int
	Enabled      []map[string]bool
	CapMap       map[string][]string
	OrderedCap   []string
}

func (c *context) prepare() error {
	c.Time = time.Now()
	c.Combinations = allCombinations(len(c.CapMap))
	for k := range c.CapMap {
		c.OrderedCap = append(c.OrderedCap, k)
	}
	sort.Strings(c.OrderedCap)
	for _, comb := range c.Combinations {
		vars := make(map[string]bool)
		for _, v := range comb {
			vars[c.OrderedCap[v]] = true
		}
		c.Enabled = append(c.Enabled, vars)
	}
	return nil
}

func allCombinations(n int) [][]int {
	var result [][]int
	combCount := 1 << uint(n)
	for i := 0; i < combCount; i++ {
		var item []int
		for j := 0; j < n; j++ {
			if i&(1<<uint(j)) != 0 {
				item = append(item, j)
			}
		}
		result = append(result, item)
	}
	return result
}

func main() {
	out := flag.String("o", "", "output file")
	flag.Parse()
	tmpl, err := template.New("tpl").Parse(fileTpl)
	if err != nil {
		log.Fatalf("unable to parse template: %v", err)
	}
	data := context{
		CapMap: capMap,
	}
	data.prepare()
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		log.Fatalf("unable to exec template: %v", err)
	}
	rawFile := buf.Bytes()
	formatedFile, err := format.Source(rawFile)
	if err != nil {
		log.Fatalf("unable to format code: %s\n%s", err, rawFile)
	}
	var file *os.File
	if *out == "-" {
		file = os.Stdout
	} else {
		file, err = os.OpenFile(*out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0660)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
	}
	file.Write(formatedFile)
}
