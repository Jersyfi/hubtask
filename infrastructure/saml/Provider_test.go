// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package saml_test

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"encoding/pem"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"

	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/infrastructure/saml"
)

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

const (
	idpEntity = "http://www.example.org/exk1"
	spEntity  = "https://hubtask.example/api/v1/auth/saml:metadata"
	acs       = "https://hubtask.example/api/v1/auth/saml:acs"
	nonce     = "4bc1nonce"
)

// The answer a provider gives, before it is signed; each case bends one part.
type answer struct {
	inResponseTo, audience, recipient, issuer, status, notOnOrAfter, nameID string
	extraAssertion                                                          bool
}

func good(now time.Time) answer {
	return answer{
		inResponseTo: saml.RequestID(nonce), audience: spEntity, recipient: acs, issuer: idpEntity,
		status: "urn:oasis:names:tc:SAML:2.0:status:Success", nameID: "00u1lena",
		notOnOrAfter: now.Add(5 * time.Minute).Format(time.RFC3339),
	}
}

func (a answer) assertion(now time.Time) string {
	return `<saml:Assertion xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion" ID="_a1" Version="2.0" IssueInstant="` + now.Format(time.RFC3339) + `">` +
		`<saml:Issuer>` + a.issuer + `</saml:Issuer>` +
		`<saml:Subject><saml:NameID Format="urn:oasis:names:tc:SAML:2.0:nameid-format:persistent">` + a.nameID + `</saml:NameID>` +
		`<saml:SubjectConfirmation Method="urn:oasis:names:tc:SAML:2.0:cm:bearer"><saml:SubjectConfirmationData InResponseTo="` + a.inResponseTo + `" Recipient="` + a.recipient + `" NotOnOrAfter="` + a.notOnOrAfter + `"/></saml:SubjectConfirmation></saml:Subject>` +
		`<saml:Conditions NotBefore="` + now.Add(-time.Minute).Format(time.RFC3339) + `" NotOnOrAfter="` + a.notOnOrAfter + `"><saml:AudienceRestriction><saml:Audience>` + a.audience + `</saml:Audience></saml:AudienceRestriction></saml:Conditions>` +
		`<saml:AuthnStatement AuthnInstant="` + now.Format(time.RFC3339) + `"/>` +
		`<saml:AttributeStatement><saml:Attribute Name="email"><saml:AttributeValue>lena@example.org</saml:AttributeValue></saml:Attribute>` +
		`<saml:Attribute Name="displayName"><saml:AttributeValue>Lena Berg</saml:AttributeValue></saml:Attribute></saml:AttributeStatement>` +
		`</saml:Assertion>`
}

// signed answers the base64 response with its assertion signed by the key store.
func signed(t *testing.T, ks dsig.X509KeyStore, a answer, now time.Time, method string, tamper func(*etree.Element)) string {
	t.Helper()
	inner := etree.NewDocument()
	if err := inner.ReadFromString(a.assertion(now)); err != nil {
		t.Fatal(err)
	}
	signing := dsig.NewDefaultSigningContext(ks)
	signing.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	if method != "" {
		if err := signing.SetSignatureMethod(method); err != nil {
			t.Fatal(err)
		}
	}
	signedAssertion, err := signing.SignEnveloped(inner.Root())
	if err != nil {
		t.Fatal(err)
	}
	if tamper != nil {
		tamper(signedAssertion)
	}
	doc := etree.NewDocument()
	response := doc.CreateElement("samlp:Response")
	response.CreateAttr("xmlns:samlp", "urn:oasis:names:tc:SAML:2.0:protocol")
	response.CreateAttr("xmlns:saml", "urn:oasis:names:tc:SAML:2.0:assertion")
	response.CreateAttr("ID", "_r1")
	response.CreateAttr("Version", "2.0")
	response.CreateAttr("InResponseTo", a.inResponseTo)
	response.CreateAttr("Destination", acs)
	response.CreateElement("saml:Issuer").SetText(a.issuer)
	response.CreateElement("samlp:Status").CreateElement("samlp:StatusCode").CreateAttr("Value", a.status)
	response.AddChild(signedAssertion)
	if a.extraAssertion {
		evil := etree.NewDocument()
		other := a
		other.nameID = "00u1admin"
		if err := evil.ReadFromString(other.assertion(now)); err != nil {
			t.Fatal(err)
		}
		response.AddChild(evil.Root())
	}
	raw, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func config(t *testing.T, ks dsig.X509KeyStore) port.Config {
	_, der, err := ks.GetKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return port.Config{
		Issuer: idpEntity, ClientID: spEntity, RedirectURL: acs, SSOURL: "https://idp.example.org/sso",
		Certificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

// UC-ID-30 check 2: only an answer signed by the configured certificate, to this request, for this
// service and audience and within its time becomes an identity; every other is the one refusal.
func TestOnlyASignedAnswerToThisRequestBecomesAnIdentity(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ks := dsig.RandomKeyStoreForTest()
	stranger := dsig.RandomKeyStoreForTest()
	provider := saml.New(fixedClock{now})

	got, err := provider.Exchange(context.Background(), config(t, ks), port.Exchange{Code: signed(t, ks, good(now), now, "", nil), Nonce: nonce})
	if err != nil {
		t.Fatalf("a good answer was refused: %v", err)
	}
	if got.Subject != "00u1lena" || got.Email != "lena@example.org" || !got.EmailVerified ||
		got.DisplayName != "Lena Berg" || got.AddressAuthoritative || got.Directory != "" || got.AuthTime.IsZero() {
		t.Fatalf("identity %+v", got)
	}

	cases := map[string]func() string{
		"another request": func() string {
			a := good(now)
			a.inResponseTo = "_other"
			return signed(t, ks, a, now, "", nil)
		},
		"another audience": func() string {
			a := good(now)
			a.audience = "https://other.example"
			return signed(t, ks, a, now, "", nil)
		},
		"another recipient": func() string {
			a := good(now)
			a.recipient = "https://evil.example/acs"
			return signed(t, ks, a, now, "", nil)
		},
		"another issuer": func() string { a := good(now); a.issuer = "http://evil.example"; return signed(t, ks, a, now, "", nil) },
		"no success": func() string {
			a := good(now)
			a.status = "urn:oasis:names:tc:SAML:2.0:status:Requester"
			return signed(t, ks, a, now, "", nil)
		},
		"expired": func() string {
			a := good(now)
			a.notOnOrAfter = now.Add(-2 * time.Minute).Format(time.RFC3339)
			return signed(t, ks, a, now, "", nil)
		},
		"another key":        func() string { return signed(t, stranger, good(now), now, "", nil) },
		"a second assertion": func() string { a := good(now); a.extraAssertion = true; return signed(t, ks, a, now, "", nil) },
		"sha-1":              func() string { return signed(t, ks, good(now), now, dsig.RSASHA1SignatureMethod, nil) },
		"changed after signing": func() string {
			return signed(t, ks, good(now), now, "", func(el *etree.Element) {
				el.FindElement(".//NameID").SetText("00u1admin")
			})
		},
		"unsigned": func() string {
			raw := `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" ID="_r" InResponseTo="` + saml.RequestID(nonce) + `"><samlp:Status><samlp:StatusCode Value="urn:oasis:names:tc:SAML:2.0:status:Success"/></samlp:Status>` + good(now).assertion(now) + `</samlp:Response>`
			return base64.StdEncoding.EncodeToString([]byte(raw))
		},
		"a document type": func() string {
			return base64.StdEncoding.EncodeToString([]byte(`<!DOCTYPE x [<!ENTITY e "x">]><samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol"/>`))
		},
		"encrypted": func() string {
			return base64.StdEncoding.EncodeToString([]byte(`<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"><saml:EncryptedAssertion/></samlp:Response>`))
		},
		"not base64": func() string { return "%%%" },
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := provider.Exchange(context.Background(), config(t, ks), port.Exchange{Code: answer(), Nonce: nonce})
			if err == nil || !strings.Contains(err.Error(), "auth.identity_token_invalid") {
				t.Fatalf("want the one refusal, got %v", err)
			}
		})
	}
}

// The request leaves deflated with the state as RelayState, asking for a fresh sign-in only when
// the flow needs one.
func TestTheRequestCarriesTheStateAndTheService(t *testing.T) {
	provider := saml.New(fixedClock{time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)})
	cfg := port.Config{Issuer: idpEntity, ClientID: spEntity, RedirectURL: acs, SSOURL: "https://idp.example.org/sso?app=1"}
	raw, err := provider.AuthorizationURL(context.Background(), cfg, port.Authorization{State: "hbt_osf_x", Nonce: nonce, Fresh: true})
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(raw)
	query := parsed.Query()
	if query.Get("RelayState") != "hbt_osf_x" || query.Get("app") != "1" || parsed.Host != "idp.example.org" {
		t.Fatalf("url %s", raw)
	}
	deflated, err := base64.StdEncoding.DecodeString(query.Get("SAMLRequest"))
	if err != nil {
		t.Fatal(err)
	}
	inflated, err := io.ReadAll(flate.NewReader(bytes.NewReader(deflated)))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`ID="_4bc1nonce"`, `AssertionConsumerServiceURL="` + acs + `"`, `ForceAuthn="true"`, spEntity} {
		if !strings.Contains(string(inflated), want) {
			t.Fatalf("request %s lacks %s", inflated, want)
		}
	}
}
