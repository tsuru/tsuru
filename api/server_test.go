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
	"io"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	check "gopkg.in/check.v1"
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
	bytes, err := io.ReadAll(resp.Body)
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

	cert, err := createTempFromData("./testdata/cert.pem")
	c.Assert(err, check.IsNil)
	defer cert.Close()

	key, err := createTempFromData("./testdata/key.pem")
	c.Assert(err, check.IsNil)
	defer key.Close()

	defer os.Remove(cert.Name()) // clean up
	defer os.Remove(key.Name())  // clean up

	config.Set("tls:cert-file", cert.Name())
	config.Set("tls:key-file", key.Name())

	defer config.Unset("use-tls")

	srvConf, err := createServers(nil)
	c.Assert(err, check.IsNil)

	tlsCertificate, encodedCert, err := generateTLSCertificate()
	c.Assert(err, check.IsNil)

	err = replaceFileContent(cert, encodedCert.Certicate)
	c.Assert(err, check.IsNil)

	err = replaceFileContent(key, encodedCert.PrivateKey)
	c.Assert(err, check.IsNil)

	expectedCertificate, err := x509.ParseCertificate(tlsCertificate.Certificate[0])
	c.Assert(err, check.IsNil)

	time.Sleep(time.Second)

	gotCertificate, err := srvConf.httpsSrv.TLSConfig.GetCertificate(nil)
	c.Assert(err, check.IsNil)

	gotX509Certificate, err := x509.ParseCertificate(gotCertificate.Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(gotX509Certificate.Equal(expectedCertificate), check.Equals, true)
}

func createTempFromData(origin string) (*os.File, error) {
	f, err := os.CreateTemp("", "file")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(origin)
	if err != nil {
		return nil, err
	}
	_, err = f.Write(data)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func replaceFileContent(f *os.File, content []byte) error {
	err := f.Truncate(0)
	if err != nil {
		return err
	}
	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	return err
}

func (s *S) TestCreateServerHTTPSWhenGetCertificateIsCalledAndCertificateIsNullShouldReturnError(c *check.C) {
	config.Set("use-tls", true)
	config.Set("tls:listen", "127.0.0.1:8443")
	config.Set("tls:cert-file", "./testdata/cert.pem")
	config.Set("tls:key-file", "./testdata/key.pem")

	defer config.Unset("use-tls")

	srvConf, err := createServers(nil)
	c.Assert(err, check.IsNil)

	srvConf.certificate.Store(nil)

	_, err = srvConf.httpsSrv.TLSConfig.GetCertificate(nil)
	c.Assert(err, check.Not(check.IsNil))
}

func (s *S) TestSrvConfig_LoadCertificate_WhenFieldCertificateIsNullShouldLoadExpectedCertificate(c *check.C) {
	srvConf := &srvConfig{
		certFile: "./testdata/cert.pem",
		keyFile:  "./testdata/key.pem",
	}

	tlsCert, err := tls.LoadX509KeyPair("./testdata/cert.pem", "./testdata/key.pem")
	c.Assert(err, check.IsNil)

	expectedCertificate, err := x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, check.IsNil)

	changed, err := srvConf.readCertificateFromFilesystem()
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, true)

	gotCertificate, err := x509.ParseCertificate(srvConf.certificate.Load().Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(expectedCertificate.Equal(gotCertificate), check.Equals, true)
}

func (s *S) TestSrvConfig_LoadCertificate_WhenNewCertificateIsEqualsToOlderCertificateShouldNotChangeCertificate(c *check.C) {
	tlsCert, err := tls.LoadX509KeyPair("./testdata/cert.pem", "./testdata/key.pem")
	c.Assert(err, check.IsNil)

	srvConf := &srvConfig{
		certFile: "./testdata/cert.pem",
		keyFile:  "./testdata/key.pem",
	}
	srvConf.certificate.Store(&tlsCert)

	changed, err := srvConf.readCertificateFromFilesystem()
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, false)

	expectedCertificate, err := x509.ParseCertificate(tlsCert.Certificate[0])
	c.Assert(err, check.IsNil)

	gotCertificate, err := x509.ParseCertificate(srvConf.certificate.Load().Certificate[0])
	c.Assert(err, check.IsNil)

	c.Assert(expectedCertificate.Equal(gotCertificate), check.Equals, true)
}

func (s *S) TestSrvConfig_readCertificateFromFilesystem_WhenCertificatesAreNotEqualShouldLoadTheNewOne(c *check.C) {
	inMemoryCertificate, _, err := generateTLSCertificate()
	c.Assert(err, check.IsNil)

	srvConf := &srvConfig{
		certFile: "./testdata/cert.pem",
		keyFile:  "./testdata/key.pem",
	}

	srvConf.certificate.Store(inMemoryCertificate)

	changed, err := srvConf.readCertificateFromFilesystem()
	c.Assert(err, check.IsNil)
	c.Assert(changed, check.Equals, true)

	expectedCertificate, err := tls.LoadX509KeyPair(srvConf.certFile, srvConf.keyFile)
	c.Assert(err, check.IsNil)

	gotCertificate := srvConf.certificate.Load()

	c.Assert(reflect.DeepEqual(inMemoryCertificate.Certificate, gotCertificate.Certificate), check.Equals, false)
	c.Assert(reflect.DeepEqual(expectedCertificate.Certificate, gotCertificate.Certificate), check.Equals, true)
}

type encodedCertificate struct {
	Certicate  []byte
	PrivateKey []byte
}

func generateTLSCertificate() (cert *tls.Certificate, raw *encodedCertificate, err error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := cryptoRand.Int(cryptoRand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
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
		return nil, nil, err
	}

	derBytes, err := x509.CreateCertificate(cryptoRand.Reader, &template, &template, privateKey.Public(), privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEMBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	certificate, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return nil, nil, err
	}
	return &certificate, &encodedCertificate{certPEMBlock, keyPEMBlock}, nil
}

func generateCertificate(template *x509.Certificate, parent *tls.Certificate) (*tls.Certificate, error) {
	var err error
	privateKey, err := rsa.GenerateKey(cryptoRand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	var certificateBytes []byte
	if parent == nil {
		certificateBytes, err = x509.CreateCertificate(cryptoRand.Reader, template, template, privateKey.Public(), privateKey)
	} else {
		var parentX509 *x509.Certificate
		if parentX509, err = x509.ParseCertificate(parent.Certificate[0]); err != nil {
			return nil, err
		}
		certificateBytes, err = x509.CreateCertificate(cryptoRand.Reader, template, parentX509, privateKey.Public(), parent.PrivateKey)
	}
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	return &certificate, err
}

func (s *S) TestValidateTLSCertificate_WhenHostConfigIsNotDefined_ShouldReturnError(c *check.C) {
	config.Unset("host")
	err := validateTLSCertificate(nil, nil)
	c.Assert(err, check.Not(check.IsNil))
}

func (s *S) TestValidateTLSCertificate_WhenCertificateIsNotDefined_ShouldReturnExpectedError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	err := validateTLSCertificate(nil, nil)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "there is no certificate provided")
}

func (s *S) TestValidateTLSCertificate_WhenCertificateIsEmpty_ShouldReturnExpectedError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	err := validateTLSCertificate(&tls.Certificate{}, nil)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "there is no certificate provided")
}

func (s *S) TestValidateTLSCertificate_WhenCertificateIsNotTrustedBySystemCertPool_ShouldReturnError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		IsCA:         true,
		Subject: pkix.Name{
			Organization: []string{"tsuru.io"},
		},
		DNSNames:  []string{"tsuru.example.org"},
		NotAfter:  time.Now().Add(time.Minute),
		NotBefore: time.Now(),
	}
	cert, err := generateCertificate(caTemplate, nil)
	c.Assert(err, check.IsNil)
	err = validateTLSCertificate(cert, nil)
	c.Assert(err, check.Not(check.IsNil))
	_, ok := err.(x509.UnknownAuthorityError)
	c.Assert(ok, check.Equals, true)
}

func (s *S) TestValidateTLSCertificate_WhenCertificateIsInRootCertificatePool_ShouldReturnNoError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		IsCA:         true,
		Subject: pkix.Name{
			Organization: []string{"tsuru.io"},
		},
		DNSNames:  []string{"tsuru.example.org"},
		NotAfter:  time.Now().Add(time.Minute),
		NotBefore: time.Now(),
	}
	cert, err := generateCertificate(caTemplate, nil)
	c.Assert(err, check.IsNil)
	certX509, err := x509.ParseCertificate(cert.Certificate[0])
	c.Assert(err, check.IsNil)
	rootPool := x509.NewCertPool()
	rootPool.AddCert(certX509)
	err = validateTLSCertificate(cert, rootPool)
	c.Assert(err, check.IsNil)
}

func (s *S) TestValidateTLSCertificate_WhenCertificateIsSignedByTrustedIntermediateCertificate_ShouldReturnNoError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		Subject: pkix.Name{
			CommonName:   "Tsuru CA #1",
			Organization: []string{"Tsuru"},
		},
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		NotAfter:              time.Now().Add(time.Minute * 10),
		NotBefore:             time.Now(),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	caCert, err := generateCertificate(caTemplate, nil)
	c.Assert(err, check.IsNil)
	intermediateTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1010),
		Subject: pkix.Name{
			CommonName:   "Tsuru Intermediate Authority",
			Organization: []string{"Tsuru"},
		},
		SubjectKeyId:          []byte{1, 2, 3, 4, 6},
		NotAfter:              time.Now().Add(time.Minute * 5),
		NotBefore:             time.Now(),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	intermediateCert, err := generateCertificate(intermediateTemplate, caCert)
	c.Assert(err, check.IsNil)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2000),
		Subject: pkix.Name{
			Organization: []string{"Tsuru"},
		},
		SubjectKeyId:          []byte{1, 2, 3, 4, 7},
		NotAfter:              time.Now().Add(time.Minute),
		NotBefore:             time.Now(),
		DNSNames:              []string{"tsuru.example.org"},
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	leafCert, err := generateCertificate(leafTemplate, intermediateCert)
	c.Assert(err, check.IsNil)
	leafCert.Certificate = append(leafCert.Certificate, intermediateCert.Certificate[0])
	caX509Cert, err := x509.ParseCertificate(caCert.Certificate[0])
	c.Assert(err, check.IsNil)
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caX509Cert)
	err = validateTLSCertificate(leafCert, rootPool)
	c.Assert(err, check.IsNil)
}

func (s *S) TestValidateTLSCertificate_WhenCertificateIsNotValidYet_ShouldReturnExpectedError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		Subject: pkix.Name{
			CommonName:   "Tsuru CA #1",
			Organization: []string{"Tsuru"},
		},
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		NotAfter:              time.Now().Add(time.Minute * 10),
		NotBefore:             time.Now().Add(time.Minute * 5),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	caCert, err := generateCertificate(caTemplate, nil)
	c.Assert(err, check.IsNil)
	caX509Cert, err := x509.ParseCertificate(caCert.Certificate[0])
	c.Assert(err, check.IsNil)
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caX509Cert)
	err = validateTLSCertificate(caCert, rootPool)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "x509: certificate has expired or is not yet valid.*")
}

func (s *S) TestValidateTLSCertificate_WhenCertificateHasBeenExpired_ShouldReturnExpectedError(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		Subject: pkix.Name{
			CommonName:   "Tsuru CA #1",
			Organization: []string{"Tsuru"},
		},
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		NotAfter:              time.Now().Add(time.Minute * -1),
		NotBefore:             time.Now().Add(time.Minute * -5),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"tsuru.example.org"},
	}
	caCert, err := generateCertificate(caTemplate, nil)
	c.Assert(err, check.IsNil)
	caX509Cert, err := x509.ParseCertificate(caCert.Certificate[0])
	c.Assert(err, check.IsNil)
	rootPool := x509.NewCertPool()
	rootPool.AddCert(caX509Cert)
	err = validateTLSCertificate(caCert, rootPool)
	c.Assert(err, check.Not(check.IsNil))
	c.Assert(err, check.ErrorMatches, "x509: certificate has expired or is not yet valid.*")
}

func (s *S) TestCertificateValidator_start_WhenCurrentlyLoadedCertificateExpire_ShouldCallShutdownFunc(c *check.C) {
	config.Set("host", "https://tsuru.example.org:8443")
	defer config.Unset("host")
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1000),
		Subject: pkix.Name{
			CommonName:   "Tsuru CA #1",
			Organization: []string{"Tsuru"},
		},
		SubjectKeyId:          []byte{1, 2, 3, 4, 5},
		NotBefore:             time.Now(),
		NotAfter:              time.Now(),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"tsuru.example.org"},
	}
	caCert, err := generateCertificate(caTemplate, nil)
	c.Assert(err, check.IsNil)
	caX509, err := x509.ParseCertificate(caCert.Certificate[0])
	c.Assert(err, check.IsNil)
	rootsCertPool := x509.NewCertPool()
	rootsCertPool.AddCert(caX509)
	srvConf := &srvConfig{
		roots: rootsCertPool,
	}
	srvConf.certificate.Store(caCert)
	invokedActionFunc := make(chan bool)
	cv := &certificateValidator{
		conf: srvConf,
		shutdownServerFunc: func(err error) {
			c.Assert(err, check.Not(check.IsNil))
			invokedActionFunc <- true
		},
	}
	cv.start()
	c.Assert(<-invokedActionFunc, check.Equals, true)
}

func (s *S) TestRunServerWithPanic(c *check.C) {
	RegisterHandler("/panic", "GET", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("ahoy")
	}))
	request, err := http.NewRequest("GET", "/panic", nil)
	c.Assert(err, check.IsNil)
	recorder := httptest.NewRecorder()
	m := RunServer(true)
	m.ServeHTTP(recorder, request)
	c.Assert(recorder.Code, check.Equals, http.StatusInternalServerError)
	c.Assert(recorder.Body.String(), check.Matches, "(?s)^PANIC: ahoy.*")
}
