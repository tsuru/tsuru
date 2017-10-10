// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package integration

import (
	check "gopkg.in/check.v1"
)

var tableExamples = []struct {
	table   string
	headers []string
	rows    [][]string
}{
	{
		`+---------------------------------------------+------------------------+----------------+--------------------------------------------------------------+
| Id                                          | IaaS                   | Address        | Creation Params                                              |
+---------------------------------------------+------------------------+----------------+--------------------------------------------------------------+
| labs-eo5g5tgbrbt4omgx3krizunah              | dockermachine          | 10.100.100.10  | cloudstack-delete-volumes=true                               |
|                                             |                        |                | cloudstack-disk-offering-id=xyz                              |
| labs-jzqzlifcjwq4a4gymikdlra62              | dockermachine          | 10.100.100.11  | cloudstack-delete-volumes=true                               |
|                                             |                        |                | cloudstack-disk-offering-id=abc                              |
|                                             |                        |                | cloudstack-network-id=123                                    |
+---------------------------------------------+------------------------+----------------+--------------------------------------------------------------+
| labs-asajdoiioj04j0f78vhbo989f              | dockermachine          | 10.100.100.12  | a=b                                                          |
|                                             |                        | 10.100.100.13  | c=d                                                          |
+---------------------------------------------+------------------------+----------------+--------------------------------------------------------------+`,
		[]string{
			"Id", "IaaS", "Address", "Creation Params",
		},
		[][]string{
			{"labs-eo5g5tgbrbt4omgx3krizunah", "dockermachine", "10.100.100.10", "cloudstack-delete-volumes=true\ncloudstack-disk-offering-id=xyz"},
			{"labs-jzqzlifcjwq4a4gymikdlra62", "dockermachine", "10.100.100.11", "cloudstack-delete-volumes=true\ncloudstack-disk-offering-id=abc\ncloudstack-network-id=123"},
			{"labs-asajdoiioj04j0f78vhbo989f", "dockermachine", "10.100.100.12\n10.100.100.13", "a=b\nc=d"},
		},
	},

	{
		`+---------------------------+---------------+--------------+-----------------------------------+
| Id                        | IaaS          | Address      | Creation Params                   |
+---------------------------+---------------+--------------+-----------------------------------+
| ean5emmgod2rybuvnhjgjlsoo | dockermachine | 172.30.0.75  | driver=amazonec2                  |
|                           |               |              | iaas-id=ean5emmgod2rybuvnhjgjlsoo |
|                           |               |              | iaas=dockermachine                |
+---------------------------+---------------+--------------+-----------------------------------+
| x2n2uizpb4bwd36oas3ocp2ae | dockermachine | 172.30.0.139 | driver=amazonec2                  |
|                           |               |              | iaas-id=x2n2uizpb4bwd36oas3ocp2ae |
|                           |               |              | iaas=dockermachine                |
+---------------------------+---------------+--------------+-----------------------------------+
`,
		[]string{
			"Id", "IaaS", "Address", "Creation Params",
		},
		[][]string{
			{"ean5emmgod2rybuvnhjgjlsoo", "dockermachine", "172.30.0.75", "driver=amazonec2\niaas-id=ean5emmgod2rybuvnhjgjlsoo\niaas=dockermachine"},
			{"x2n2uizpb4bwd36oas3ocp2ae", "dockermachine", "172.30.0.139", "driver=amazonec2\niaas-id=x2n2uizpb4bwd36oas3ocp2ae\niaas=dockermachine"},
		},
	},
}

func (s *S) TestResultTableParse(c *check.C) {
	for _, tt := range tableExamples {
		table := resultTable{raw: tt.table}
		table.parse()
		c.Assert(table.header, check.DeepEquals, tt.headers)
		c.Assert(table.rows, check.DeepEquals, tt.rows)
	}
}
