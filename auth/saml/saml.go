// Copyright 2015 tsuru authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package saml

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/diego-araujo/go-saml"
	"github.com/pkg/errors"
	"github.com/tsuru/config"
	"github.com/tsuru/tsuru/auth"
	"github.com/tsuru/tsuru/auth/native"
	samlErrors "github.com/tsuru/tsuru/auth/saml/errors"
	tsuruErrors "github.com/tsuru/tsuru/errors"
	"github.com/tsuru/tsuru/log"
	authTypes "github.com/tsuru/tsuru/types/auth"
	"github.com/tsuru/tsuru/validation"
)

type SAMLAuthParser interface {
	Parse(infoResponse string) (*saml.Response, error)
}

type SAMLAuthScheme struct {
	BaseConfig BaseConfig
	Parser     SAMLAuthParser
}

type BaseConfig struct {
	EntityID              string
	DisplayName           string
	Description           string
	PublicCert            string
	PrivateKey            string
	IdpURL                string
	IdpPublicCert         string
	SignRequest           bool
	SignedResponse        bool
	DeflatEncodedResponse bool
}

func init() {
	auth.RegisterScheme("saml", &SAMLAuthScheme{})
}

func (s SAMLAuthScheme) AppLogout(token string) error {
	return s.Logout(token)
}

// This method loads basic config and returns a copy of the
// config object.
func (s *SAMLAuthScheme) loadConfig() (BaseConfig, error) {
	if s.BaseConfig.EntityID != "" {
		return s.BaseConfig, nil
	}
	if s.Parser == nil {
		s.Parser = s
	}
	var emptyConfig BaseConfig
	publicCert, err := config.GetString("auth:saml:sp-publiccert")
	if err != nil {
		return emptyConfig, err
	}
	privateKey, err := config.GetString("auth:saml:sp-privatekey")
	if err != nil {
		return emptyConfig, err
	}
	idpURL, err := config.GetString("auth:saml:idp-ssourl")
	if err != nil {
		return emptyConfig, err
	}
	displayName, err := config.GetString("auth:saml:sp-display-name")
	if err != nil {
		displayName = "Tsuru"
		log.Debugf("auth:saml:sp-display-name not found using default: %s", err)
	}
	description, err := config.GetString("auth:saml:sp-description")
	if err != nil {
		description = "Tsuru Platform as a Service software"
		log.Debugf("auth:saml:sp-description not found using default: %s", err)
	}
	idpPublicCert, err := config.GetString("auth:saml:idp-publiccert")
	if err != nil {
		return emptyConfig, err
	}
	entityId, err := config.GetString("auth:saml:sp-entityid")
	if err != nil {
		return emptyConfig, err
	}
	signRequest, err := config.GetBool("auth:saml:sp-sign-request")
	if err != nil {
		return emptyConfig, err
	}
	signedResponse, err := config.GetBool("auth:saml:idp-sign-response")
	if err != nil {
		return emptyConfig, err
	}
	deflatEncodedResponse, err := config.GetBool("auth:saml:idp-deflate-encoding")
	if err != nil {
		deflatEncodedResponse = false
		log.Debugf("auth:saml:idp-deflate-encoding not found using default [false]: %s", err)
	}
	s.BaseConfig = BaseConfig{
		EntityID:              entityId,
		DisplayName:           displayName,
		Description:           description,
		PublicCert:            publicCert,
		PrivateKey:            privateKey,
		IdpURL:                idpURL,
		IdpPublicCert:         idpPublicCert,
		SignRequest:           signRequest,
		SignedResponse:        signedResponse,
		DeflatEncodedResponse: deflatEncodedResponse,
	}
	return s.BaseConfig, nil
}

func Metadata() (string, error) {
	scheme := SAMLAuthScheme{}
	sp, err := scheme.createSP()
	if err != nil {
		return "", err
	}
	md, err := sp.GetEntityDescriptor()
	if err != nil {
		return "", err
	}
	return md, nil
}

func (s *SAMLAuthScheme) Login(params map[string]string) (auth.Token, error) {
	_, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	if _, ok := params["callback"]; ok {
		return nil, s.callback(params)
	}
	requestId, ok := params["request_id"]
	if !ok {
		return nil, samlErrors.ErrMissingRequestIdError
	}
	req := request{}
	err = req.getById(requestId)
	if err != nil {
		return nil, err
	}
	if !req.Authed {
		return nil, samlErrors.ErrRequestWaitingForCredentials
	}
	user, err := auth.GetUserByEmail(req.Email)
	if err != nil {
		if err != authTypes.ErrUserNotFound {
			return nil, err
		}
		registrationEnabled, _ := config.GetBool("auth:user-registration")
		if !registrationEnabled {
			return nil, err
		}
		user = &auth.User{Email: req.Email}
		err = user.Create()
		if err != nil {
			return nil, err
		}
	}
	token, err := createToken(user)
	if err != nil {
		return nil, err
	}
	req.Remove()
	return token, nil
}

func (s *SAMLAuthScheme) idpHost() string {
	url, err := url.Parse(s.BaseConfig.IdpURL)
	if err != nil {
		return ""
	}
	hostport := strings.Split(url.Host, ":")
	return hostport[0]
}

func (s *SAMLAuthScheme) callback(params map[string]string) error {
	xml, ok := params["xml"]
	if !ok {
		return samlErrors.ErrMissingFormValueError
	}
	log.Debugf("Data received from identity provider: %s", xml)
	response, err := s.Parser.Parse(xml)
	if err != nil {
		log.Errorf("Got error while parsing IDP data: %s", err)
		return samlErrors.ErrParseResponseError
	}
	sp, err := s.createSP()
	if err != nil {
		return err
	}
	err = validateResponse(response, sp)
	if err != nil {
		log.Errorf("Got error while validing IDP data: %s", err)
		if strings.Contains(err.Error(), "assertion has expired") {
			return ErrRequestNotFound
		}
		return samlErrors.ErrParseResponseError
	}
	requestId, err := getRequestIdFromResponse(response)
	if requestId == "" && err == ErrRequestIdNotFound {
		log.Debugf("Request ID %s not found: %s", requestId, err.Error())
		return err
	}
	req := request{}
	err = req.getById(requestId)
	if err != nil {
		return err
	}
	email, err := getUserIdentity(response)
	if err != nil {
		return err
	}
	if !validation.ValidateEmail(email) {
		if strings.Contains(email, "@") {
			return &tsuruErrors.ValidationError{Message: "attribute user identity contains invalid character"}
		}
		// we need create a unique email for the user
		email = strings.Join([]string{email, "@", s.idpHost()}, "")
		if !validation.ValidateEmail(email) {
			return &tsuruErrors.ValidationError{Message: "could not create valid email with auth:saml:idp-attribute-user-identity"}
		}
	}
	req.Authed = true
	req.Email = email
	req.Update()
	return nil
}

func (s *SAMLAuthScheme) AppLogin(appName string) (auth.Token, error) {
	nativeScheme := native.NativeScheme{}
	return nativeScheme.AppLogin(appName)
}

func (s *SAMLAuthScheme) Logout(token string) error {
	return deleteToken(token)
}

func (s *SAMLAuthScheme) Auth(token string) (auth.Token, error) {
	return getToken(token)
}

func (s *SAMLAuthScheme) Name() string {
	return "saml"
}

func (s *SAMLAuthScheme) generateAuthnRequest() (*AuthnRequestData, error) {
	sp, err := s.createSP()
	if err != nil {
		return nil, err
	}
	authnRequest := sp.GetAuthnRequest()
	b64XML, err := authnRequest.CompressedEncodedSignedString(sp.PrivateKeyPath)
	if err != nil {
		return nil, err
	}
	url, err := saml.GetAuthnRequestURL(sp.IDPSSOURL, b64XML, sp.AssertionConsumerServiceURL)
	if err != nil {
		return nil, err
	}
	data := AuthnRequestData{
		Base64AuthRequest: b64XML,
		URL:               url,
		ID:                authnRequest.ID,
	}
	return &data, nil
}

type AuthnRequestData struct {
	Base64AuthRequest string
	URL               string
	ID                string
}

func (s *SAMLAuthScheme) createSP() (*saml.ServiceProviderSettings, error) {
	conf, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	authCallbackURL, _ := config.GetString("host")
	sp := saml.ServiceProviderSettings{
		PublicCertPath:              conf.PublicCert,
		PrivateKeyPath:              conf.PrivateKey,
		IDPSSOURL:                   conf.IdpURL,
		DisplayName:                 conf.DisplayName,
		Description:                 conf.Description,
		IDPPublicCertPath:           conf.IdpPublicCert,
		Id:                          conf.EntityID,
		SPSignRequest:               conf.SignRequest,
		IDPSignResponse:             conf.SignedResponse,
		AssertionConsumerServiceURL: authCallbackURL + "/auth/saml",
	}
	sp.Init()
	return &sp, nil
}

func (s *SAMLAuthScheme) Info() (auth.SchemeInfo, error) {
	authnRequestData, err := s.generateAuthnRequest()
	if err != nil {
		return nil, err
	}
	r := request{}
	if _, err := r.Create(authnRequestData); err != nil {
		return nil, err
	}
	return auth.SchemeInfo{
		"request_id":      authnRequestData.ID,
		"saml_request":    authnRequestData.Base64AuthRequest,
		"url":             authnRequestData.URL,
		"request_timeout": fmt.Sprintf("%.0f", r.expireTime().Seconds()),
	}, nil
}

func (s *SAMLAuthScheme) Parse(xml string) (*saml.Response, error) {
	if xml == "" {
		return nil, samlErrors.ErrMissingFormValueError
	}
	var response *saml.Response
	var err error
	if !s.BaseConfig.DeflatEncodedResponse {
		response, err = saml.ParseEncodedResponse(xml)
	} else {
		response, err = saml.ParseCompressedEncodedResponse(xml)
	}
	if err != nil || response == nil {
		return nil, errors.Wrapf(err, "unable to parse identity provider data: %s", xml)
	}
	sp, err := s.createSP()
	if err != nil {
		return nil, errors.Wrap(err, "unable to create service provider object")
	}
	if response.IsEncrypted() {
		if err = response.Decrypt(sp.PrivateKeyPath); err != nil {
			respData, _ := response.String()
			return nil, errors.Wrapf(err, "unable to decrypt identity provider data: %s", respData)
		}
	}
	resp, _ := response.String()
	log.Debugf("Data received from identity provider decoded: %s", resp)
	return response, nil
}

func (s *SAMLAuthScheme) Create(user *auth.User) (*auth.User, error) {
	user.Password = ""
	if err := user.Create(); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *SAMLAuthScheme) Remove(u *auth.User) error {
	if err := deleteAllTokens(u.Email); err != nil {
		return err
	}
	return u.Delete()
}
