// Copyright 2017 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubernetes

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/provision/provisiontest"
	appTypes "github.com/tsuru/tsuru/types/app"
	provTypes "github.com/tsuru/tsuru/types/provision"
	check "gopkg.in/check.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func (s *S) TestClusterAppNamespace(_ *check.C) {
	c1 := provTypes.Cluster{Addresses: []string{"addr1"}}
	s.mockService.Cluster.OnFindByPool = func(_, _ string) (*provTypes.Cluster, error) {
		return &c1, nil
	}
	client, err := NewClusterClient(&c1)
	require.NoError(s.t, err)
	a := provisiontest.NewFakeApp("myapp", "python", 0)
	err = s.p.Provision(context.TODO(), a)
	require.NoError(s.t, err)
	ns, err := client.AppNamespace(context.TODO(), a)
	require.NoError(s.t, err)
	require.Equal(s.t, "default", ns)
}

func (s *S) TestClustersForApps(_ *check.C) {
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
		require.EqualValues(s.t, []string{"abc", "deleted-pool", "p1", "p2", "xyz"}, pools)
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
	cApps, err := clustersForApps(context.TODO(), []*appTypes.App{a1, a2, a3, a4, a5})
	require.NoError(s.t, err)
	require.Len(s.t, cApps, 2)
	sort.Slice(cApps, func(i, j int) bool {
		return cApps[i].client.Name < cApps[j].client.Name
	})
	require.Equal(s.t, "c1", cApps[0].client.Name)
	require.Equal(s.t, "c2", cApps[1].client.Name)
	for idx := range cApps {
		sort.Slice(cApps[idx].apps, func(i, j int) bool {
			return cApps[idx].apps[i].Name < cApps[idx].apps[j].Name
		})
	}
	require.EqualValues(s.t, []*appTypes.App{a1, a4}, cApps[0].apps)
	require.EqualValues(s.t, []*appTypes.App{a2, a3}, cApps[1].apps)
}

func TestClusterClient(t *testing.T) {
	t.Run("Cluster Init Client", func(t *testing.T) {
		c := provTypes.Cluster{
			Name:        "c1",
			Addresses:   []string{"addr1"},
			CaCert:      testCA,
			ClientCert:  testCert,
			ClientKey:   testKey,
			Default:     true,
			Provisioner: provisionerName,
		}
		cli, err := NewClusterClient(&c)
		require.NoError(t, err)
		require.NotNil(t, cli.Interface)
		require.NotNil(t, cli.restConfig)
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
		require.Equal(t, expected.APIPath, cli.restConfig.APIPath)
		require.Equal(t, expected.Host, cli.restConfig.Host)
		require.Equal(t, expected.Timeout, cli.restConfig.Timeout)
		require.EqualValues(t, expected.TLSClientConfig, cli.restConfig.TLSClientConfig)
	})

	t.Run("Cluster Init Client By Kube Config", func(t *testing.T) {
		c := provTypes.Cluster{
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
		cli, err := NewClusterClient(&c)
		require.NoError(t, err)
		require.NotNil(t, cli.Interface)
		require.NotNil(t, cli.restConfig)
		expected := &rest.Config{
			Host: "http://blah.com",
			TLSClientConfig: rest.TLSClientConfig{
				CAData: testCA,
			},
			Timeout: time.Minute,
		}
		require.Equal(t, expected.Host, cli.restConfig.Host)
		require.Equal(t, expected.Timeout, cli.restConfig.Timeout)
	})

	t.Run("Cluster Init Client By Kube Config With Proxy and TLS", func(t *testing.T) {
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

		c := provTypes.Cluster{
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
		restConfig, err := getRestConfigByKubeConfig(&c)
		require.NoError(t, err)
		require.Equal(t, "/api", restConfig.APIPath)

		k8s, err := kubernetes.NewForConfig(restConfig)
		require.NoError(t, err)

		endpoint, err := k8s.CoreV1().Endpoints("default").Get(context.Background(), "kubernetes", metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, "1.2.3.4", endpoint.Subsets[0].Addresses[0].IP)
	})

	t.Run("Cluster Get Rest Config Multiple Addrs Random", func(t *testing.T) {
		c := provTypes.Cluster{
			Name:        "c1",
			Addresses:   []string{"addr1", "addr2"},
			Default:     true,
			Provisioner: provisionerName,
		}
		// reinitialize rand seed
		randomGenerator = rand.New(rand.NewSource(3))
		defer func() {
			randomGenerator = nil
		}()

		cfg, err := getRestConfig(&c)
		require.NoError(t, err)
		require.Equal(t, "addr1", cfg.Host)
		cfg, err = getRestConfig(&c)
		require.NoError(t, err)
		require.Equal(t, "addr2", cfg.Host)
	})

	t.Run("Cluster Client Set Timeout", func(t *testing.T) {
		c1 := provTypes.Cluster{
			Name:        "c1",
			Addresses:   []string{"addr1", "addr2"},
			Default:     true,
			Provisioner: provisionerName,
		}
		client, err := NewClusterClient(&c1)
		require.NoError(t, err)
		client.SetTimeout(time.Hour)
		require.Equal(t, time.Hour, client.restConfig.Timeout)
	})

	t.Run("Cluster Client Config Precedente Max Surge", func(t *testing.T) {
		client, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
		require.NoError(t, err)
		require.Equal(t, intstr.FromString("100%"), client.maxSurge("mypool"))
		config.Set("clusters:defaults:max-surge", "1%")
		defer config.Unset("clusters:defaults:max-surge")
		client, err = NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
		require.NoError(t, err)
		require.Equal(t, intstr.FromString("1%"), client.maxSurge("mypool"))
		client, err = NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"max-surge": "20%", "mypool2:max-surge": "30%"}})
		require.NoError(t, err)
		require.Equal(t, intstr.FromString("20%"), client.maxSurge("mypool"))
		require.Equal(t, intstr.FromString("30%"), client.maxSurge("mypool2"))
	})

	// here

	t.Run("Cluster Namespace", func(t *testing.T) {
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "x"}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "x", client.PoolNamespace("mypool"))
		c = provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": ""}}
		client, err = NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "default", client.PoolNamespace("mypool"))
		c = provTypes.Cluster{Addresses: []string{"addr1"}}
		client, err = NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "default", client.PoolNamespace("mypool"))
	})

	t.Run("Cluster Namespace Per Pool", func(t *testing.T) {
		config.Set("kubernetes:use-pool-namespaces", true)
		defer config.Unset("kubernetes:use-pool-namespaces")
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "x"}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "x-mypool", client.PoolNamespace("mypool"))
		require.Equal(t, "x", client.PoolNamespace(""))
		c = provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": ""}}
		client, err = NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "tsuru-mypool", client.PoolNamespace("mypool"))
		c = provTypes.Cluster{Addresses: []string{"addr1"}}
		client, err = NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "tsuru-mypool", client.PoolNamespace("mypool"))
		require.Equal(t, "tsuru", client.PoolNamespace(""))
	})

	t.Run("Cluster Namespace Per Pool With Invalid Characters", func(t *testing.T) {
		config.Set("kubernetes:use-pool-namespaces", true)
		defer config.Unset("kubernetes:use-pool-namespaces")
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"namespace": "tsuru"}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		require.Equal(t, "tsuru-my-pool-has--invalid--chars", client.PoolNamespace("my_pool has *INVALID* chars"))
	})

	t.Run("Cluster Overcommit Factor", func(t *testing.T) {
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
			"overcommit-factor":         "2",
			"my-pool:overcommit-factor": "3",
			"invalid:overcommit-factor": "a",
			"float:overcommit-factor":   "1.5",
		}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		ovf, err := client.OvercommitFactor("my-pool")
		require.NoError(t, err)
		require.Equal(t, float64(3), ovf)
		ovf, err = client.OvercommitFactor("global")
		require.NoError(t, err)
		require.Equal(t, float64(2), ovf)
		ovf, err = client.OvercommitFactor("invalid")
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid syntax")
		require.Equal(t, float64(0), ovf)
		ovf, err = client.OvercommitFactor("float")
		require.NoError(t, err)
		require.Equal(t, float64(1.5), ovf)
	})

	t.Run("Cluster CPU Overcommit Factor", func(t *testing.T) {
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
			"cpu-overcommit-factor":         "3",
			"my-pool:cpu-overcommit-factor": "4",
			"invalid:cpu-overcommit-factor": "a",
			"float:cpu-overcommit-factor":   "1.5",
		}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		ovf, err := client.CPUOvercommitFactor("my-pool")
		require.NoError(t, err)
		require.Equal(t, float64(4), ovf)
		ovf, err = client.CPUOvercommitFactor("global")
		require.NoError(t, err)
		require.Equal(t, float64(3), ovf)
		ovf, err = client.CPUOvercommitFactor("invalid")
		require.ErrorContains(t, err, "invalid syntax")
		require.Equal(t, float64(0), ovf)
		ovf, err = client.CPUOvercommitFactor("float")
		require.NoError(t, err)
		require.Equal(t, float64(1.5), ovf)
	})

	t.Run("Cluster Memory Overcommit Factor", func(t *testing.T) {
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
			"memory-overcommit-factor":         "3",
			"my-pool:memory-overcommit-factor": "4",
			"invalid:memory-overcommit-factor": "a",
			"float:memory-overcommit-factor":   "1.5",
		}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		ovf, err := client.MemoryOvercommitFactor("my-pool")
		require.NoError(t, err)
		require.Equal(t, float64(4), ovf)
		ovf, err = client.MemoryOvercommitFactor("global")
		require.NoError(t, err)
		require.Equal(t, float64(3), ovf)
		ovf, err = client.MemoryOvercommitFactor("invalid")
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid syntax")
		require.Equal(t, float64(0), ovf)
		ovf, err = client.MemoryOvercommitFactor("float")
		require.NoError(t, err)
		require.Equal(t, float64(1.5), ovf)
	})

	t.Run("Cluster CPU Burst Factor", func(t *testing.T) {
		c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{
			"cpu-burst-factor":         "3",
			"my-pool:cpu-burst-factor": "4",
			"invalid:cpu-burst-factor": "a",
			"float:cpu-burst-factor":   "1.5",
		}}
		client, err := NewClusterClient(&c)
		require.NoError(t, err)
		ovf, err := client.CPUBurstFactor("my-pool")
		require.NoError(t, err)
		require.Equal(t, float64(4), ovf)
		ovf, err = client.CPUBurstFactor("global")
		require.NoError(t, err)
		require.Equal(t, float64(3), ovf)
		ovf, err = client.CPUBurstFactor("invalid")
		require.ErrorContains(t, err, "invalid syntax")
		require.Equal(t, float64(0), ovf)
		ovf, err = client.CPUBurstFactor("float")
		require.NoError(t, err)
		require.Equal(t, float64(1.5), ovf)
	})

	t.Run("Cluster single pool", func(t *testing.T) {
		t.Run("Empty value", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"single-pool": ""}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			ovf, err := client.SinglePool()
			require.NoError(t, err)
			require.False(t, ovf)
		})
		t.Run("True value", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"single-pool": "true"}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			ovf, err := client.SinglePool()
			require.NoError(t, err)
			require.True(t, ovf)
		})
		t.Run("Zero value", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"single-pool": "0"}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			ovf, err := client.SinglePool()
			require.NoError(t, err)
			require.False(t, ovf)
		})
		t.Run("Invalid value", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"single-pool": "a"}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			ovf, err := client.SinglePool()
			require.ErrorContains(t, err, "invalid syntax")
			require.False(t, ovf)
		})
	})

	t.Run("Cluster versioned services", func(t *testing.T) {
		t.Run("Empty value", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"enable-versioned-services": ""}})
			require.NoError(t, err)
			val, err := c.EnableVersionedServices()
			require.NoError(t, err)
			require.Equal(t, false, val)
		})
		t.Run("True value", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"enable-versioned-services": "true"}})
			require.NoError(t, err)
			val, err := c.EnableVersionedServices()
			require.NoError(t, err)
			require.Equal(t, true, val)
		})
		t.Run("Zero value", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"enable-versioned-services": "0"}})
			require.NoError(t, err)
			val, err := c.EnableVersionedServices()
			require.NoError(t, err)
			require.Equal(t, false, val)
		})
		t.Run("Invalid value", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"enable-versioned-services": "a"}})
			require.NoError(t, err)
			val, err := c.EnableVersionedServices()
			require.Error(t, err)
			require.ErrorContains(t, err, "invalid syntax")
			require.False(t, val)
		})
	})

	t.Run("Cluster Base Service Annotations", func(t *testing.T) {
		t.Run("Default no annotations", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			annotations, err := client.ServiceAnnotations(baseServicesAnnotations)
			require.NoError(t, err)
			require.Nil(t, annotations)
		})
		t.Run("Empty value anotations should not be persisted", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"base-services-annotations": ""}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			annotations, err := client.ServiceAnnotations(baseServicesAnnotations)
			require.NoError(t, err)
			require.Nil(t, annotations)
		})
		t.Run("Should parse JSON annotations", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"base-services-annotations": `{"xpto.io/name": "custom-name"}`}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			annotations, err := client.ServiceAnnotations(baseServicesAnnotations)
			require.NoError(t, err)
			require.EqualValues(t, map[string]string{"xpto.io/name": "custom-name"}, annotations)
		})
		t.Run("Should parse YAML annotations", func(t *testing.T) {
			c := provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"base-services-annotations": `xpto.io/name: custom-name
abc.io/name: test`}}
			client, err := NewClusterClient(&c)
			require.NoError(t, err)
			annotations, err := client.ServiceAnnotations(baseServicesAnnotations)
			require.NoError(t, err)
			require.EqualValues(t, map[string]string{
				"abc.io/name":  "test",
				"xpto.io/name": "custom-name",
			}, annotations)
		})
	})

	t.Run("Disable PDB", func(t *testing.T) {
		t.Run("PDB Enabled by default", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
			require.NoError(t, err)
			require.False(t, c.disablePDB("mypool"))
		})
		t.Run("Cluster wide PDB disabled", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"disable-pdb": "true", "mypool2:disable-pdb": "false"}})
			require.NoError(t, err)
			require.True(t, c.disablePDB("mypool"))
		})
		t.Run("Pool specific PDB disabled", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"disable-pdb": "false", "mypool:disable-pdb": "true"}})
			require.NoError(t, err)
			require.True(t, c.disablePDB("mypool"))
		})
		t.Run("Pool specific PDB enabled", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"disable-pdb": "true", "mypool:disable-pdb": "false"}})
			require.NoError(t, err)
			require.False(t, c.disablePDB("mypool"))
		})
	})

	t.Run("Cluster Registry", func(t *testing.T) {
		t.Run("Default empty registry", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
			require.NoError(t, err)
			require.Equal(t, "", string(c.Registry()))
		})
		t.Run("Defined registry", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"registry": "169.196.0.100:5000/tsuru"}})
			require.NoError(t, err)
			require.Equal(t, "169.196.0.100:5000/tsuru", string(c.Registry()))
		})
	})

	t.Run("Insecure Cluster Registry", func(t *testing.T) {
		t.Run("Default secure registry", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}})
			require.NoError(t, err)
			require.False(t, c.InsecureRegistry())
		})
		t.Run("Insecure registry", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"registry-insecure": "true"}})
			require.NoError(t, err)
			require.True(t, c.InsecureRegistry())
		})
		t.Run("Secure registry", func(t *testing.T) {
			c, err := NewClusterClient(&provTypes.Cluster{Addresses: []string{"addr1"}, CustomData: map[string]string{"registry-insecure": "false"}})
			require.NoError(t, err)
			require.False(t, c.InsecureRegistry())
		})
	})
}
