// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cluster

import (
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	"gopkg.in/check.v1"
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

type S struct {
	conn *db.Storage
}

var _ = check.Suite(&S{})

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpSuite(c *check.C) {
	config.Set("log:disable-syslog", true)
	config.Set("database:driver", "mongodb")
	config.Set("database:url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("database:name", "provision_kubernetes_cluster_tests_s")
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
}

func (s *S) SetUpTest(c *check.C) {
	provisiontest.ProvisionerInstance.Reset()
	err := dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
}

func (s *S) TearDownSuite(c *check.C) {
	s.conn.Close()
}

func (s *S) TestClusterSave(c *check.C) {
	cluster := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1", "addr2"},
		CaCert:      testCA,
		ClientCert:  testCert,
		ClientKey:   testKey,
		Default:     true,
		Provisioner: "fake",
		CustomData: map[string]string{
			"a": "b",
		},
	}
	err := cluster.Save()
	c.Assert(err, check.IsNil)
	coll, err := clusterCollection()
	c.Assert(err, check.IsNil)
	var dbCluster Cluster
	err = coll.FindId("c1").One(&dbCluster)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster, check.DeepEquals, cluster)
}

func (s *S) TestClusterSaveRemoveDefaults(c *check.C) {
	c1 := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Default:     true,
		Provisioner: "fake",
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:        "c2",
		Addresses:   []string{"addr2"},
		Default:     true,
		Provisioner: "fake",
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	coll, err := clusterCollection()
	c.Assert(err, check.IsNil)
	var dbCluster1, dbCluster2 Cluster
	err = coll.FindId("c1").One(&dbCluster1)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster1.Default, check.Equals, false)
	err = coll.FindId("c2").One(&dbCluster2)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster2.Default, check.Equals, true)
}

func (s *S) TestClusterSaveRemovePools(c *check.C) {
	c1 := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Pools:       []string{"p1", "p2"},
		Provisioner: "fake",
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:        "c2",
		Addresses:   []string{"addr2"},
		Pools:       []string{"p2", "p3"},
		Provisioner: "fake",
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	coll, err := clusterCollection()
	c.Assert(err, check.IsNil)
	var dbCluster1, dbCluster2 Cluster
	err = coll.FindId("c1").One(&dbCluster1)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster1.Pools, check.DeepEquals, []string{"p1"})
	err = coll.FindId("c2").One(&dbCluster2)
	c.Assert(err, check.IsNil)
	c.Assert(dbCluster2.Pools, check.DeepEquals, []string{"p2", "p3"})
}

type initClusterProv struct {
	*provisiontest.FakeProvisioner
	callCluster *Cluster
}

func (p *initClusterProv) InitializeCluster(c *Cluster) error {
	p.callCluster = c
	return nil
}

func (s *S) TestClusterSaveCallsProvInit(c *check.C) {
	inst := initClusterProv{FakeProvisioner: provisiontest.ProvisionerInstance}
	provision.Register("fake-cluster", func() (provision.Provisioner, error) {
		return &inst, nil
	})
	defer provision.Unregister("fake-cluster")
	c1 := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Pools:       []string{"p1", "p2"},
		Provisioner: "fake-cluster",
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.DeepEquals, *inst.callCluster)
}

func (s *S) TestClusterSaveValidation(c *check.C) {
	tests := []struct {
		c   Cluster
		err string
	}{
		{
			c: Cluster{
				Name:        "  ",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "cluster name is mandatory",
		},
		{
			c: Cluster{
				Name:        "1c",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "Invalid cluster name, cluster name should have at most 63 " +
				"characters, containing only lower case letters, numbers or dashes, " +
				"starting with a letter.",
		},
		{
			c: Cluster{
				Name:        "c_1",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "Invalid cluster name, cluster name should have at most 63 " +
				"characters, containing only lower case letters, numbers or dashes, " +
				"starting with a letter.",
		},
		{
			c: Cluster{
				Name:        "C1",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "Invalid cluster name, cluster name should have at most 63 " +
				"characters, containing only lower case letters, numbers or dashes, " +
				"starting with a letter.",
		},
		{
			c: Cluster{
				Name:        "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
				Addresses:   []string{"addr1", "addr2"},
				Default:     true,
				Provisioner: "fake",
			},
			err: "Invalid cluster name, cluster name should have at most 63 " +
				"characters, containing only lower case letters, numbers or dashes, " +
				"starting with a letter.",
		},
		{
			c: Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     false,
				Provisioner: "fake",
			},
			err: "either default or a list of pools must be set",
		},
		{
			c: Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     true,
				Pools:       []string{"p1"},
				Provisioner: "fake",
			},
			err: "cannot have both pools and default set",
		},
		{
			c: Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     true,
				Provisioner: "",
			},
			err: "provisioner name is mandatory",
		},
		{
			c: Cluster{
				Name:        "c1",
				Addresses:   []string{"addr1"},
				Default:     true,
				Provisioner: "invalid",
			},
			err: "unknown provisioner: \"invalid\"",
		},
	}
	for _, tt := range tests {
		err := tt.c.Save()
		c.Assert(err, check.ErrorMatches, tt.err)
		c.Assert(errors.Cause(err), check.FitsTypeOf, &tsuruErrors.ValidationError{})
	}
}

func (s *S) TestAllClusters(c *check.C) {
	c1 := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Pools:       []string{"p1"},
		Provisioner: "fake",
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:        "c2",
		Addresses:   []string{"addr2"},
		Pools:       []string{"p2"},
		Provisioner: "fake",
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	clusters, err := AllClusters()
	c.Assert(err, check.IsNil)
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Name < clusters[j].Name
	})
	c.Assert(clusters, check.HasLen, 2)
	c.Assert(clusters, check.DeepEquals, []*Cluster{&c1, &c2})
}

func (s *S) TestForPool(c *check.C) {
	c1 := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Pools:       []string{"p1", "p2"},
		Provisioner: "fake",
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	c2 := Cluster{
		Name:        "c2",
		Addresses:   []string{"addr2"},
		Pools:       []string{"p3"},
		Provisioner: "fake",
	}
	err = c2.Save()
	c.Assert(err, check.IsNil)
	c3 := Cluster{
		Name:        "c3",
		Addresses:   []string{"addr2"},
		Default:     true,
		Provisioner: "fake",
	}
	err = c3.Save()
	c.Assert(err, check.IsNil)
	cluster, err := ForPool("fake", "p1")
	c.Assert(err, check.IsNil)
	c.Assert(cluster, check.DeepEquals, &c1)
	c.Assert(cluster.Name, check.Equals, "c1")
	cluster, err = ForPool("fake", "p2")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "c1")
	cluster, err = ForPool("fake", "p3")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "c2")
	cluster, err = ForPool("fake", "p4")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "c3")
	cluster, err = ForPool("fake", "")
	c.Assert(err, check.IsNil)
	c.Assert(cluster.Name, check.Equals, "c3")
	err = DeleteCluster("c3")
	c.Assert(err, check.IsNil)
	_, err = ForPool("fake", "p4")
	c.Assert(err, check.Equals, ErrNoCluster)
	_, err = ForPool("other", "p3")
	c.Assert(err, check.Equals, ErrNoCluster)
}

func (s *S) TestByName(c *check.C) {
	c1 := Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Pools:       []string{"p1", "p2"},
		Provisioner: "fake",
	}
	err := c1.Save()
	c.Assert(err, check.IsNil)
	dbC1, err := ByName("c1")
	c.Assert(err, check.IsNil)
	c.Assert(c1, check.DeepEquals, *dbC1)
	_, err = ByName("cx")
	c.Assert(err, check.Equals, ErrClusterNotFound)
}
