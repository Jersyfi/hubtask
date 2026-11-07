// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type samlWatchStore struct {
	watches  []domain.SAMLWatch
	recorded int
}

func (s *samlWatchStore) SAMLWatches(context.Context) ([]domain.SAMLWatch, error) {
	return append([]domain.SAMLWatch(nil), s.watches...), nil
}

func (s *samlWatchStore) MarkSAMLExpiryTold(_ context.Context, id shared.ID, end time.Time, days int) error {
	for i := range s.watches {
		if s.watches[i].ProviderID == id {
			s.watches[i].ToldFor, s.watches[i].ToldDays = end, days
			return nil
		}
	}
	return shared.ErrNotFound
}

func (s *samlWatchStore) RecordSAMLCertificatesEnd(_ context.Context, id shared.ID, end time.Time) error {
	for i := range s.watches {
		if s.watches[i].ProviderID == id {
			s.watches[i].RecordedEnd = end
			s.recorded++
			return nil
		}
	}
	return shared.ErrNotFound
}

type samlTold struct{ providers []shared.ID }

func (t *samlTold) SAMLCertificateEnding(_ context.Context, _, providerID shared.ID) error {
	t.providers = append(t.providers, providerID)
	return nil
}

// identity.md §26.1: a certificate ending within 30 days is told once; the watch comes back when
// the 7-day warning is due, and a workspace with no SAML provider ends its watch.
func TestASAMLCertificateEndingIsToldOncePerWarning(t *testing.T) {
	at := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	end := at.AddDate(0, 0, 20)
	providerID := shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	store := &samlWatchStore{watches: []domain.SAMLWatch{{
		ProviderID: providerID, Enabled: true, Certificate: samlTestCertificateUntil(t, at, end),
	}}}
	told := &samlTold{}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}
	watch := WatchSAMLCertificates{Watches: store, Tell: told, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(at)}

	next, err := watch.Execute(t.Context(), actor)
	if err != nil || next == nil {
		t.Fatalf("watching: %v, next %v", err, next)
	}
	if len(told.providers) != 1 || store.watches[0].ToldDays != 30 {
		t.Fatalf("told %v, recorded %d days", told.providers, store.watches[0].ToldDays)
	}
	if want := at.Add(samlWatchInterval); !next.Equal(want) {
		t.Errorf("next look %v, want %v (a day: the 7-day warning is further)", next, want)
	}
	if _, err := watch.Execute(t.Context(), actor); err != nil || len(told.providers) != 1 {
		t.Errorf("a second pass told again: %v, %v", told.providers, err)
	}

	store.watches = nil
	if next, err := watch.Execute(t.Context(), actor); err != nil || next != nil {
		t.Errorf("no SAML provider kept the watch: %v, %v", next, err)
	}
}

// Saving a SAML provider seeds its workspace's watch; nothing else would start it.
func TestSavingASAMLProviderSeedsTheCertificateWatch(t *testing.T) {
	at := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	f := newProviderFixture(at)
	jobs := &ldapJobs{}
	f.writer.Session.Jobs = jobs
	cmd := ConfigureIdentityProviderCommand{
		Issuer: "http://www.example.org/exk1", ClientID: "https://hubtask.example/sp", Kind: "SAML",
		SAMLSSOURL: "https://idp.example.org/sso", SAMLCertificate: samlTestCertificate(t, at),
		AllowedEmailDomains: []string{"example.org"},
	}
	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).Execute(t.Context(), providerActor(), cmd); err != nil {
		t.Fatalf("configuring: %v", err)
	}
	seeded := false
	for _, request := range jobs.requests {
		if request.Kind == queue.KindSAMLCertificateWatch && request.TenantID == tenant {
			seeded = true
		}
	}
	if !seeded {
		t.Errorf("no watch was seeded: %+v", jobs.requests)
	}
}

// The installation's count of ending certificates reads the end the watch keeps on every SAML
// provider (identity.md §26.1): written when it changes - a new certificate pasted moves it - and
// left alone while it does not.
func TestTheWatchKeepsEveryProvidersCertificateEnd(t *testing.T) {
	at := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	end := at.AddDate(1, 0, 0)
	providerID := shared.ID("01936f2a-7c1e-7000-8000-0000000000c2")
	store := &samlWatchStore{watches: []domain.SAMLWatch{{
		ProviderID: providerID, Certificate: samlTestCertificateUntil(t, at, end),
	}}}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}
	watch := WatchSAMLCertificates{Watches: store, Tell: &samlTold{}, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(at)}

	if _, err := watch.Execute(t.Context(), actor); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.watches[0].CertificatesEnd(); !store.watches[0].RecordedEnd.Equal(got) || store.recorded != 1 {
		t.Fatalf("recorded %v (%d writes), want %v", store.watches[0].RecordedEnd, store.recorded, got)
	}
	if _, err := watch.Execute(t.Context(), actor); err != nil || store.recorded != 1 {
		t.Errorf("an unchanged end was written again: %d writes, %v", store.recorded, err)
	}
}
