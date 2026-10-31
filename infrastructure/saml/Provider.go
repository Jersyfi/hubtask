// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package saml is the SAML 2.0 service provider, and the only package in this repository that knows
// what XML signature verification is (identity.md §26, ADR-0099).
//
// It implements the same relying-party port as `infrastructure/oidc`: an authentication request on
// the way out (HTTP-Redirect binding, unsigned), and on the way back an answer that becomes an
// identity only after every check passed - the signature against the configured certificates, and
// only the signed element read afterwards (never the tree it came in, which is how signature
// wrapping hands over somebody else's assertion), the issuer exactly, the audience, the recipient,
// the request it answers, and the time window with a minute's skew. No encrypted assertion, no
// SHA-1, no document type declaration.
package saml

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

const (
	maxClockSkew = 60 * time.Second
	// maxAnswer bounds the decoded answer; the posted base64 is bounded before it is kept.
	maxAnswer = 256 << 10

	nsProtocol  = "urn:oasis:names:tc:SAML:2.0:protocol"
	nsAssertion = "urn:oasis:names:tc:SAML:2.0:assertion"
	nsSignature = "http://www.w3.org/2000/09/xmldsig#"
	statusOK    = "urn:oasis:names:tc:SAML:2.0:status:Success"
	bindingPOST = "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST"
	bearer      = "urn:oasis:names:tc:SAML:2.0:cm:bearer" //nolint:gosec // G101: a confirmation method's name, not a credential
	formatEmail = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"
)

// The attribute names an address and a name are read from, as the common providers publish them.
var (
	emailAttributes = []string{
		"email", "mail", "emailaddress",
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
		"urn:oid:0.9.2342.19200300.100.1.3",
	}
	nameAttributes = []string{
		"displayname", "name",
		"http://schemas.microsoft.com/identity/claims/displayname",
		"urn:oid:2.16.840.1.113730.3.1.241",
	}
	weakAlgorithms = []string{
		"http://www.w3.org/2000/09/xmldsig#rsa-sha1",
		"http://www.w3.org/2000/09/xmldsig#dsa-sha1",
		"http://www.w3.org/2000/09/xmldsig#sha1",
	}
)

// Provider is the service provider. It holds no workspace's configuration: each call carries it.
type Provider struct {
	clock clock.Clock
}

// New builds the service provider on the installation's clock.
func New(clk clock.Clock) *Provider { return &Provider{clock: clk} }

// Check has nothing to ask: a SAML provider publishes no discovery, and its certificate and address
// were checked by the domain when the configuration was read.
func (p *Provider) Check(context.Context, string) error { return nil }

// RequestID is the authentication request's identifier for a flow's nonce: an XML ID must not
// begin with a digit, which a random nonce may.
func RequestID(nonce string) string { return "_" + nonce }

// AuthorizationURL is the single sign-on address with the deflated, encoded request and the state.
func (p *Provider) AuthorizationURL(_ context.Context, cfg port.Config, auth port.Authorization) (string, error) {
	target, err := url.Parse(cfg.SSOURL)
	if err != nil || target.Scheme != "https" {
		return "", shared.ErrValidation.WithDetail("identity_provider.saml_sso_url_invalid")
	}
	doc := etree.NewDocument()
	request := doc.CreateElement("samlp:AuthnRequest")
	request.CreateAttr("xmlns:samlp", nsProtocol)
	request.CreateAttr("xmlns:saml", nsAssertion)
	request.CreateAttr("ID", RequestID(auth.Nonce))
	request.CreateAttr("Version", "2.0")
	request.CreateAttr("IssueInstant", p.clock.Now().UTC().Format(time.RFC3339))
	request.CreateAttr("Destination", cfg.SSOURL)
	request.CreateAttr("AssertionConsumerServiceURL", cfg.RedirectURL)
	request.CreateAttr("ProtocolBinding", bindingPOST)
	if auth.Fresh {
		request.CreateAttr("ForceAuthn", "true")
	}
	request.CreateElement("saml:Issuer").SetText(cfg.ClientID)
	policy := request.CreateElement("samlp:NameIDPolicy")
	policy.CreateAttr("AllowCreate", "true")
	raw, err := doc.WriteToBytes()
	if err != nil {
		return "", shared.ErrInternal.WithCause(err)
	}
	var deflated bytes.Buffer
	writer, err := flate.NewWriter(&deflated, flate.BestCompression)
	if err != nil {
		return "", shared.ErrInternal.WithCause(err)
	}
	if _, err := writer.Write(raw); err != nil {
		return "", shared.ErrInternal.WithCause(err)
	}
	if err := writer.Close(); err != nil {
		return "", shared.ErrInternal.WithCause(err)
	}
	query := target.Query()
	query.Set("SAMLRequest", base64.StdEncoding.EncodeToString(deflated.Bytes()))
	query.Set("RelayState", auth.State)
	if auth.LoginHint != "" {
		query.Set("login_hint", auth.LoginHint)
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

// Exchange verifies the provider's answer (the exchange's Code) and reads the identity out of the
// signed assertion. Every failure is the one refusal.
func (p *Provider) Exchange(_ context.Context, cfg port.Config, exchange port.Exchange) (port.Identity, error) {
	identity, err := p.verify(cfg, exchange)
	if err != nil {
		return port.Identity{}, shared.ErrValidation.WithDetail("auth.identity_token_invalid").
			WithCause(fmt.Errorf("verifying the SAML answer: %w", err))
	}
	return identity, nil
}

func (p *Provider) verify(cfg port.Config, exchange port.Exchange) (port.Identity, error) {
	certs, err := certificates(cfg.Certificate)
	if err != nil {
		return port.Identity{}, err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(exchange.Code), ""))
	if err != nil || len(decoded) == 0 || len(decoded) > maxAnswer {
		return port.Identity{}, fmt.Errorf("the answer is not base64 of a bounded size")
	}
	doc := etree.NewDocument()
	doc.ReadSettings = etree.ReadSettings{Permissive: false}
	if err := doc.ReadFromBytes(decoded); err != nil {
		return port.Identity{}, fmt.Errorf("the answer is not XML: %w", err)
	}
	for _, token := range doc.Child {
		if _, isDirective := token.(*etree.Directive); isDirective {
			return port.Identity{}, fmt.Errorf("a document type declaration is refused")
		}
	}
	response := doc.Root()
	if response == nil || response.Tag != "Response" || response.NamespaceURI() != nsProtocol {
		return port.Identity{}, fmt.Errorf("the root is not a samlp:Response")
	}
	if len(response.FindElements("//EncryptedAssertion")) > 0 {
		return port.Identity{}, fmt.Errorf("an encrypted assertion is not supported")
	}
	now := p.clock.Now()
	validator := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: certs})
	validator.Clock = dsig.NewFakeClockAt(now)

	// The response's own envelope first: signed, its validated copy is what is read from here on.
	signedResponse := false
	if signature := directChild(response, nsSignature, "Signature"); signature != nil {
		if weak(signature) {
			return port.Identity{}, fmt.Errorf("a SHA-1 signature is refused")
		}
		validated, err := validator.Validate(response)
		if err != nil {
			return port.Identity{}, fmt.Errorf("the response's signature: %w", err)
		}
		response, signedResponse = validated, true
	}
	if err := p.checkResponse(response, cfg, exchange); err != nil {
		return port.Identity{}, err
	}
	assertions := directChildren(response, nsAssertion, "Assertion")
	if len(assertions) != 1 {
		return port.Identity{}, fmt.Errorf("an answer carries exactly one assertion")
	}
	assertion := assertions[0]
	if signature := directChild(assertion, nsSignature, "Signature"); signature != nil {
		if weak(signature) {
			return port.Identity{}, fmt.Errorf("a SHA-1 signature is refused")
		}
		validated, err := validator.Validate(assertion)
		if err != nil {
			return port.Identity{}, fmt.Errorf("the assertion's signature: %w", err)
		}
		assertion = validated
	} else if !signedResponse {
		return port.Identity{}, fmt.Errorf("neither the response nor the assertion is signed")
	}
	return p.readAssertion(assertion, cfg, exchange, now)
}

func (p *Provider) checkResponse(response *etree.Element, cfg port.Config, exchange port.Exchange) error {
	if response.SelectAttrValue("InResponseTo", "") != RequestID(exchange.Nonce) {
		return fmt.Errorf("the answer is to another request")
	}
	if destination := response.SelectAttrValue("Destination", ""); destination != "" && destination != cfg.RedirectURL {
		return fmt.Errorf("the answer is for another service")
	}
	if issuer := directChild(response, nsAssertion, "Issuer"); issuer != nil && strings.TrimSpace(issuer.Text()) != cfg.Issuer {
		return fmt.Errorf("the answer is from another issuer")
	}
	status := directChild(response, nsProtocol, "Status")
	if status == nil {
		return fmt.Errorf("the answer has no status")
	}
	code := directChild(status, nsProtocol, "StatusCode")
	if code == nil || code.SelectAttrValue("Value", "") != statusOK {
		return fmt.Errorf("the provider did not answer success")
	}
	return nil
}

func (p *Provider) readAssertion(
	assertion *etree.Element, cfg port.Config, exchange port.Exchange, now time.Time,
) (port.Identity, error) {
	issuer := directChild(assertion, nsAssertion, "Issuer")
	if issuer == nil || strings.TrimSpace(issuer.Text()) != cfg.Issuer {
		return port.Identity{}, fmt.Errorf("the assertion is from another issuer")
	}
	conditions := directChild(assertion, nsAssertion, "Conditions")
	if conditions == nil {
		return port.Identity{}, fmt.Errorf("the assertion has no conditions")
	}
	if err := within(conditions, now, true); err != nil {
		return port.Identity{}, err
	}
	audienceHeld := false
	for _, restriction := range directChildren(conditions, nsAssertion, "AudienceRestriction") {
		for _, audience := range directChildren(restriction, nsAssertion, "Audience") {
			if strings.TrimSpace(audience.Text()) == cfg.ClientID {
				audienceHeld = true
			}
		}
	}
	if !audienceHeld {
		return port.Identity{}, fmt.Errorf("the assertion is for another audience")
	}
	subject := directChild(assertion, nsAssertion, "Subject")
	if subject == nil {
		return port.Identity{}, fmt.Errorf("the assertion has no subject")
	}
	nameID := directChild(subject, nsAssertion, "NameID")
	if nameID == nil || strings.TrimSpace(nameID.Text()) == "" {
		return port.Identity{}, fmt.Errorf("the assertion names nobody")
	}
	confirmed := false
	for _, confirmation := range directChildren(subject, nsAssertion, "SubjectConfirmation") {
		if confirmation.SelectAttrValue("Method", "") != bearer {
			continue
		}
		data := directChild(confirmation, nsAssertion, "SubjectConfirmationData")
		if data == nil || data.SelectAttrValue("Recipient", "") != cfg.RedirectURL ||
			data.SelectAttrValue("NotOnOrAfter", "") == "" {
			continue
		}
		if to := data.SelectAttrValue("InResponseTo", ""); to != "" && to != RequestID(exchange.Nonce) {
			continue
		}
		if within(data, now, false) == nil {
			confirmed = true
		}
	}
	if !confirmed {
		return port.Identity{}, fmt.Errorf("no bearer confirmation for this service holds")
	}

	identity := port.Identity{Subject: strings.TrimSpace(nameID.Text())}
	attributes := attributesOf(assertion)
	identity.Email = first(attributes, emailAttributes)
	if identity.Email == "" && nameID.SelectAttrValue("Format", "") == formatEmail {
		identity.Email = identity.Subject
	}
	// The address comes signed by the provider this workspace configured for its own domains;
	// whom it admits is still `allowed_email_domains`, and it is never authoritative (§26).
	identity.EmailVerified = identity.Email != ""
	identity.DisplayName = first(attributes, nameAttributes)
	if identity.DisplayName == "" {
		given, surname := first(attributes, []string{"givenname",
			"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname"}),
			first(attributes, []string{"surname", "sn",
				"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname"})
		identity.DisplayName = strings.TrimSpace(given + " " + surname)
	}
	if statement := directChild(assertion, nsAssertion, "AuthnStatement"); statement != nil {
		if at, err := time.Parse(time.RFC3339Nano, statement.SelectAttrValue("AuthnInstant", "")); err == nil {
			identity.AuthTime = at.UTC()
		}
	}
	return identity, nil
}

// within checks NotBefore and NotOnOrAfter with a minute's skew; endOptional lets NotOnOrAfter be
// absent, which the conditions may and a bearer confirmation may not.
func within(el *etree.Element, now time.Time, endOptional bool) error {
	if raw := el.SelectAttrValue("NotBefore", ""); raw != "" {
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || now.Add(maxClockSkew).Before(at) {
			return fmt.Errorf("not valid yet")
		}
	}
	raw := el.SelectAttrValue("NotOnOrAfter", "")
	if raw == "" {
		if endOptional {
			return nil
		}
		return fmt.Errorf("no end")
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || !now.Add(-maxClockSkew).Before(at) {
		return fmt.Errorf("no longer valid")
	}
	return nil
}

func attributesOf(assertion *etree.Element) map[string]string {
	out := map[string]string{}
	for _, statement := range directChildren(assertion, nsAssertion, "AttributeStatement") {
		for _, attribute := range directChildren(statement, nsAssertion, "Attribute") {
			name := strings.ToLower(attribute.SelectAttrValue("Name", ""))
			if values := directChildren(attribute, nsAssertion, "AttributeValue"); len(values) > 0 {
				if _, held := out[name]; !held {
					out[name] = strings.TrimSpace(values[0].Text())
				}
			}
		}
	}
	return out
}

func first(attributes map[string]string, names []string) string {
	for _, name := range names {
		if value := attributes[strings.ToLower(name)]; value != "" {
			return value
		}
	}
	return ""
}

func weak(signature *etree.Element) bool {
	for _, tag := range []string{"SignatureMethod", "DigestMethod"} {
		for _, el := range signature.FindElements(".//" + tag) {
			algorithm := el.SelectAttrValue("Algorithm", "")
			for _, refused := range weakAlgorithms {
				if algorithm == refused {
					return true
				}
			}
		}
	}
	return false
}

func directChild(parent *etree.Element, namespace, tag string) *etree.Element {
	if children := directChildren(parent, namespace, tag); len(children) > 0 {
		return children[0]
	}
	return nil
}

func directChildren(parent *etree.Element, namespace, tag string) []*etree.Element {
	var out []*etree.Element
	for _, child := range parent.ChildElements() {
		if child.Tag == tag && child.NamespaceURI() == namespace {
			out = append(out, child)
		}
	}
	return out
}

func certificates(raw string) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	rest := []byte(raw)
	for {
		block, remainder := pem.Decode(rest)
		if block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, cert)
		rest = remainder
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no certificate is configured")
	}
	return out, nil
}

// Metadata is this installation's service provider metadata: its entity ID and the assertion
// consumer service, assertions wanted signed.
func Metadata(entityID, acsURL string) ([]byte, error) {
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)
	descriptor := doc.CreateElement("md:EntityDescriptor")
	descriptor.CreateAttr("xmlns:md", "urn:oasis:names:tc:SAML:2.0:metadata")
	descriptor.CreateAttr("entityID", entityID)
	sp := descriptor.CreateElement("md:SPSSODescriptor")
	sp.CreateAttr("AuthnRequestsSigned", "false")
	sp.CreateAttr("WantAssertionsSigned", "true")
	sp.CreateAttr("protocolSupportEnumeration", nsProtocol)
	sp.CreateElement("md:NameIDFormat").SetText("urn:oasis:names:tc:SAML:2.0:nameid-format:persistent")
	sp.CreateElement("md:NameIDFormat").SetText(formatEmail)
	acs := sp.CreateElement("md:AssertionConsumerService")
	acs.CreateAttr("Binding", bindingPOST)
	acs.CreateAttr("Location", acsURL)
	acs.CreateAttr("index", "0")
	acs.CreateAttr("isDefault", "true")
	doc.Indent(2)
	return doc.WriteToBytes()
}
