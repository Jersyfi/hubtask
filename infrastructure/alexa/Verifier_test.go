// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package alexa

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const chainURL = "https://s3.amazonaws.com/echo.api/echo-api-cert-12.pem"

// signer is a test authority and a signing certificate it issued for a name.
type signer struct {
	roots *x509.CertPool
	chain []byte
	key   *rsa.PrivateKey
}

func newSigner(t *testing.T, name string, notAfter time.Time) signer {
	t.Helper()
	caKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	leafKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	return signer{roots: roots, chain: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), key: leafKey}
}

func (s signer) sign(body []byte) string {
	digest := sha256.Sum256(body)
	signature, _ := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	return base64.StdEncoding.EncodeToString(signature)
}

func (s signer) verifier() *Verifier {
	return &Verifier{Roots: s.roots, Fetch: func(context.Context, string) ([]byte, error) { return s.chain, nil }}
}

func headers(address, signature string) http.Header {
	header := http.Header{}
	header.Set(HeaderCertChain, address)
	header.Set(HeaderSignature, signature)
	return header
}

func TestAlexasSignatureIsAccepted(t *testing.T) {
	alexa := newSigner(t, "echo-api.amazon.com", time.Now().Add(24*time.Hour))
	body := []byte(`{"version":"1.0"}`)
	if err := alexa.verifier().Verify(context.Background(), headers(chainURL, alexa.sign(body)), body); err != nil {
		t.Fatalf("a signed request was refused: %v", err)
	}
}

func TestWhatIsNotAlexaIsRefused(t *testing.T) {
	alexa := newSigner(t, "echo-api.amazon.com", time.Now().Add(24*time.Hour))
	other := newSigner(t, "evil.example.com", time.Now().Add(24*time.Hour))
	expired := newSigner(t, "echo-api.amazon.com", time.Now().Add(-time.Minute))
	body := []byte(`{"version":"1.0"}`)
	cases := map[string]struct {
		verifier *Verifier
		header   http.Header
		body     []byte
	}{
		"a changed body":         {alexa.verifier(), headers(chainURL, alexa.sign(body)), []byte(`{"version":"2.0"}`)},
		"another name":           {other.verifier(), headers(chainURL, other.sign(body)), body},
		"an expired certificate": {expired.verifier(), headers(chainURL, expired.sign(body)), body},
		"no signature":           {alexa.verifier(), headers(chainURL, ""), body},
		"a foreign chain URL":    {alexa.verifier(), headers("https://evil.example.com/echo.api/cert.pem", alexa.sign(body)), body},
	}
	for name, each := range cases {
		t.Run(name, func(t *testing.T) {
			if err := each.verifier.Verify(context.Background(), each.header, each.body); !errors.Is(err, shared.ErrUnauthenticated) {
				t.Errorf("answered %v", err)
			}
		})
	}
}

func TestAmazonsChainAddressRule(t *testing.T) {
	cases := map[string]bool{
		"https://s3.amazonaws.com/echo.api/echo-api-cert.pem":        true,
		"https://s3.amazonaws.com:443/echo.api/echo-api-cert.pem":    true,
		"https://s3.amazonaws.com/echo.api/../echo.api/echo-api.pem": true,
		"HTTPS://S3.AMAZONAWS.COM/echo.api/echo-api-cert.pem":        true,
		"http://s3.amazonaws.com/echo.api/echo-api-cert.pem":         false,
		"https://notamazon.com/echo.api/echo-api-cert.pem":           false,
		"https://s3.amazonaws.com/EcHo.aPi/echo-api-cert.pem":        false,
		"https://s3.amazonaws.com/invalid.path/echo-api-cert.pem":    false,
		"https://s3.amazonaws.com:563/echo.api/echo-api-cert.pem":    false,
		"https://s3.amazonaws.com/echo.api/../invalid/echo-api.pem":  false,
	}
	for address, want := range cases {
		if _, got := AmazonChainURL(address); got != want {
			t.Errorf("AmazonChainURL(%q) = %t", address, got)
		}
	}
	// What is fetched is Amazon's host and the normalised path, and nothing else of the header.
	for address, want := range map[string]string{
		"HTTPS://S3.AMAZONAWS.COM:443/echo.api/../echo.api/echo-api.pem":   "https://s3.amazonaws.com/echo.api/echo-api.pem",
		"https://user@s3.amazonaws.com/echo.api/echo-api-cert.pem?x=1#top": "https://s3.amazonaws.com/echo.api/echo-api-cert.pem",
	} {
		if got, ok := AmazonChainURL(address); !ok || got != want {
			t.Errorf("AmazonChainURL(%q) = %q, %t; want %q", address, got, ok, want)
		}
	}
}
