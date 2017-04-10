// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ajg/form"
	"github.com/tsuru/tsuru/api"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	"github.com/tsuru/tsuru/db"
	"github.com/tsuru/tsuru/db/dbtest"
	"github.com/tsuru/tsuru/permission"
	"github.com/tsuru/tsuru/permission/permissiontest"
	"github.com/tsuru/tsuru/provision/kubernetes"
	"gopkg.in/check.v1"
)

var _ = check.Suite(&S{})

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
	conn  *db.Storage
	user  *auth.User
	token auth.Token
}

func Test(t *testing.T) {
	check.TestingT(t)
}

func (s *S) SetUpTest(c *check.C) {
	var err error
	s.conn, err = db.Conn()
	c.Assert(err, check.IsNil)
	err = dbtest.ClearAllCollections(s.conn.Apps().Database)
	c.Assert(err, check.IsNil)
	nativeScheme := auth.ManagedScheme(native.NativeScheme{})
	app.AuthScheme = nativeScheme
	s.user, s.token = permissiontest.CustomUserWithPermission(c, nativeScheme, "usr", permission.Permission{
		Scheme:  permission.PermKubernetesCluster,
		Context: permission.PermissionContext{CtxType: permission.CtxGlobal},
	})
}

func (s *S) TearDownTest(c *check.C) {
	s.conn.Close()
}

func (s *S) TestUpdateCluster(c *check.C) {
	cluster := kubernetes.Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	encoded, err := form.EncodeToString(cluster)
	c.Assert(err, check.IsNil)
	body := strings.NewReader(encoded)
	request, err := http.NewRequest("POST", "/1.3/kubernetes/clusters", body)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := kubernetes.AllClusters()
	c.Assert(err, check.IsNil)
	c.Assert(clusters, check.HasLen, 1)
	c.Assert(clusters[0].Name, check.Equals, "c1")
	c.Assert(clusters[0].Addresses, check.DeepEquals, []string{"addr1"})
	c.Assert(clusters[0].Default, check.Equals, true)
}

func (s *S) TestListClusters(c *check.C) {
	cluster := kubernetes.Cluster{
		Name:       "c1",
		Addresses:  []string{"addr1"},
		CaCert:     testCA,
		ClientCert: testCert,
		ClientKey:  testKey,
		Default:    true,
	}
	err := cluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("GET", "/1.3/kubernetes/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	var retClusters []kubernetes.Cluster
	err = json.Unmarshal(recorder.Body.Bytes(), &retClusters)
	c.Assert(err, check.IsNil)
	c.Assert(retClusters, check.DeepEquals, []kubernetes.Cluster{{
		Name:       "c1",
		Addresses:  []string{"addr1"},
		CaCert:     testCA,
		ClientCert: testCert,
		Default:    true,
	}})
}

func (s *S) TestListClustersNoContent(c *check.C) {
	request, err := http.NewRequest("GET", "/1.3/kubernetes/clusters", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNoContent, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteClusterNotFound(c *check.C) {
	request, err := http.NewRequest("DELETE", "/1.3/kubernetes/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusNotFound, check.Commentf("body: %q", recorder.Body.String()))
}

func (s *S) TestDeleteCluster(c *check.C) {
	cluster := kubernetes.Cluster{
		Name:      "c1",
		Addresses: []string{"addr1"},
		Default:   true,
	}
	err := cluster.Save()
	c.Assert(err, check.IsNil)
	request, err := http.NewRequest("DELETE", "/1.3/kubernetes/clusters/c1", nil)
	c.Assert(err, check.IsNil)
	request.Header.Set("Authorization", "bearer "+s.token.GetValue())
	recorder := httptest.NewRecorder()
	m := api.RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusOK, check.Commentf("body: %q", recorder.Body.String()))
	clusters, err := kubernetes.AllClusters()
	c.Assert(err, check.Equals, kubernetes.ErrNoCluster)
	c.Assert(clusters, check.HasLen, 0)
}
