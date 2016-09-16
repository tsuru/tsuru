// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package swarm

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"path/filepath"

	"github.com/tsuru/config"
)

var swarmConfig swarmProvisionerConfig

type swarmProvisionerConfig struct {
	swarmPort  int
	dockerPort int
	tlsConfig  *tls.Config
}

func (p *swarmProvisioner) Initialize() error {
	swarmConfig.swarmPort, _ = config.GetInt("swarm:swarm-port")
	if swarmConfig.swarmPort == 0 {
		swarmConfig.swarmPort = 2377
	}
	caPath, _ := config.GetString("swarm:tls:root-path")
	var err error
	if caPath != "" {
		swarmConfig.tlsConfig, err = readTLSConfig(caPath)
		if err != nil {
			return err
		}
	}
	swarmConfig.dockerPort, _ = config.GetInt("swarm:docker-port")
	if swarmConfig.dockerPort == 0 {
		if swarmConfig.tlsConfig != nil {
			swarmConfig.dockerPort = 2376
		} else {
			swarmConfig.dockerPort = 2375
		}
	}
	return err
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
