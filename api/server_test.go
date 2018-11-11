// Copyright 2013 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	cryptoRand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"gopkg.in/check.v1"
)

func authorizedTsuruHandler(w http.ResponseWriter, r *http.Request, t auth.Token) error {
	fmt.Fprint(w, r.Method)
	return nil
}

func selectAvailablePort() (string, error) {
	var err error
	for i := 0; i < 20; i++ {
		port := strconv.Itoa(rand.Intn(20000) + 8000)
		var conn net.Listener
		conn, err = net.Listen("tcp", "localhost:"+port)
		if err == nil {
			conn.Close()
			return port, nil
		}
	}
	return "", err
}

func waitForServer(addr string) error {
	var err error
	for i := 0; i < 100; i++ {
		_, err = net.DialTimeout("tcp", addr, 1*time.Second)
		if err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

func (s *S) testRequest(url string, c *check.C) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	c.Assert(err, check.IsNil)
	req.Header.Set("Authorization", "b "+s.token.GetValue())
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := client.Do(req)
	c.Assert(err, check.IsNil)
	bytes, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, check.IsNil)
	defer resp.Body.Close()
	c.Assert(string(bytes), check.Equals, "GET")
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaGet(c *check.C) {
	RegisterHandler("/foo/bar", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("GET", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPost(c *check.C) {
	RegisterHandler("/foo/bar", "POST", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("POST", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaPut(c *check.C) {
	RegisterHandler("/foo/bar", "PUT", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("PUT", check.Equals, rec.Body.String())
}

func (s *S) TestRegisterHandlerMakesHandlerAvailableViaDelete(c *check.C) {
	RegisterHandler("/foo/bar", "DELETE", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("DELETE", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("DELETE", check.Equals, rec.Body.String())
}

func (s *S) TestIsNotAdmin(c *check.C) {
	RegisterHandler("/foo/bar", "POST", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()
	rec := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "http://example.com/foo/bar", nil)
	req.Header.Set("Authorization", "bearer "+s.token.GetValue())
	c.Assert(err, check.IsNil)
	m := RunServer(true)
	m.ServeHTTP(rec, req)
	c.Assert("POST", check.Equals, rec.Body.String())
}

func (s *S) TestCreateServersHTTPOnly(c *check.C) {
	port, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Set("listen", "0.0.0.0:"+port)
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queuedb")
	RegisterHandler("/foo", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()

	handler := RunServer(true)
	srvConf, err := createServers(handler)
	c.Assert(err, check.IsNil)
	go srvConf.start()

	err = waitForServer("localhost:" + port)
	c.Assert(err, check.IsNil)
	s.testRequest(fmt.Sprintf("http://localhost:%s/foo", port), c)
}

func (s *S) TestCreateServersHTTPSOnly(c *check.C) {
	port, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Set("listen", "0.0.0.0:"+port)
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queuedb")
	config.Set("use-tls", true)
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")
	defer config.Unset("use-tls")
	RegisterHandler("/foo", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()

	handler := RunServer(true)
	srvConf, err := createServers(handler)
	c.Assert(err, check.IsNil)
	go srvConf.start()

	err = waitForServer("localhost:" + port)
	c.Assert(err, check.IsNil)
	s.testRequest(fmt.Sprintf("https://localhost:%s/foo", port), c)
}

func (s *S) TestCreateServersHTTPSOnlyWithTlsListenConfig(c *check.C) {
	port, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Unset("listen")
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queuedb")
	config.Set("use-tls", true)
	config.Set("tls:listen", "0.0.0.0:"+port)
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")
	defer config.Unset("use-tls")
	RegisterHandler("/foo", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()

	handler := RunServer(true)
	srvConf, err := createServers(handler)
	c.Assert(err, check.IsNil)
	go srvConf.start()

	err = waitForServer("localhost:" + port)
	c.Assert(err, check.IsNil)
	s.testRequest(fmt.Sprintf("https://localhost:%s/foo", port), c)
}

func (s *S) TestCreateServersHTTPAndHTTPS(c *check.C) {
	httpPort, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	httpsPort, err := selectAvailablePort()
	c.Assert(err, check.IsNil)
	config.Set("listen", "0.0.0.0:"+httpPort)
	config.Set("queue:mongo-url", "127.0.0.1:27017?maxPoolSize=100")
	config.Set("queue:mongo-database", "queuedb")
	config.Set("use-tls", true)
	config.Set("tls:listen", "0.0.0.0:"+httpsPort)
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")
	defer config.Unset("use-tls")
	RegisterHandler("/foo", "GET", AuthorizationRequiredHandler(authorizedTsuruHandler))
	defer resetHandlers()

	handler := RunServer(true)
	srvConf, err := createServers(handler)
	c.Assert(err, check.IsNil)
	go srvConf.start()

	err = waitForServer("localhost:" + httpsPort)
	c.Assert(err, check.IsNil)
	s.testRequest(fmt.Sprintf("https://localhost:%s/foo", httpsPort), c)
	err = waitForServer("localhost:" + httpPort)
	c.Assert(err, check.IsNil)
	s.testRequest(fmt.Sprintf("http://localhost:%s/foo", httpPort), c)
}

func (s *S) TestCreateServerHTTPSWhenGetCertificateIsCalledReturnsTheNewerLoadedCertificate(c *check.C) {
	config.Set("use-tls", true)
	config.Set("tls:listen", "127.0.0.1:8443")
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")
	config.Set("tls:auto-reload:interval", "500ms")

	defer config.Unset("use-tls")

	expectedTLSCertificate, err := tls.LoadX509KeyPair("./testdata/cert.pem", "./testdata/key.pem")
	c.Assert(err, check.IsNil)

	expectedCertificate, err := x509.ParseCertificate(expectedTLSCertificate.Certificate[0])
	c.Assert(err, check.IsNil)

	srvConf, err := createServers(nil)
	c.Assert(err, check.IsNil)

	tlsCertificate, err := generateTLSCertificate()
	c.Assert(err, check.IsNil)

	srvConf.Lock()
	srvConf.certificate = tlsCertificate
	srvConf.Unlock()

	time.Sleep(time.Second)

	gotCertificate, err := srvConf.httpsSrv.TLSConfig.GetCertificate(nil)
	c.Assert(err, check.IsNil)

	gotX509Certificate, err := x509.ParseCertificate(gotCertificate.Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(gotX509Certificate.Equal(expectedCertificate), check.Equals, true)
}

func (s *S) TestCreateServerHTTPSWhenGetCertificateIsCalledAndCertificateIsNullShouldReturnError(c *check.C) {
	config.Set("use-tls", true)
	config.Set("tls:listen", "127.0.0.1:8443")
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")

	defer config.Unset("use-tls")

	srvConf, err := createServers(nil)
	c.Assert(err, check.IsNil)

	srvConf.Lock()
	srvConf.certificate = nil
	srvConf.Unlock()

	_, err = srvConf.httpsSrv.TLSConfig.GetCertificate(nil)
	c.Assert(err, check.Not(check.IsNil))
}

func (s *S) TestSrvConfig_LoadCertificate_WhenFieldCertificateIsNullShouldLoadExpectedCertificate(c *check.C) {
	srvConf := &srvConfig{
		certificate: nil,
		certFile:    "./testdata/cert.pem",
		keyFile:     "./testdata/key.pem",
	}

	tlsCert, err := tls.LoadX509KeyPair("./testdata/cert.pem", "./testdata/key.pem")
	c.Assert(err, check.IsNil)

	expectedCertificate, err := x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, check.IsNil)

	changed, err := srvConf.loadCertificate()
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, true)

	gotCertificate, err := x509.ParseCertificate(srvConf.certificate.Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(expectedCertificate.Equal(gotCertificate), check.Equals, true)
}

func (s *S) TestSrvConfig_LoadCertificate_WhenNewCertificateIsEqualsToOlderCertificateShouldNotChangeCertificate(c *check.C) {
	tlsCert, err := tls.LoadX509KeyPair("./testdata/cert.pem", "./testdata/key.pem")
	c.Assert(err, check.IsNil)

	srvConf := &srvConfig{
		certificate: &tlsCert,
		certFile:    "./testdata/cert.pem",
		keyFile:     "./testdata/key.pem",
	}

	changed, err := srvConf.loadCertificate()
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, false)

	expectedCertificate, err := x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, check.IsNil)

	gotCertificate, err := x509.ParseCertificate(srvConf.certificate.Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(expectedCertificate.Equal(gotCertificate), check.Equals, true)
}

func (s *S) TestSrvConfig_LoadCertificate_WhenCertificatesAreNotEqualShouldLoadTheNewOne(c *check.C) {
	tlsCert, err := generateTLSCertificate()
	c.Assert(err, check.IsNil)

	srvConf := &srvConfig{
		certificate: tlsCert,
		certFile:    "./testdata/cert.pem",
		keyFile:     "./testdata/key.pem",
	}

	changed, err := srvConf.loadCertificate()
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, true)

	expectedCertificate, err := x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, check.IsNil)

	gotCertificate, err := x509.ParseCertificate(srvConf.certificate.Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(expectedCertificate.Equal(gotCertificate), check.Equals, false)
}

func generateTLSCertificate() (*tls.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := cryptoRand.Int(cryptoRand.Reader, serialNumberLimit)

	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Issuer: pkix.Name{
			Organization: []string{"tsuru.io"},
		},
		Subject: pkix.Name{
			Organization: []string{"tsuru.io"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Minute),
	}

	privateKey, err := rsa.GenerateKey(cryptoRand.Reader, 1024)

	if err != nil {
		return nil, err
	}

	derBytes, err := x509.CreateCertificate(cryptoRand.Reader, &template, &template, privateKey.Public(), privateKey)

	if err != nil {
		return nil, err
	}

	certPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	certificate, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)

	return &certificate, err
}
