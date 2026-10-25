// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package alexa proves that a request came from Alexa (ADR-0093): the signing certificate is
// fetched only from Amazon's own address, its chain is verified against the system's roots and
// names echo-api.amazon.com, and the body's SHA-256 signature is checked with its key.
package alexa

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// The two headers Alexa signs a request with.
const (
	HeaderCertChain = "SignatureCertChainUrl"
	HeaderSignature = "Signature-256"
)

// signerName is the name the signing certificate must carry.
const signerName = "echo-api.amazon.com"

// maxChainBytes bounds the certificate chain read.
const maxChainBytes = 64 * 1024

// Verifier checks Alexa's signature on a request.
type Verifier struct {
	// Client is the guarded outbound client (rule 6) the chain is read with.
	Client httpport.Port
	// Fetch replaces the client, for tests.
	Fetch func(ctx context.Context, address string) ([]byte, error)
	// Roots verify the chain; nil uses the system's.
	Roots *x509.CertPool
	Now   func() time.Time

	mu    sync.Mutex
	cache map[string]cachedChain
}

type cachedChain struct {
	leaf    *x509.Certificate
	expires time.Time
}

// errNotAlexa is the one answer every failed check gets: what failed tells a forger what to fix.
func errNotAlexa(reason string) error {
	return shared.ErrUnauthenticated.WithDetail("voice.not_alexa").WithCause(errors.New(reason))
}

// Verify proves the body was signed by Alexa.
func (v *Verifier) Verify(ctx context.Context, header http.Header, body []byte) error {
	address := header.Get(HeaderCertChain)
	if !IsAmazonChainURL(address) {
		return errNotAlexa("certificate URL not Amazon's")
	}
	signature, err := base64.StdEncoding.DecodeString(header.Get(HeaderSignature))
	if err != nil || len(signature) == 0 {
		return errNotAlexa("signature unreadable")
	}
	leaf, err := v.leaf(ctx, address)
	if err != nil {
		return err
	}
	key, ok := leaf.PublicKey.(*rsa.PublicKey)
	if !ok {
		return errNotAlexa("signing key not RSA")
	}
	digest := sha256.Sum256(body)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return errNotAlexa("signature does not match")
	}
	return nil
}

// IsAmazonChainURL is Amazon's rule for the certificate's address: https, s3.amazonaws.com on
// port 443, a path under /echo.api/ once normalised.
func IsAmazonChainURL(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), "s3.amazonaws.com") {
		return false
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return false
	}
	return strings.HasPrefix(path.Clean(parsed.Path), "/echo.api/")
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// leaf answers the verified signing certificate, from the cache while it is valid.
func (v *Verifier) leaf(ctx context.Context, address string) (*x509.Certificate, error) {
	now := v.now()
	v.mu.Lock()
	if cached, ok := v.cache[address]; ok && now.Before(cached.expires) {
		v.mu.Unlock()
		return cached.leaf, nil
	}
	v.mu.Unlock()

	read := v.fetch
	if v.Fetch != nil {
		read = v.Fetch
	}
	raw, err := read(ctx, address)
	if err != nil {
		return nil, errNotAlexa("certificate unreadable: " + err.Error())
	}
	var certificates []*x509.Certificate
	for rest := raw; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errNotAlexa("certificate unparseable")
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 {
		return nil, errNotAlexa("no certificate")
	}
	leaf := certificates[0]
	intermediates := x509.NewCertPool()
	for _, each := range certificates[1:] {
		intermediates.AddCert(each)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		DNSName: signerName, Roots: v.Roots, Intermediates: intermediates, CurrentTime: now,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, errNotAlexa("certificate chain not trusted: " + err.Error())
	}
	v.mu.Lock()
	if v.cache == nil {
		v.cache = map[string]cachedChain{}
	}
	// Kept an hour at most, and never past the certificate's own end.
	expires := now.Add(time.Hour)
	if leaf.NotAfter.Before(expires) {
		expires = leaf.NotAfter
	}
	v.cache[address] = cachedChain{leaf: leaf, expires: expires}
	v.mu.Unlock()
	return leaf, nil
}

func (v *Verifier) fetch(ctx context.Context, address string) ([]byte, error) {
	if v.Client == nil {
		return nil, errors.New("no outbound client")
	}
	response, err := v.Client.Do(ctx, httpport.Request{
		Method: http.MethodGet, URL: address, TargetClass: "alexa",
	})
	if err != nil {
		return nil, err
	}
	if response.Status != http.StatusOK {
		return nil, fmt.Errorf("status %d", response.Status)
	}
	if len(response.Body) > maxChainBytes {
		return nil, errors.New("certificate chain too large")
	}
	return response.Body, nil
}
