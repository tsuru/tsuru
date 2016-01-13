package saml

import (
	"encoding/base64"
	"encoding/xml"
	"errors"
	"github.com/diego-araujo/go-saml/util"
	"time"
)

func ParseCompressedEncodedResponse(b64ResponseXML string) (*Response, error) {
	authnResponse := Response{}
	compressedXML, err := base64.StdEncoding.DecodeString(b64ResponseXML)
	if err != nil {
		return nil, err
	}
	bXML := util.Decompress(compressedXML)
	err = xml.Unmarshal(bXML, &authnResponse)
	if err != nil {
		return nil, err
	}

	// There is a bug with XML namespaces in Go that's causing XML attributes with colons to not be roundtrip
	// marshal and unmarshaled so we'll keep the original string around for validation.
	authnResponse.originalString = string(bXML)
	return &authnResponse, nil

}

func ParseEncodedResponse(b64ResponseXML string) (*Response, error) {
	response := Response{}
	bytesXML, err := base64.StdEncoding.DecodeString(b64ResponseXML)
	//dst := string(bytesXML[:])
	if err != nil {
		return nil, err
	}
	err = xml.Unmarshal(bytesXML, &response)
	if err != nil {
		return nil, err
	}
	//fmt.Printf("%+v\n", response)
	// There is a bug with XML namespaces in Go that's causing XML attributes with colons to not be roundtrip
	// marshal and unmarshaled so we'll keep the original string around for validation.
	response.originalString = string(bytesXML)
	return &response, nil
}

func (r *Response) IsEncrypted() bool {

	//Test if exits EncryptedAssertion tag
	if r.EncryptedAssertion.EncryptedData.EncryptionMethod.Algorithm == "" {
		return false
	} else {
		return true
	}
}

func (r *Response) Decrypt(privateKeyPath string) error {
	s := r.originalString

	if r.IsEncrypted() == false {
		return errors.New("missing EncryptedAssertion tag on SAML Response, is encrypted?")

	}
	plainXML, err := DecryptResponse(s, privateKeyPath)
	if err != nil {
		return err
	}
	err = xml.Unmarshal([]byte(plainXML), &r)
	if err != nil {
		return err
	}

	r.originalString = plainXML
	return nil
}

func (r *Response) ValidateResponseSignature(s *ServiceProviderSettings) error {

	assertion, err := r.getAssertion()
	if err != nil {
		return err
	}

	if len(assertion.Signature.SignatureValue.Value) == 0 {
		return errors.New("no signature")
	}

	err = VerifyResponseSignature(r.originalString, s.IDPPublicCertPath)
	if err != nil {
		return err
	}

	return nil
}

func (r *Response) getAssertion() (Assertion, error) {

	assertion := Assertion{}

	if r.IsEncrypted() {
		assertion = r.EncryptedAssertion.Assertion
	} else {
		assertion = r.Assertion
	}

	if len(assertion.ID) == 0 {
		return assertion, errors.New("no Assertions")
	}
	return assertion, nil
}

func (r *Response) Validate(s *ServiceProviderSettings) error {
	if r.Version != "2.0" {
		return errors.New("unsupported SAML Version")
	}

	if len(r.ID) == 0 {
		return errors.New("missing ID attribute on SAML Response")
	}

	assertion, err := r.getAssertion()
	if err != nil {
		return err
	}

	if assertion.Subject.SubjectConfirmation.Method != "urn:oasis:names:tc:SAML:2.0:cm:bearer" {
		return errors.New("assertion method exception")
	}

	if assertion.Subject.SubjectConfirmation.SubjectConfirmationData.Recipient != s.AssertionConsumerServiceURL {
		return errors.New("subject recipient mismatch, expected: " + s.AssertionConsumerServiceURL + " not " + assertion.Subject.SubjectConfirmation.SubjectConfirmationData.Recipient)
	}

	if r.Destination != s.AssertionConsumerServiceURL {
		return errors.New("destination mismath expected: " + s.AssertionConsumerServiceURL + " not " + r.Destination)
	}

	return nil
}

func (r *Response) ValidateExpiredConfirmation(s *ServiceProviderSettings) error {

	assertion, err := r.getAssertion()
	if err != nil {
		return err
	}

	//CHECK TIMES
	expires := assertion.Subject.SubjectConfirmation.SubjectConfirmationData.NotOnOrAfter
	notOnOrAfter, e := time.Parse(time.RFC3339, expires)
	if e != nil {
		return e
	}
	if notOnOrAfter.Before(time.Now()) {
		return errors.New("assertion has expired on: " + expires)
	}

	return nil
}
func NewSignedResponse() *Response {
	return &Response{
		XMLName: xml.Name{
			Local: "samlp:Response",
		},
		SAMLP:        "urn:oasis:names:tc:SAML:2.0:protocol",
		SAML:         "urn:oasis:names:tc:SAML:2.0:assertion",
		SAMLSIG:      "http://www.w3.org/2000/09/xmldsig#",
		ID:           util.ID(),
		Version:      "2.0",
		IssueInstant: time.Now().UTC().Format(time.RFC3339Nano),
		Issuer: Issuer{
			XMLName: xml.Name{
				Local: "saml:Issuer",
			},
			Url: "", // caller must populate ar.AppSettings.AssertionConsumerServiceURL,
		},
		Signature: Signature{
			XMLName: xml.Name{
				Local: "samlsig:Signature",
			},
			Id: "Signature1",
			SignedInfo: SignedInfo{
				XMLName: xml.Name{
					Local: "samlsig:SignedInfo",
				},
				CanonicalizationMethod: CanonicalizationMethod{
					XMLName: xml.Name{
						Local: "samlsig:CanonicalizationMethod",
					},
					Algorithm: "http://www.w3.org/2001/10/xml-exc-c14n#",
				},
				SignatureMethod: SignatureMethod{
					XMLName: xml.Name{
						Local: "samlsig:SignatureMethod",
					},
					Algorithm: "http://www.w3.org/2000/09/xmldsig#rsa-sha1",
				},
				SamlsigReference: SamlsigReference{
					XMLName: xml.Name{
						Local: "samlsig:Reference",
					},
					URI: "", // caller must populate "#" + ar.Id,
					Transforms: Transforms{
						XMLName: xml.Name{
							Local: "samlsig:Transforms",
						},
						Transform: Transform{
							XMLName: xml.Name{
								Local: "samlsig:Transform",
							},
							Algorithm: "http://www.w3.org/2000/09/xmldsig#enveloped-signature",
						},
					},
					DigestMethod: DigestMethod{
						XMLName: xml.Name{
							Local: "samlsig:DigestMethod",
						},
						Algorithm: "http://www.w3.org/2000/09/xmldsig#sha1",
					},
					DigestValue: DigestValue{
						XMLName: xml.Name{
							Local: "samlsig:DigestValue",
						},
					},
				},
			},
			SignatureValue: SignatureValue{
				XMLName: xml.Name{
					Local: "samlsig:SignatureValue",
				},
			},
			KeyInfo: KeyInfo{
				XMLName: xml.Name{
					Local: "samlsig:KeyInfo",
				},
				X509Data: X509Data{
					XMLName: xml.Name{
						Local: "samlsig:X509Data",
					},
					X509Certificate: X509Certificate{
						XMLName: xml.Name{
							Local: "samlsig:X509Certificate",
						},
						Cert: "", // caller must populate cert,
					},
				},
			},
		},
		Status: Status{
			XMLName: xml.Name{
				Local: "samlp:Status",
			},
			StatusCode: StatusCode{
				XMLName: xml.Name{
					Local: "samlp:StatusCode",
				},
				// TODO unsuccesful responses??
				Value: "urn:oasis:names:tc:SAML:2.0:status:Success",
			},
		},
		Assertion: Assertion{
			XMLName: xml.Name{
				Local: "saml:Assertion",
			},
			XS:           "http://www.w3.org/2001/XMLSchema",
			XSI:          "http://www.w3.org/2001/XMLSchema-instance",
			SAML:         "urn:oasis:names:tc:SAML:2.0:assertion",
			Version:      "2.0",
			ID:           util.ID(),
			IssueInstant: time.Now().UTC().Format(time.RFC3339Nano),
			Issuer: Issuer{
				XMLName: xml.Name{
					Local: "saml:Issuer",
				},
				Url: "", // caller must populate ar.AppSettings.AssertionConsumerServiceURL,
			},
			Subject: Subject{
				XMLName: xml.Name{
					Local: "saml:Subject",
				},
				NameID: NameID{
					XMLName: xml.Name{
						Local: "saml:NameID",
					},
					Format: "urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified",
					Value:  "",
				},
				SubjectConfirmation: SubjectConfirmation{
					XMLName: xml.Name{
						Local: "saml:SubjectConfirmation",
					},
					Method: "urn:oasis:names:tc:SAML:2.0:cm:bearer",
					SubjectConfirmationData: SubjectConfirmationData{
						InResponseTo: "",
						NotOnOrAfter: time.Now().Add(time.Minute * 5).UTC().Format(time.RFC3339Nano),
						Recipient:    "",
					},
				},
			},
			Conditions: Conditions{
				XMLName: xml.Name{
					Local: "saml:Conditions",
				},
				NotBefore:    time.Now().Add(time.Minute * -5).UTC().Format(time.RFC3339Nano),
				NotOnOrAfter: time.Now().Add(time.Minute * 5).UTC().Format(time.RFC3339Nano),
			},
			AttributeStatement: AttributeStatement{
				XMLName: xml.Name{
					Local: "saml:AttributeStatement",
				},
				Attributes: []Attribute{},
			},
		},
	}
}

// AddAttribute add strong attribute to the Response
func (r *Response) AddAttribute(name, value string) {
	r.Assertion.AttributeStatement.Attributes = append(r.Assertion.AttributeStatement.Attributes, Attribute{
		XMLName: xml.Name{
			Local: "saml:Attribute",
		},
		Name:       name,
		NameFormat: "urn:oasis:names:tc:SAML:2.0:attrname-format:basic",
		AttributeValue: AttributeValue{
			XMLName: xml.Name{
				Local: "saml:AttributeValue",
			},
			Type:  "xs:string",
			Value: value,
		},
	})
}

func (r *Response) String() (string, error) {
	b, err := xml.MarshalIndent(r, "", "    ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (r *Response) OriginalString() string {
	return r.originalString
}

func (r *Response) SignedString(privateKeyPath string) (string, error) {
	s, err := r.String()
	if err != nil {
		return "", err
	}

	return SignResponse(s, privateKeyPath)
}

func (r *Response) EncodedSignedString(privateKeyPath string) (string, error) {
	signed, err := r.SignedString(privateKeyPath)
	if err != nil {
		return "", err
	}
	b64XML := base64.StdEncoding.EncodeToString([]byte(signed))
	return b64XML, nil
}

func (r *Response) CompressedEncodedSignedString(privateKeyPath string) (string, error) {
	signed, err := r.SignedString(privateKeyPath)
	if err != nil {
		return "", err
	}
	compressed := util.Compress([]byte(signed))
	b64XML := base64.StdEncoding.EncodeToString(compressed)
	return b64XML, nil
}

// GetAttribute by Name or by FriendlyName. Return blank string if not found
func (r *Response) GetAttribute(name string) string {
	attrStatement := AttributeStatement{}

	if r.IsEncrypted() {
		attrStatement = r.EncryptedAssertion.Assertion.AttributeStatement
	} else {
		attrStatement = r.Assertion.AttributeStatement
	}

	for _, attr := range attrStatement.Attributes {
		if attr.Name == name || attr.FriendlyName == name {
			return attr.AttributeValue.Value
		}
	}
	return ""
}
