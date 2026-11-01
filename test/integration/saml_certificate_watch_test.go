// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func samlCertificateUntil(t *testing.T, from, until time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: from, NotAfter: until}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// identity.md §26.1 against a real database: the watch reads a workspace's SAML providers with the
// warning last given, records the next one, and neither reaches into another workspace (SG-3).
func TestTheSAMLCertificateWatchReadsAndMarksOnlyItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Second)
	end := now.AddDate(0, 0, 20).Truncate(time.Second)

	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: freshID(t), TenantID: idpTenantA, Issuer: "http://www.example.org/" + lettersOf(freshID(t)),
		ClientID: "https://hubtask.example/sp", Kind: "SAML", SAMLSSOURL: "https://idp.example.org/sso",
		SAMLCertificate:     samlCertificateUntil(t, now.AddDate(-1, 0, 0), end),
		AllowedEmailDomains: []string{"example.org"}, Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if _, err := providers.Insert(ctx, configured, cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte{0}}); err != nil {
			t.Fatalf("writing the provider: %v", err)
		}
		return nil
	})

	watchOf := func(watches []domain.SAMLWatch) (domain.SAMLWatch, bool) {
		for _, watch := range watches {
			if watch.ProviderID == configured.ID {
				return watch, true
			}
		}
		return domain.SAMLWatch{}, false
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		watches, err := providers.SAMLWatches(ctx)
		if err != nil {
			t.Fatalf("reading the watches: %v", err)
		}
		watch, found := watchOf(watches)
		if !found || watch.ToldDays != 0 || !watch.Enabled {
			t.Fatalf("watch %+v (found %v)", watch, found)
		}
		if days, _ := watch.Owed(now); days != 30 {
			t.Errorf("owed %d, want 30", days)
		}
		return providers.MarkSAMLExpiryTold(ctx, configured.ID, end, 30)
	})
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		watches, err := providers.SAMLWatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		watch, _ := watchOf(watches)
		if watch.ToldDays != 30 || !watch.ToldFor.Equal(end) {
			t.Errorf("recorded %d days for %v, want 30 for %v", watch.ToldDays, watch.ToldFor, end)
		}
		if days, _ := watch.Owed(now); days != 0 {
			t.Errorf("still owed %d after the warning", days)
		}
		return nil
	})

	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		watches, err := providers.SAMLWatches(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, found := watchOf(watches); found {
			t.Error("A's SAML provider is watched next door")
		}
		if err := providers.MarkSAMLExpiryTold(ctx, configured.ID, end, 7); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("B marked A's provider: %v", err)
		}
		return nil
	})

	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		_, err := providers.Delete(ctx, configured.ID, now)
		return err
	})
}
