// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/swarm"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/fsouza/go-dockerclient/testing"
	"github.com/kr/pretty"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/app"
	"github.com/tsuru/tsuru/app/image"
	"github.com/tsuru/tsuru/provision"
	"github.com/tsuru/tsuru/provision/nodecontainer"
	"github.com/tsuru/tsuru/provision/servicecommon"
	check "gopkg.in/check.v1"
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
	testServerCert = []byte(`-----BEGIN CERTIFICATE-----
MIIC+TCCAeGgAwIBAgIPcIPq45AlSN+Itq+6Ap/dMA0GCSqGSIb3DQEBCwUAMBUx
EzARBgNVBAoTCnRzdXJ1IEluYy4wHhcNMTYxMTIxMTg1NDExWhcNMjYxMTE5MTg1
NDExWjAVMRMwEQYDVQQKEwp0c3VydSBJbmMuMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEAyilhi42eWUr2ihmftZrjqD24CPo1bJYtGdL4+4+bXlKvpDSN
BADXoyLqDNjOl1ohwmYPR2POqA7HjzNJW3BCMXDHd1SUZF0vTB/HYEEHt4kD/DlQ
uujjQZ7dSeVFjZhazNP43Gp+DYTMlSB1sFriG82uIugIBzfObZxxWb+q93s/d2lU
HLJv/1Eep1K66A+TEkyEka6KuNs6s2gc2hutqX4krHGaBOCEM1kBw4yzpu0wi8YL
Z8Icv+MyAvvXVM5q11b1SAEbOJP32eJOU/NmjtJO772maU8CFh//t8pBV+m9HZPI
hX04f3Vj2fH2aWBosvXOL779vYJI5QC3w8DGcQIDAQABo0YwRDAOBgNVHQ8BAf8E
BAMCB4AwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDAYDVR0TAQH/BAIwADAPBgNVHREE
CDAGhwR/AAABMA0GCSqGSIb3DQEBCwUAA4IBAQB2R1GNW39NbvKVwEHJ0lDSNbxV
5b2Eucg1nHZ1FxnroC5nha53+ew7a4m3IFejAKzRICG7Kfg6Mb7uq+B9EZG2Xk+1
/6CFgzScWeP4uYpj5T+rTNgNUTif02xJWTt6bRCR1ja0ZJm60EcJPAUbs3356JTc
U+VZCeXDg0edHlEkrrQykw1nfLr1N+1IRB7+0vU1HtYdFIAyJz9bOS7CR/JLxfX0
kL0ycRKXpcLCUip29fQd1A0B9Tziz/wCm2PeRIwC25XenNAgRyJWzPVJPVvfJF9E
p7CqJZIvYUuMrGpjHJ6B5E76OUtcXONftzAjb/xmgoCKXgwwiehcivauMvBx
-----END CERTIFICATE-----`)
	testServerKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAyilhi42eWUr2ihmftZrjqD24CPo1bJYtGdL4+4+bXlKvpDSN
BADXoyLqDNjOl1ohwmYPR2POqA7HjzNJW3BCMXDHd1SUZF0vTB/HYEEHt4kD/DlQ
uujjQZ7dSeVFjZhazNP43Gp+DYTMlSB1sFriG82uIugIBzfObZxxWb+q93s/d2lU
HLJv/1Eep1K66A+TEkyEka6KuNs6s2gc2hutqX4krHGaBOCEM1kBw4yzpu0wi8YL
Z8Icv+MyAvvXVM5q11b1SAEbOJP32eJOU/NmjtJO772maU8CFh//t8pBV+m9HZPI
hX04f3Vj2fH2aWBosvXOL779vYJI5QC3w8DGcQIDAQABAoIBAD84h6/Lvvxvq//u
GXsCkDVZ78am8LQflsUfrAuHknAB7bmtUXgyBz2WOpl/58N/RVV080xBEyyNSq0m
vcchqSGrAkX4JlvopFTrDz+ztoUYDS4AgpWhJQitdMiaMZEhVyv9EjNj/j2eDRiJ
ySQ4l8NYJB/4biJLunue0/fcL8wqqWnXPFiMChFT5LS7LhQqKfbLghydzEKr81jG
mw/cRUZi263M8w99p5aqwUrt6aezB+xqM4AnqG9RXAMh/zq8IgGOIerckN9SeCtk
VvTUi0I61bE5TCandkFni/NpVxZvj0QVIL3aawcXrgZVUyUY7TGkD1MqPqVGQiLU
C+Zcf50CgYEA2B/ZvOU8dORngeWP0Y2sFTdnG5Bqtrhy3m198bXABiNFb073N+RT
pnJA9t2j13whb2MUd3Zxi7QNRfWDkeQl5Bxyn2TvNxepFYWKEVNucSLe6SG6ylWf
ffqu7yum4NS/PBme13EmOnz0UXQmQrCyVqe5hXX+H1glLllix5uD5GcCgYEA73YH
zOFLTfCbRPA6kFOT/fMCk9WispuQ/jIHpiI0mAPT7nPdxlXzw86Hn7SePq/2/XWP
kP2HXnvLMDWD7uTxkEOz1xFEe35+pQ0T+K/8Ds5Qp8Sn2L4QzvQ8P7ag4PvLajL6
3VTnLPTeKirloIe6OG8Fj+VKFUY2c63TvVbcd2cCgYBhu2V3KiKAqZi1AN5cYLhk
j70slc3r+tTXCKRfXVUMcX7AqvDYcYPyTNBb0jZ5B0UHXcKvkvwdtLob3L42hvkr
gkHDGp2iSCzJ8q1Q0G2s85vhyMLzJG0PRwE8Xn0ERrCDuQI/YodrA35oJyH2HnlG
/mnClGzqN634m6szoHuwGQKBgQCGLumoEQcVoaIgO01V2r+vKiFjne8RjsLs7jQD
EF/QXzS/BgZcQYXbTzwIbjnOfuQ0m0/bu3XDqDLvzM0lbP1ADfAUsARj/zoQWwe5
70ObOFlR6Yz0k2zvy0SHn1r/N5mA5RhWNmFke8KSdn8+OVBMl0nSnHWq/jE9GUbx
bl8UOQKBgAzXEdpWm8BuZsRvVGwbDM+Li5de254Jk6unUkDYcuk+pOqoRa6iAw2A
A2B3fIz4r3Q6772hpZvhx4Tkx/Cb9o5Mc6NpgUYD8seIqggxg3S4NfLoBYL5b2Fc
X46PqW9WOcNb3P7CbXwhcOTwXpPtGqWdm+O5rjZ287D9jhfBcWoq
-----END RSA PRIVATE KEY-----`)
)

func (s *S) TestNewClient(c *check.C) {
	srv, err := testing.NewServer("127.0.0.1:0", nil, nil)
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	cli, err := newClient(srv.URL(), nil)
	c.Assert(err, check.IsNil)
	err = cli.Ping()
	c.Assert(err, check.IsNil)
	httpTrans, ok := cli.HTTPClient.Transport.(*http.Transport)
	c.Assert(ok, check.Equals, true)
	c.Assert(httpTrans.DisableKeepAlives, check.Equals, true)
	c.Assert(httpTrans.MaxIdleConnsPerHost, check.Equals, -1)
}

func (s *S) TestNewClientTLSConfig(c *check.C) {
	caPath := tmpFileWith(c, testCA)
	certPath := tmpFileWith(c, testServerCert)
	keyPath := tmpFileWith(c, testServerKey)
	defer os.Remove(certPath)
	defer os.Remove(keyPath)
	defer os.Remove(caPath)
	srv, err := testing.NewTLSServer("127.0.0.1:0", nil, nil, testing.TLSConfig{
		RootCAPath:  caPath,
		CertPath:    certPath,
		CertKeyPath: keyPath,
	})
	c.Assert(err, check.IsNil)
	defer srv.Stop()
	url := srv.URL()
	url = strings.Replace(url, "http://", "https://", 1)
	tlsCert, err := tls.X509KeyPair([]byte(testCert), []byte(testKey))
	c.Assert(err, check.IsNil)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM([]byte(testCA))
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      caPool,
	}
	cli, err := newClient(url, tlsConfig)
	c.Assert(err, check.IsNil)
	c.Assert(cli.TLSConfig.Certificates, check.HasLen, 1)
	c.Assert(cli.TLSConfig.RootCAs, check.NotNil)
	err = cli.Ping()
	c.Assert(err, check.IsNil)
}

func (s *S) TestServiceSpecForApp(c *check.C) {
	intPtr := func(i uint64) *uint64 {
		return &i
	}
	s.addCluster(c)
	a := &app.App{Name: "myapp", TeamOwner: s.team.Name, Deploys: 1}
	err := app.CreateApp(a, s.user)
	c.Assert(err, check.IsNil)
	err = image.SaveImageCustomData("myapp:v1", map[string]interface{}{
		"processes": map[string]interface{}{
			"web": "python myapp.py",
		},
	})
	c.Assert(err, check.IsNil)
	base := func() swarm.ServiceSpec {
		return swarm.ServiceSpec{
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Image: "myapp:v1",
					Env: []string{
						"TSURU_SERVICES={}",
						"TSURU_PROCESSNAME=web",
						"TSURU_HOST=http://tsuruhost",
						"port=8888",
						"PORT=8888",
					},
					Labels: map[string]string{
						"tsuru.provisioner":          "swarm",
						"tsuru.builder":              "",
						"tsuru.app-process":          "web",
						"tsuru.app-pool":             "bonehunters",
						"tsuru.is-stopped":           "false",
						"tsuru.router-type":          "fake",
						"tsuru.is-isolated-run":      "false",
						"tsuru.app-process-replicas": "0",
						"tsuru.app-name":             "myapp",
						"tsuru.is-deploy":            "false",
						"tsuru.is-tsuru":             "true",
						"tsuru.is-service":           "true",
						"tsuru.is-build":             "false",
						"tsuru.app-platform":         "",
						"tsuru.router-name":          "fake",
					},
					Command: []string{
						"/bin/sh",
						"-lc",
						"[ -d /home/application/current ] && cd /home/application/current; curl -sSL -m15 -XPOST -d\"hostname=$(hostname)\" -o/dev/null -H\"Content-Type:application/x-www-form-urlencoded\" -H\"Authorization:bearer \" http://tsuruhost/apps/myapp/units/register || true && exec python myapp.py",
					},
				},
				Networks: []swarm.NetworkAttachmentConfig{
					{Target: "app-myapp-overlay"},
				},
				RestartPolicy: &swarm.RestartPolicy{
					Condition: swarm.RestartPolicyConditionAny,
				},
				Placement: &swarm.Placement{
					Constraints: []string{
						"node.labels.tsuru.pool == bonehunters",
					},
				},
			},
			EndpointSpec: &swarm.EndpointSpec{
				Mode: swarm.ResolutionModeVIP,
				Ports: []swarm.PortConfig{
					{TargetPort: 8888, PublishedPort: 0},
				},
			},
			Annotations: swarm.Annotations{
				Name: "myapp-web",
				Labels: map[string]string{
					"tsuru.provisioner":          "swarm",
					"tsuru.builder":              "",
					"tsuru.app-process":          "web",
					"tsuru.app-pool":             "bonehunters",
					"tsuru.is-stopped":           "false",
					"tsuru.router-type":          "fake",
					"tsuru.is-isolated-run":      "false",
					"tsuru.app-process-replicas": "0",
					"tsuru.app-name":             "myapp",
					"tsuru.is-deploy":            "false",
					"tsuru.is-tsuru":             "true",
					"tsuru.is-service":           "true",
					"tsuru.is-build":             "false",
					"tsuru.app-platform":         "",
					"tsuru.router-name":          "fake",
				},
			},
			Mode: swarm.ServiceMode{
				Replicated: &swarm.ReplicatedService{
					Replicas: intPtr(0),
				},
			},
		}
	}
	tests := []struct {
		opts     tsuruServiceOpts
		expected swarm.ServiceSpec
	}{
		{
			opts: tsuruServiceOpts{
				app:     a,
				image:   "myapp:v1",
				process: "web",
			},
			expected: base(),
		},
		{
			opts: tsuruServiceOpts{
				app:        a,
				image:      "myapp:v1",
				isDeploy:   true,
				buildImage: "myapp:v2",
			},
			expected: func() swarm.ServiceSpec {
				tt := base()
				tt.Annotations.Name = "myapp--build"
				tt.Annotations.Labels["tsuru.is-deploy"] = "true"
				tt.Annotations.Labels["tsuru.app-process"] = ""
				tt.Annotations.Labels["tsuru.build-image"] = "myapp:v2"
				tt.TaskTemplate.ContainerSpec.Labels = tt.Annotations.Labels
				tt.TaskTemplate.ContainerSpec.Command = nil
				tt.TaskTemplate.ContainerSpec.Env = []string{"TSURU_HOST=http://tsuruhost"}
				tt.TaskTemplate.Networks = nil
				tt.TaskTemplate.LogDriver = &swarm.Driver{Name: "json-file"}
				tt.Mode.Replicated.Replicas = intPtr(1)
				tt.EndpointSpec = nil
				return tt
			}(),
		},
		{
			opts: tsuruServiceOpts{
				app:           a,
				image:         "myapp:v1",
				isIsolatedRun: true,
			},
			expected: func() swarm.ServiceSpec {
				tt := base()
				tt.Annotations.Name = "myapp-isolated-run"
				tt.Annotations.Labels["tsuru.is-isolated-run"] = "true"
				tt.Annotations.Labels["tsuru.app-process"] = ""
				tt.TaskTemplate.ContainerSpec.Labels = tt.Annotations.Labels
				tt.TaskTemplate.ContainerSpec.Command = nil
				tt.TaskTemplate.ContainerSpec.Env[1] = "TSURU_PROCESSNAME="
				tt.TaskTemplate.Networks = nil
				tt.Mode.Replicated.Replicas = intPtr(1)
				tt.EndpointSpec = nil
				return tt
			}(),
		},
		{
			opts: tsuruServiceOpts{
				app:      a,
				image:    "myapp:v1",
				process:  "web",
				replicas: 9,
			},
			expected: func() swarm.ServiceSpec {
				tt := base()
				tt.Mode.Replicated.Replicas = intPtr(9)
				return tt
			}(),
		},
		{
			opts: tsuruServiceOpts{
				app:     a,
				image:   "myapp:v1",
				process: "web",
				labels: func() *provision.LabelSet {
					b := base()
					b.Annotations.Labels["tsuru.other"] = "val"
					return &provision.LabelSet{
						Prefix: tsuruLabelPrefix,
						Labels: b.Annotations.Labels,
					}
				}(),
				replicas: 8,
			},
			expected: func() swarm.ServiceSpec {
				tt := base()
				tt.Annotations.Labels["tsuru.other"] = "val"
				tt.TaskTemplate.ContainerSpec.Labels = tt.Annotations.Labels
				tt.Mode.Replicated.Replicas = intPtr(8)
				return tt
			}(),
		},
	}
	for _, tt := range tests {
		var spec *swarm.ServiceSpec
		spec, err = serviceSpecForApp(tt.opts)
		c.Assert(err, check.IsNil)
		c.Assert(spec, check.DeepEquals, &tt.expected, check.Commentf("Diff %#v\n", pretty.Diff(spec, &tt.expected)))
	}
}

func (s *S) TestServiceSpecForNodeContainer(c *check.C) {
	c1 := nodecontainer.NodeContainerConfig{
		Name: "swarmbs",
		Config: docker.Config{
			Image: "bsimg",
			Env: []string{
				"A=1",
				"B=2",
			},
			Labels: map[string]string{"label1": "val1"},
		},
		HostConfig: docker.HostConfig{
			RestartPolicy: docker.AlwaysRestart(),
			Privileged:    true,
			Binds:         []string{"/xyz:/abc:ro"},
		},
	}
	err := nodecontainer.AddNewContainer("", &c1)
	c.Assert(err, check.IsNil)
	loadedC1, err := nodecontainer.LoadNodeContainer("", "swarmbs")
	c.Assert(err, check.IsNil)
	serviceSpec, err := serviceSpecForNodeContainer(loadedC1, "", servicecommon.PoolFilter{})
	c.Assert(err, check.IsNil)
	expectedLabels := map[string]string{
		"tsuru.is-tsuru":            "true",
		"tsuru.is-node-container":   "true",
		"tsuru.provisioner":         "swarm",
		"tsuru.label1":              "val1",
		"tsuru.node-container-name": "swarmbs",
		"tsuru.node-container-pool": "",
	}
	expected := &swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name:   "node-container-swarmbs-all",
			Labels: expectedLabels,
		},
		Mode: swarm.ServiceMode{Global: &swarm.GlobalService{}},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image:  "bsimg",
				Env:    []string{"A=1", "B=2"},
				Labels: expectedLabels,
				Mounts: []mount.Mount{
					{
						Type:     mount.TypeBind,
						Source:   "/xyz",
						Target:   "/abc",
						ReadOnly: true,
					},
				},
			},
			Placement: &swarm.Placement{Constraints: []string(nil)},
		},
	}
	c.Assert(serviceSpec, check.DeepEquals, expected)
	err = nodecontainer.AddNewContainer("p1", &c1)
	c.Assert(err, check.IsNil)
	err = nodecontainer.AddNewContainer("p2", &c1)
	c.Assert(err, check.IsNil)
	serviceSpec, err = serviceSpecForNodeContainer(loadedC1, "p1", servicecommon.PoolFilter{Include: []string{"p1"}})
	c.Assert(err, check.IsNil)
	c.Assert(serviceSpec.TaskTemplate.Placement.Constraints, check.DeepEquals, []string{"node.labels.tsuru.pool == p1"})
	serviceSpec, err = serviceSpecForNodeContainer(loadedC1, "", servicecommon.PoolFilter{Exclude: []string{"p1", "p2"}})
	c.Assert(err, check.IsNil)
	constraints := sort.StringSlice(serviceSpec.TaskTemplate.Placement.Constraints)
	constraints.Sort()
	c.Assert([]string(constraints), check.DeepEquals, []string{"node.labels.tsuru.pool != p1", "node.labels.tsuru.pool != p2"})
	loadedC1.HostConfig.NetworkMode = "host"
	serviceSpec, err = serviceSpecForNodeContainer(loadedC1, "", servicecommon.PoolFilter{})
	c.Assert(err, check.IsNil)
	c.Assert(serviceSpec.TaskTemplate.Networks, check.DeepEquals, []swarm.NetworkAttachmentConfig{
		{Target: "host"},
	})

}

func tmpFileWith(c *check.C, contents []byte) string {
	f, err := ioutil.TempFile("", "tsuru-cert")
	c.Assert(err, check.IsNil)
	defer f.Close()
	_, err = f.Write(contents)
	c.Assert(err, check.IsNil)
	return f.Name()
}

func (s *S) TestNodeAddr(c *check.C) {
	s.addCluster(c)
	node, err := s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	swarmNode := node.(*swarmNodeWrapper).Node
	addr := nodeAddr(s.clusterCli, swarmNode)
	c.Assert(addr, check.Equals, s.clusterSrv.URL())
	swarmNode.Spec.Annotations.Labels = map[string]string{}
	addr = nodeAddr(s.clusterCli, swarmNode)
	c.Assert(addr, check.Equals, "http://127.0.0.1:2375")
	config.Set("swarm:node-port", 9999)
	defer config.Unset("swarm:node-port")
	addr = nodeAddr(s.clusterCli, swarmNode)
	c.Assert(addr, check.Equals, "http://127.0.0.1:9999")
}

func (s *S) TestNodeAddrTLS(c *check.C) {
	s.addTLSCluster(c)
	node, err := s.p.GetNode(s.clusterSrv.URL())
	c.Assert(err, check.IsNil)
	swarmNode := node.(*swarmNodeWrapper).Node
	swarmNode.Spec.Annotations.Labels = map[string]string{}
	addr := nodeAddr(s.clusterCli, swarmNode)
	c.Assert(addr, check.Equals, "https://127.0.0.1:2376")
	config.Set("swarm:node-port", 9999)
	defer config.Unset("swarm:node-port")
	addr = nodeAddr(s.clusterCli, swarmNode)
	c.Assert(addr, check.Equals, "https://127.0.0.1:9999")
}
