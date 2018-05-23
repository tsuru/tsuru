// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"math/rand"
	"sort"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/cluster"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
	"k8s.io/client-go/rest"
)

var (
	testCA = []byte(`-----BEGIN CERTIFICATE-----
MIIDCDCCAfCgAwIBAgIRAM6vVAlsqKsGzdFbksis/oUwDQYJKoZIhvcNAQELBQAw
FTETMBEGA1UEChMKdHN1cnUgSW5jLjAeFw0xNjExMjExODU0MTFaFw0yNjExMTkx
ODU0MTFaMBUxEzARBgNVBAoTCnRzdXJ1IEluYy4wggEiMA0GCSqGSIb3DQEBAQUA
A4IBDwAwggEKAoIBAQDJ/o/mI7xXSBxpOyEtImNzJn2gNm5GqVNcRCRDi+cuJOCn
YEi8WrygM6SaiqflclRtL2F+8NEQ+4pL8W16OBrJgQXvG9GXrGU44fJM7wacQLPV
oQ5D7ZOU3alGXhXrbeLP0FAspDoYNq5lYYKfSX+Ao0niOZ+BbBNUWH3t96ztQX6m
rJ2i4Dr+b08qNMmPUzifQDvmQMO9ZyZR/6vlH+rWP70hRQb9shNvmsXCZl7yOuz8
XL06YFbd5Si00lxi8kAOUdWUIHFVJszjO8+kPHrWQIVkUoJjhVu8SJLk0R+a7P0A
F4Hl/nJG4N1F+Q/32U9YcYRVeu6aRYtty8pORXs/AgMBAAGjUzBRMA4GA1UdDwEB
/wQEAwIChDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwDwYDVR0TAQH/
BAUwAwEB/zAPBgNVHREECDAGhwR/AAABMA0GCSqGSIb3DQEBCwUAA4IBAQC66WlT
duzfp7v9u2H9ivlTNJHRYLbuJPmBWzZNnJ+dCAOeNxzKBYuP6k6TMcwpXcKDMtaP
V0HWNMMdqu6UqXMIi9gZGjwXHHLUQNXHdMiUrCg/6b2X0DbWWEeIZ9fZuU7EWqbD
L+2Xf644e+jzitDsHTJBgGB9tibsn3bLfZL/Ew9RmxbWy4uPD1M6TW3s7bUKyBby
g+qhU1+9MJbsC40WiNNN9o6u8Jg8SrDEZbyNR4a64DttkpgExyVBOjaa09Msw4kP
y5f7zisJSrd4xQJvOTeUXz/nsFf/3+UyvtXi/Ka2BeD3QQAdhu+d5MMK2YfLBxDd
IfmeyWrhEgLxhXWT
-----END CERTIFICATE-----`)
	testCert = []byte(`-----BEGIN CERTIFICATE-----
MIIC6jCCAdKgAwIBAgIRAIblZTW4K3X3Fu2xvm4v0PQwDQYJKoZIhvcNAQELBQAw
FTETMBEGA1UEChMKdHN1cnUgSW5jLjAeFw0xNjExMjExODU0MTFaFw0yNjExMTkx
ODU0MTFaMBUxEzARBgNVBAoTCnRzdXJ1IEluYy4wggEiMA0GCSqGSIb3DQEBAQUA
A4IBDwAwggEKAoIBAQDLkj+xbtgT8aHLqBl6sc5F7CPX2WFM8gnsHf+Dg9z0NOIf
bv/NpypWPsxrN/7TIvTQX7TbYX6kXNQFBl74q9OhwWL3R3StWXsHnMiem/Ay8EWf
ibFOsXLDnJZfzkWjlSHQqoYW5iwbmb6tlFL/jf3TpjYCGpkWxpRVB9ucAdJ8uT4u
4OK74kZ87cBMWvQlgtR7z4IUSDIt9h3jqUqnycgZrHH0GpASRpeE0EhpF2cHA3TT
M5PRq++XagYR+O683ONNuU92o4fj8XqJ6BwizFIpGUzNJD1wASpsDZglzbHHJCZB
zGu8VH2IVPcMLxJ1MHViiMJMucWl2OvDu4I5mO5lAgMBAAGjNTAzMA4GA1UdDwEB
/wQEAwIHgDATBgNVHSUEDDAKBggrBgEFBQcDAjAMBgNVHRMBAf8EAjAAMA0GCSqG
SIb3DQEBCwUAA4IBAQCryUo0q6wSb3CRNSnGNZpwJF6heKPqRs1arlQcWul8Jjl6
JIJR30+vzpS0LzLtVzY7G7KDzKb0/ZlMEf8dYj3chkX2tJWiBSMC7kZUgR7GFSE8
j1rZOFArlt/8n/vMYNgPB/CABLs11cNPNgtSK+h9czHm3cfyHHQBKq0VnuYO2myZ
Fxh7275ENIOP6McbPugPwKkCrpe2euXg/dnJdqL9JCvCCqp8IqgojcPNrNWAAD1v
qzT0RHwwU4GDqrPaf6RtYZNGrgTIwzL3X+LbcE/LwRd51lu7dmyaHCIfJD19kVfW
V7SqyOStT5BRk1BTeoQ1q3Cg6n1w1Uan7DarQPtY
-----END CERTIFICATE-----`)
	testKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAy5I/sW7YE/Ghy6gZerHORewj19lhTPIJ7B3/g4Pc9DTiH27/
zacqVj7Mazf+0yL00F+022F+pFzUBQZe+KvTocFi90d0rVl7B5zInpvwMvBFn4mx
TrFyw5yWX85Fo5Uh0KqGFuYsG5m+rZRS/43906Y2AhqZFsaUVQfbnAHSfLk+LuDi
u+JGfO3ATFr0JYLUe8+CFEgyLfYd46lKp8nIGaxx9BqQEkaXhNBIaRdnBwN00zOT
0avvl2oGEfjuvNzjTblPdqOH4/F6iegcIsxSKRlMzSQ9cAEqbA2YJc2xxyQmQcxr
vFR9iFT3DC8SdTB1YojCTLnFpdjrw7uCOZjuZQIDAQABAoIBAHTkha5c97Z8CWvo
GrlZYBjGf85yBG0qjogGsbHTBg00UKO8GSb91lGvvbHTdX/HkswUKMXQNegrZZN9
FQu1ntBDO5DCdz0TJJI6dPiekk6tqUzyw91sB3pLcA2TZGmKOOCZCmYbxTgUEGmb
wgz8e8QMrPaIT1/Ep2gsGu56HWN/84+qJQgA5yprVg2zPZTrGKmcURa3GZJFycHq
FIn1IcUAIaGyRBVUwuhYlaGqKtSbGiLQJogHpTDXdN276bXojbwG8Lr9lmDRr0HD
YhfPsucF7DgP06rvwYw1CRvhBF6cb7QcQUCDC5Up2IPDNZ9WJx71hzAHMO5hiBbN
7h1yxAECgYEA/c/HVLERuWHKUD4YqqM36emlbrtNTPAwN4zSTmPvP9bD4y8/yo4C
5W8twY045D6SMtEp2WfmAjjmT56ngz++V/BR59JT3PgOcizyZW41XvNi3wLQs/In
Bw9qkxxuvuRNATD75wzKprol6IbSeZV1mK/MelzTLdNPsmNEb2G2N6cCgYEAzVOT
+M2areZVETEGkGtgX/sqHJQinxUR5kltCWhb+e8WAlkuirCA6yNhPAN9VmIW0UKe
/3kh6z5R0tLEvk5o9/Wk7Gr2TD0HfqcQdoU7A2G79TSVolfzhcisc/EjhL0s0O0I
UY0dWUDCVnuYSdYSVBCU1nv+UH3ypkXB7KuwaxMCgYBNC6Ogi9erhInbbd4i/kTc
1rYHNQg0EL0yP6cfcKqRoGn6Lr+Yhx9N8j/bfzkD4BKVJnUjr6xchFU1Wh3Tc6ge
Ha9fRbN7YjlQY2B5dcjxt8QNmlcsKJe8Ruu9GGZtv/O4JtxwuKtTjTIwsax2h+4Y
mVTi2Aaq5HhO2F9PyEN+BQKBgQDBLtqxI9ldtrHg2+yeIrjpdWnYu7ObU3qk3f69
9Dddf4qIqRn2GT0ifwY0LeBWTzHCr1jjazfzmo3nurCrkSCH89G5MqYLcvxDOLJv
oMi2VNVATrpepTuVawp+h6nwcQDijbHe8NNlL13pep39EnHqMDOpXb4YQ3fy4v0j
TSJOiwKBgD0ShhQXpBk6x3InwAoBAPFcqbBhY2y0Ts97h2MkKwt3ZXHxFHMQTp6n
MZd+pt1LzfT9/E1gf7WYoGlK3N2GS0F0kieMiiAdTiEMPzBu5YXxRmkpSq4uCxD+
qni/3jTJOxDGMH+x06HZjWietWmbY+aKWkKCyGGVVzlKTEBUMSSU
-----END RSA PRIVATE KEY-----`)
)

func (s *S) TestClusterInitClient(c *check.C) {
	c1 := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      testCA,
		ClientCert:  testCert,
		ClientKey:   testKey,
		Default:     true,
		Provisioner: provisionerName,
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	cli, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(cli.Interface, check.NotNil)
	c.Assert(cli.restConfig, check.NotNil)
	expected := &rest.Config{
		APIPath: "/api",
		Host:    "addr1",
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   testCA,
			CertData: testCert,
			KeyData:  testKey,
		},
		Timeout: time.Minute,
	}
	expected.ContentConfig = cli.restConfig.ContentConfig
	c.Assert(cli.restConfig, check.DeepEquals, expected)
}

func (s *S) TestClusterGetRestConfigMultipleAddrsRandom(c *check.C) {
	c1 := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1", "addr2"},
		Default:     true,
		Provisioner: provisionerName,
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	rand.Seed(3)
	cfg, err := getRestConfig(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(cfg.Host, check.Equals, "addr1")
	cfg, err = getRestConfig(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(cfg.Host, check.Equals, "addr2")
}

func (s *S) TestClusterClientSetTimeout(c *check.C) {
	c1 := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1", "addr2"},
		Default:     true,
		Provisioner: provisionerName,
	}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	client.SetTimeout(time.Hour)
	c.Assert(client.restConfig.Timeout, check.Equals, time.Hour)
}

func (s *S) TestClusterAppNamespace(c *check.C) {
	c1 := cluster.Cluster{Addresses: []string{"addr1"}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	c.Assert(client.AppNamespace(a), check.Equals, "default")
}

func (s *S) TestClusterNamespace(c *check.C) {
	c1 := cluster.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "x"}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.Namespace("mypool"), check.Equals, "x")
	c1 = cluster.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": ""}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.Namespace("mypool"), check.Equals, "default")
	c1 = cluster.Cluster{Addresses: []string{"addr1"}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.Namespace("mypool"), check.Equals, "default")
}

func (s *S) TestClusterNamespacePerPool(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	c1 := cluster.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "x"}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.Namespace("mypool"), check.Equals, "x-mypool")
	c.Assert(client.Namespace(""), check.Equals, "x")
	c1 = cluster.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": ""}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.Namespace("mypool"), check.Equals, "tsuru-mypool")
	c1 = cluster.Cluster{Addresses: []string{"addr1"}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.Namespace("mypool"), check.Equals, "tsuru-mypool")
	c.Assert(client.Namespace(""), check.Equals, "tsuru")
}

func (s *S) TestClusterOvercommitFactor(c *check.C) {
	c1 := cluster.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
		"overcommit-factor":         "2",
		"my-pool:overcommit-factor": "3",
		"invalid:overcommit-factor": "a",
	}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	ovf, err := client.OvercommitFactor("my-pool")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, int64(3))
	ovf, err = client.OvercommitFactor("global")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, int64(2))
	ovf, err = client.OvercommitFactor("invalid")
	c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
	c.Assert(ovf, check.Equals, int64(0))
}

func (s *S) TestClustersForApps(c *check.C) {
	c1 := cluster.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Default:     true,
		Provisioner: provisionerName,
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := cluster.Cluster{
		Name:        "c2",
		Addresses:   []string{"addr2"},
		Pools:       []string{"p1", "p2"},
		Provisioner: provisionerName,
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	a1 := provisiontest.NewFakeApp("myapp1", "python", 0)
	a1.Pool = "xyz"
	a2 := provisiontest.NewFakeApp("myapp2", "python", 0)
	a2.Pool = "p1"
	a3 := provisiontest.NewFakeApp("myapp3", "python", 0)
	a3.Pool = "p2"
	a4 := provisiontest.NewFakeApp("myapp4", "python", 0)
	a4.Pool = "abc"
	cApps, err := clustersForApps([]provision.App{a1, a2, a3, a4})
	c.Assert(err, check.IsNil)
	c.Assert(cApps, check.HasLen, 2)
	sort.Slice(cApps, func(i, j int) bool {
		return cApps[i].client.Name < cApps[j].client.Name
	})
	c.Assert(cApps[0].client.Name, check.Equals, "c1")
	c.Assert(cApps[1].client.Name, check.Equals, "c2")
	for idx := range cApps {
		sort.Slice(cApps[idx].apps, func(i, j int) bool {
			return cApps[idx].apps[i].GetName() < cApps[idx].apps[j].GetName()
		})
	}
	c.Assert(cApps[0].apps, check.DeepEquals, []provision.App{a1, a4})
	c.Assert(cApps[1].apps, check.DeepEquals, []provision.App{a2, a3})
}
