// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/provisiontest"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
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
	c1 := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		CaCert:      testCA,
		ClientCert:  testCert,
		ClientKey:   testKey,
		Default:     true,
		Provisioner: provisionerName,
	}
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
	cli.restConfig.Dial = nil
	c.Assert(cli.restConfig.APIPath, check.DeepEquals, expected.APIPath)
	c.Assert(cli.restConfig.Host, check.DeepEquals, expected.Host)
	c.Assert(cli.restConfig.TLSClientConfig, check.DeepEquals, expected.TLSClientConfig)
	c.Assert(cli.restConfig.Timeout, check.DeepEquals, expected.Timeout)
}

func (s *S) TestClusterInitClientByKubeConfig(c *check.C) {
	c1 := provTypes.Cluster{
		Name:        "c1",
		Default:     true,
		Provisioner: provisionerName,
		KubeConfig: &provTypes.KubeConfig{
			Cluster: clientcmdapi.Cluster{
				Server:                   "http://blah.com",
				CertificateAuthorityData: testCA,
			},
			AuthInfo: clientcmdapi.AuthInfo{
				ClientCertificateData: testCert,
				ClientKeyData:         testKey,
			},
		},
	}
	cli, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(cli.Interface, check.NotNil)
	c.Assert(cli.restConfig, check.NotNil)
	expected := &rest.Config{
		Host: "http://blah.com",
		TLSClientConfig: rest.TLSClientConfig{
			CAData: testCA,
		},
		Timeout: time.Minute,
	}
	c.Assert(cli.restConfig.Host, check.DeepEquals, expected.Host)
	c.Assert(cli.restConfig.Timeout, check.DeepEquals, expected.Timeout)
}

func (s *S) TestClusterInitClientByKubeConfigWithProxyANDTLS(c *check.C) {
	fakeK8SServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/namespaces/default/endpoints/kubernetes" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(&corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kubernetes",
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{
							{
								IP: "1.2.3.4",
							},
						},
					},
				},
			})
			return
		}
		http.Error(w, "Unepected path: "+r.URL.Path, http.StatusInternalServerError)
	}))
	fakeK8SServer.StartTLS()
	defer fakeK8SServer.Close()

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	fakeProxy := httptest.NewServer(proxy)
	defer fakeProxy.Close()

	c1 := provTypes.Cluster{
		Name:        "c1",
		Default:     true,
		Provisioner: provisionerName,
		KubeConfig: &provTypes.KubeConfig{
			Cluster: clientcmdapi.Cluster{
				Server:                fakeK8SServer.URL,
				InsecureSkipTLSVerify: true,
			},
			AuthInfo: clientcmdapi.AuthInfo{
				AuthProvider: &clientcmdapi.AuthProviderConfig{
					Name: "gcp-with-proxy",
					Config: map[string]string{
						"dry-run":    "true",
						"http-proxy": fakeProxy.URL,
					},
				},
			},
		},
		HTTPProxy: fakeProxy.URL,
	}
	restConfig, err := getRestConfigByKubeConfig(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(restConfig.APIPath, check.DeepEquals, "/api")

	k8s, err := kubernetes.NewForConfig(restConfig)
	c.Assert(err, check.IsNil)

	endpoint, err := k8s.CoreV1().Endpoints("default").Get(context.Background(), "kubernetes", metav1.GetOptions{})
	c.Assert(err, check.IsNil)
	c.Assert(endpoint.Subsets[0].Addresses[0].IP, check.Equals, "1.2.3.4")
}

func (s *S) TestClusterGetRestConfigMultipleAddrsRandom(c *check.C) {
	c1 := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1", "addr2"},
		Default:     true,
		Provisioner: provisionerName,
	}
	cfg, err := getRestConfig(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(cfg.Host, check.Equals, "addr1")
	cfg, err = getRestConfig(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(cfg.Host, check.Equals, "addr2")
}

func (s *S) TestClusterClientSetTimeout(c *check.C) {
	c1 := provTypes.Cluster{
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
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}}
	s.mockService.Cluster.OnFindByPool = func(_, _ string) (*provTypes.Cluster, error) {
		return &c1, nil
	}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err = s.p.Provision(context.TODO(), a)
	c.Assert(err, check.IsNil)
	ns, err := client.AppNamespace(context.TODO(), a)
	c.Assert(err, check.IsNil)
	c.Assert(ns, check.Equals, "default")
}

func (s *S) TestClusterNamespace(c *check.C) {
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "x"}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("mypool"), check.Equals, "x")
	c1 = provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": ""}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("mypool"), check.Equals, "default")
	c1 = provTypes.Cluster{Addresses: []string{"addr1"}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("mypool"), check.Equals, "default")
}

func (s *S) TestClusterNamespacePerPool(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "x"}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("mypool"), check.Equals, "x-mypool")
	c.Assert(client.PoolNamespace(""), check.Equals, "x")
	c1 = provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": ""}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("mypool"), check.Equals, "tsuru-mypool")
	c1 = provTypes.Cluster{Addresses: []string{"addr1"}}
	client, err = NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("mypool"), check.Equals, "tsuru-mypool")
	c.Assert(client.PoolNamespace(""), check.Equals, "tsuru")
}

func (s *S) TestClusterNamespacePerPoolWithInvalidCharacters(c *check.C) {
	config.Set("kubernetes:use-pool-namespaces", true)
	defer config.Unset("kubernetes:use-pool-namespaces")
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "tsuru"}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	c.Assert(client.PoolNamespace("my_pool has *INVALID* chars"), check.Equals, "tsuru-my-pool-has--invalid--chars")
}

func (s *S) TestClusterOvercommitFactor(c *check.C) {
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
		"overcommit-factor":         "2",
		"my-pool:overcommit-factor": "3",
		"invalid:overcommit-factor": "a",
		"float:overcommit-factor":   "1.5",
	}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	ovf, err := client.OvercommitFactor("my-pool")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(3))
	ovf, err = client.OvercommitFactor("global")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(2))
	ovf, err = client.OvercommitFactor("invalid")
	c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
	c.Assert(ovf, check.Equals, float64(0))
	ovf, err = client.OvercommitFactor("float")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(1.5))
}

func (s *S) TestClusterCPUOvercommitFactor(c *check.C) {
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
		"cpu-overcommit-factor":         "3",
		"my-pool:cpu-overcommit-factor": "4",
		"invalid:cpu-overcommit-factor": "a",
		"float:cpu-overcommit-factor":   "1.5",
	}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	ovf, err := client.CPUOvercommitFactor("my-pool")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(4))
	ovf, err = client.CPUOvercommitFactor("global")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(3))
	ovf, err = client.CPUOvercommitFactor("invalid")
	c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
	c.Assert(ovf, check.Equals, float64(0))
	ovf, err = client.CPUOvercommitFactor("float")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(1.5))
}

func (s *S) TestClusterMemoryOvercommitFactor(c *check.C) {
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
		"memory-overcommit-factor":         "3",
		"my-pool:memory-overcommit-factor": "4",
		"invalid:memory-overcommit-factor": "a",
		"float:memory-overcommit-factor":   "1.5",
	}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	ovf, err := client.MemoryOvercommitFactor("my-pool")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(4))
	ovf, err = client.MemoryOvercommitFactor("global")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(3))
	ovf, err = client.MemoryOvercommitFactor("invalid")
	c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
	c.Assert(ovf, check.Equals, float64(0))
	ovf, err = client.MemoryOvercommitFactor("float")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(1.5))
}

func (s *S) TestClusterCPUBurstFactor(c *check.C) {
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
		"cpu-burst-factor":         "3",
		"my-pool:cpu-burst-factor": "4",
		"invalid:cpu-burst-factor": "a",
		"float:cpu-burst-factor":   "1.5",
	}}
	client, err := NewClusterClient(&c1)
	c.Assert(err, check.IsNil)
	ovf, err := client.CPUBurstFactor("my-pool")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(4))
	ovf, err = client.CPUBurstFactor("global")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(3))
	ovf, err = client.CPUBurstFactor("invalid")
	c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
	c.Assert(ovf, check.Equals, float64(0))
	ovf, err = client.CPUBurstFactor("float")
	c.Assert(err, check.IsNil)
	c.Assert(ovf, check.Equals, float64(1.5))
}

func (s *S) TestClusterSinglePool(c *check.C) {
	tests := []struct {
		customData map[string]string
		expected   struct {
			val bool
			err bool
		}
	}{
		{
			customData: map[string]string{
				"single-pool": "",
			},
			expected: struct {
				val bool
				err bool
			}{false, false},
		},
		{
			customData: map[string]string{
				"single-pool": "true",
			},
			expected: struct {
				val bool
				err bool
			}{true, false},
		},
		{
			customData: map[string]string{
				"single-pool": "0",
			},
			expected: struct {
				val bool
				err bool
			}{false, false},
		},
		{
			customData: map[string]string{
				"single-pool": "a",
			},
			expected: struct {
				val bool
				err bool
			}{false, true},
		},
	}
	for _, tt := range tests {
		c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: tt.customData}
		client, err := NewClusterClient(&c1)
		c.Assert(err, check.IsNil)
		ovf, err := client.SinglePool()
		if tt.expected.err {
			c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
			c.Assert(ovf, check.Equals, false)
		} else {
			c.Assert(err, check.IsNil)
			c.Assert(ovf, check.Equals, tt.expected.val)
		}
	}
}

func (s *S) TestClusterAvoidMultipleServicesFlag(c *check.C) {
	tests := []struct {
		customData map[string]string
		expected   struct {
			val bool
			err bool
		}
	}{
		{
			customData: map[string]string{
				"enable-versioned-services": "",
			},
			expected: struct {
				val bool
				err bool
			}{false, false},
		},
		{
			customData: map[string]string{
				"enable-versioned-services": "true",
			},
			expected: struct {
				val bool
				err bool
			}{true, false},
		},
		{
			customData: map[string]string{
				"enable-versioned-services": "0",
			},
			expected: struct {
				val bool
				err bool
			}{false, false},
		},
		{
			customData: map[string]string{
				"enable-versioned-services": "a",
			},
			expected: struct {
				val bool
				err bool
			}{false, true},
		},
	}
	for _, tt := range tests {
		c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: tt.customData}
		client, err := NewClusterClient(&c1)
		c.Assert(err, check.IsNil)
		ovf, err := client.EnableVersionedServices()
		if tt.expected.err {
			c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
			c.Assert(ovf, check.Equals, false)
		} else {
			c.Assert(err, check.IsNil)
			c.Assert(ovf, check.Equals, tt.expected.val)
		}
	}
}

func (s *S) TestClusterBaseServiceAnnotations(c *check.C) {
	tests := []struct {
		customData          map[string]string
		expectedAnnotations map[string]string
		expectedErr         bool
	}{
		{
			customData:          map[string]string{},
			expectedAnnotations: nil,
		},
		{
			customData: map[string]string{
				"base-services-annotations": "",
			},
			expectedAnnotations: nil,
		},
		{
			customData: map[string]string{
				"base-services-annotations": `{"xpto.io/name": "custom-name"}`,
			},
			expectedAnnotations: map[string]string{
				"xpto.io/name": "custom-name",
			},
		},
		{
			customData: map[string]string{
				"base-services-annotations": `xpto.io/name: custom-name
abc.io/name: test`,
			},
			expectedAnnotations: map[string]string{
				"abc.io/name":  "test",
				"xpto.io/name": "custom-name",
			},
		},
	}
	for _, tt := range tests {
		c1 := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: tt.customData}
		client, err := NewClusterClient(&c1)
		c.Assert(err, check.IsNil)
		annotations, err := client.ServiceAnnotations(baseServicesAnnotations)
		if tt.expectedErr {
			c.Assert(err, check.ErrorMatches, ".*invalid syntax.*")
			c.Assert(annotations, check.Equals, nil)
		} else {
			c.Assert(err, check.IsNil)
			c.Assert(annotations, check.DeepEquals, tt.expectedAnnotations)
		}

	}

}

func (s *S) TestClustersForApps(c *check.C) {
	c1 := provTypes.Cluster{
		Name:        "c1",
		Addresses:   []string{"addr1"},
		Default:     true,
		Provisioner: provisionerName,
	}
	c2 := provTypes.Cluster{
		Name:        "c2",
		Addresses:   []string{"addr2"},
		Pools:       []string{"p1", "p2"},
		Provisioner: provisionerName,
	}
	s.mockService.Cluster.OnFindByPools = func(prov string, pools []string) (map[string]provTypes.Cluster, error) {
		sort.Strings(pools)
		c.Assert(pools, check.DeepEquals, []string{"abc", "deleted-pool", "p1", "p2", "xyz"})
		return map[string]provTypes.Cluster{
			"p1":  c2,
			"p2":  c2,
			"xyz": c1,
			"abc": c1,
		}, nil
	}
	a1 := provisiontest.NewFakeApp("myapp1", "python", 0)
	a1.Pool = "xyz"
	a2 := provisiontest.NewFakeApp("myapp2", "python", 0)
	a2.Pool = "p1"
	a3 := provisiontest.NewFakeApp("myapp3", "python", 0)
	a3.Pool = "p2"
	a4 := provisiontest.NewFakeApp("myapp4", "python", 0)
	a4.Pool = "abc"
	a5 := provisiontest.NewFakeApp("myapp5", "python", 0)
	a5.Pool = "deleted-pool"
	cApps, err := clustersForApps(context.TODO(), []provision.App{a1, a2, a3, a4, a5})
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

func (s *S) TestClusterDisablePDB(c *check.C) {
	c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
	c.Assert(err, check.IsNil)
	c.Assert(c1.disablePDB("mypool"), check.Equals, false)
	c2, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"disable-pdb": "true", "mypool2:disable-pdb": "false"}})
	c.Assert(err, check.IsNil)
	c.Assert(c2.disablePDB("mypool"), check.Equals, true)
	c3, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"disable-pdb": "false", "mypool:disable-pdb": "true"}})
	c.Assert(err, check.IsNil)
	c.Assert(c3.disablePDB("mypool"), check.Equals, true)
	c4, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"disable-pdb": "true", "mypool:disable-pdb": "false"}})
	c.Assert(err, check.IsNil)
	c.Assert(c4.disablePDB("mypool"), check.Equals, false)
}

func (s *S) TestCluster_Registry(c *check.C) {
	c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
	c.Assert(err, check.IsNil)
	c.Assert(string(c1.Registry()), check.Equals, "")

	c1, err = NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"registry": "169.196.0.100:5000/tsuru"}})
	c.Assert(err, check.IsNil)
	c.Assert(string(c1.Registry()), check.Equals, "169.196.0.100:5000/tsuru")
}

func (s *S) TestCluster_InsecureRegistry(c *check.C) {
	c1, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
	c.Assert(err, check.IsNil)
	c.Assert(c1.InsecureRegistry(), check.Equals, false)

	c1, err = NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"registry-insecure": "true"}})
	c.Assert(err, check.IsNil)
	c.Assert(c1.InsecureRegistry(), check.Equals, true)

	c1, err = NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"registry-insecure": "false"}})
	c.Assert(err, check.IsNil)
	c.Assert(c1.InsecureRegistry(), check.Equals, false)
}
