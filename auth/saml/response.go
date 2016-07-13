// Copyright 2016 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package saml

import (
	"fmt"

	"github.com/diego-araujo/go-saml"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/errors"
)

var (
	ErrRequestIdNotFound = &errors.ValidationError{Message: "Field attribute InResponseTo not found in saml response data"}
	ErrCheckSignature    = &errors.ValidationError{Message: "SAMLResponse signature validation"}
)

func getRequestIdFromResponse(r *saml.Response) (string, error) {
	var idRequest string
	if r.IsEncrypted() {
		idRequest = r.EncryptedAssertion.Assertion.Subject.SubjectConfirmation.SubjectConfirmationData.InResponseTo
	} else {
		idRequest = r.Assertion.Subject.SubjectConfirmation.SubjectConfirmationData.InResponseTo
	}
	if idRequest == "" {
		return "", ErrRequestIdNotFound
	}
	return idRequest, nil
}

func getUserIdentity(r *saml.Response) (string, error) {
	attrFriendlyNameIdentifier, err := config.GetString("auth:saml:idp-attribute-user-identity")
	if err != nil {
		return "", fmt.Errorf("error reading config auth:saml:idp-attribute-user-identity: %s ", err)
	}
	userIdentifier := r.GetAttribute(attrFriendlyNameIdentifier)
	if userIdentifier == "" {
		return "", fmt.Errorf("unable to parse identity provider data - not found  <Attribute FriendlyName=" + attrFriendlyNameIdentifier + "> ")
	}
	return userIdentifier, nil
}

func validateResponse(r *saml.Response, sp *saml.ServiceProviderSettings) error {
	if err := r.Validate(sp); err != nil {
		return err
	}
	if sp.IDPSignResponse {
		if err := r.ValidateResponseSignature(sp); err != nil {
			return err
		}
	}
	if err := r.ValidateExpiredConfirmation(sp); err != nil {
		return err
	}
	return nil
}
