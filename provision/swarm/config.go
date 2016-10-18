// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
)

var swarmConfig swarmProvisionerConfig

type swarmProvisionerConfig struct {
	swarmPort int
	tlsConfig *tls.Config
}

func readTLSConfig(caPath string) (*tls.Config, error) {
	// TODO(cezarsa): It's possible to automatically generate a new cert.pem and
	// key.pem every time the server is started. We would only need the "ca.pem"
	// file and the private key for the CA. It might be a better option to
	// easily scale the tsuru api.
	certPEMBlock, errCert := ioutil.ReadFile(filepath.Join(caPath, "cert.pem"))
	if errCert != nil {
		return nil, errCert
	}
	keyPEMBlock, errCert := ioutil.ReadFile(filepath.Join(caPath, "key.pem"))
	if errCert != nil {
		return nil, errCert
	}
	caPEMCert, errCert := ioutil.ReadFile(filepath.Join(caPath, "ca.pem"))
	if errCert != nil {
		return nil, errCert
	}
	tlsCert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return nil, err
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEMCert) {
		return nil, errors.New("Could not add RootCA pem")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      caPool,
	}, nil
}
