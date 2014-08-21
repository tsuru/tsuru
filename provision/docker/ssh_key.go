// Copyright 2014 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"code.google.com/p/go.crypto/ssh"
)

// marshalKey returns two byte slices: one represent the private key in the PEM
// format, and the other representing the public key in the authorized_keys
// format.
func marshalKey(keyPair *rsa.PrivateKey) (privateKey []byte, publicKey []byte, err error) {
	sshPublicKey, err := ssh.NewPublicKey(&keyPair.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	publicKey = ssh.MarshalAuthorizedKey(sshPublicKey)
	block := pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(keyPair)}
	privateKey = pem.EncodeToMemory(&block)
	return privateKey, publicKey, nil
}
