// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	automationrepo "github.com/Jersyfi/hubtask/core/application/repository/automation"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	integrationrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The re-seal's five views of the stores and the census over them (ADR-0045, #368), against the
// real boundary. Gate SG-3: one negative per port method - a re-seal reaches every table that
// holds a credential to something outside this system, and it must reach only its own tenant's.

var (
	_ identityrepo.MfaSealings             = postgres.MfaRepository{}
	_ identityrepo.IdentityProviderSealing = postgres.IdentityProviderRepository{}
	_ integrationrepo.WebhookSealings      = postgres.WebhookSubscriptionRepository{}
	_ backuprepo.CredentialSealings        = postgres.BackupTargetRepository{}
	_ automationrepo.RuleSealings          = postgres.AutomationRuleRepository{}
)

func enrolmentListed(listed []identityrepo.MfaEnrollment, accountID shared.ID, keyID string) bool {
	for _, row := range listed {
		if row.AccountID == accountID && row.Secret.KeyID == keyID {
			return true
		}
	}
	return false
}

func subscriptionListed(listed []integrationrepo.StoredSubscription, id shared.ID, keyID string) bool {
	for _, row := range listed {
		if row.Subscription.ID == id && row.Secret.KeyID == keyID {
			return true
		}
	}
	return false
}

func movedSecret(keyID string) cryptoport.Sealed {
	return cryptoport.Sealed{KeyID: keyID, Ciphertext: []byte("rewrapped-not-a-secret")}
}

// Gate SG-3: MfaSealings.SealedNotUnder, MfaSealings.Rewrap.
func TestAnEnrolmentIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, sealedSecret(), now); err != nil {
			t.Fatalf("enrolling: %v", err)
		}
		return nil
	})

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		listed, err := mfa.SealedNotUnder(ctx, "k2")
		if err != nil || len(listed) != 0 {
			t.Errorf("another tenant's enrolment was listed for re-sealing (%v, %v)", listed, err)
		}
		if moved, err := mfa.Rewrap(ctx, sessionAccountA, movedSecret("k2"), "k1"); err != nil || moved {
			t.Errorf("another tenant's enrolment was rewrapped from here (%v, %v)", moved, err)
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		// Found by identity rather than counted: the database is shared with every other test
		// in the package, and their enrolments under other keys are listed beside this one.
		listed, err := mfa.SealedNotUnder(ctx, "k2")
		if err != nil || !enrolmentListed(listed, sessionAccountA, "k1") {
			t.Fatalf("the enrolment to re-seal was not listed (%d, %v)", len(listed), err)
		}
		// Guarded by the key the row named: a stale expectation moves nothing.
		if moved, err := mfa.Rewrap(ctx, sessionAccountA, movedSecret("k2"), "k0"); err != nil || moved {
			t.Errorf("a stale rewrap changed the row (%v, %v)", moved, err)
		}
		if moved, err := mfa.Rewrap(ctx, sessionAccountA, movedSecret("k2"), "k1"); err != nil || !moved {
			t.Fatalf("rewrapping (%v, %v)", moved, err)
		}
		if listed, err := mfa.SealedNotUnder(ctx, "k2"); err != nil || enrolmentListed(listed, sessionAccountA, "k1") {
			t.Errorf("a rewrapped enrolment is still listed (%d, %v)", len(listed), err)
		}
		return nil
	})
}

// Gate SG-3: IdentityProviderSealing.RewrapSecret.
func TestAProviderSecretIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	configured, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		TenantID: idpTenantA, Issuer: "https://login.reseal.example", ClientID: "hubtask-a",
		AllowedEmailDomains: []string{"a.example"}, Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		_, err := providers.Upsert(ctx, configured, sealedSecret(), now)
		return err
	})

	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		if moved, err := providers.RewrapSecret(ctx, movedSecret("k2"), "k1"); err != nil || moved {
			t.Errorf("another tenant's provider secret was rewrapped from here (%v, %v)", moved, err)
		}
		return nil
	})

	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if moved, err := providers.RewrapSecret(ctx, movedSecret("k2"), "k1"); err != nil || !moved {
			t.Fatalf("rewrapping (%v, %v)", moved, err)
		}
		_, sealed, err := providers.FindWithSecret(ctx)
		if err != nil || sealed.KeyID != "k2" {
			t.Errorf("the provider still names %q after the rewrap (%v)", sealed.KeyID, err)
		}
		return nil
	})
}

// Gate SG-3: WebhookSealings.SealedNotUnder, WebhookSealings.Rewrap.
func TestASubscriptionIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	subscription := seedSubscription(ctx, t, tenantA)
	subscriptions := postgres.NewWebhookSubscriptionRepository()
	moved := integrationrepo.SealedSecret{KeyID: "k2", Ciphertext: []byte("rewrapped-not-a-secret")}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		listed, err := subscriptions.SealedNotUnder(ctx, "k2")
		if err != nil || len(listed) != 0 {
			t.Errorf("another tenant's subscription was listed for re-sealing (%v, %v)", listed, err)
		}
		if ok, err := subscriptions.Rewrap(ctx, subscription.ID, moved, integrationrepo.SealedSecret{}, subscription.Version); err != nil || ok {
			t.Errorf("another tenant's subscription was rewrapped from here (%v, %v)", ok, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		listed, err := subscriptions.SealedNotUnder(ctx, "k2")
		if err != nil || !subscriptionListed(listed, subscription.ID, "k1") {
			t.Fatalf("the subscription to re-seal was not listed (%d, %v)", len(listed), err)
		}
		if ok, err := subscriptions.Rewrap(ctx, subscription.ID, moved, integrationrepo.SealedSecret{}, subscription.Version+7); err != nil || ok {
			t.Errorf("a stale version rewrapped the row (%v, %v)", ok, err)
		}
		if ok, err := subscriptions.Rewrap(ctx, subscription.ID, moved, integrationrepo.SealedSecret{}, subscription.Version); err != nil || !ok {
			t.Fatalf("rewrapping (%v, %v)", ok, err)
		}
		stored, err := subscriptions.Find(ctx, subscription.ID)
		if err != nil || stored.Secret.KeyID != "k2" || stored.Subscription.Version != subscription.Version {
			t.Errorf("after the rewrap: key %q, version %d (%v)", stored.Secret.KeyID, stored.Subscription.Version, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Gate SG-3: CredentialSealings.SealedNotUnder, CredentialSealings.Rewrap.
func TestATargetCredentialIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, sealedCredential("reseal"))
	targets := targetRepo()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		listed, err := targets.SealedNotUnder(ctx, "k2")
		for _, credential := range listed {
			if credential.TargetID == target.ID {
				t.Error("another tenant's credential was listed for re-sealing")
			}
		}
		if err != nil {
			t.Error(err)
		}
		if ok, err := targets.Rewrap(ctx, target.ID, movedSecret("k2"), "k2026"); err != nil || ok {
			t.Errorf("another tenant's credential was rewrapped from here (%v, %v)", ok, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		listed, err := targets.SealedNotUnder(ctx, "k2")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, credential := range listed {
			found = found || (credential.TargetID == target.ID && credential.Credential.KeyID == "k2026")
		}
		if !found {
			t.Fatal("the credential to re-seal was not listed")
		}
		if ok, err := targets.Rewrap(ctx, target.ID, movedSecret("k2"), "k2026"); err != nil || !ok {
			t.Fatalf("rewrapping (%v, %v)", ok, err)
		}
		sealed, err := targets.Credential(ctx, target.ID)
		if err != nil || sealed.KeyID != "k2" {
			t.Errorf("the credential still names %q after the rewrap (%v)", sealed.KeyID, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// sealedHTTPRule is a rule whose HTTP_REQUEST action carries a header secret sealed under keyID,
// the stored shape sealActions writes (core/application/service/automation/Outbound.go).
func sealedHTTPRule(t *testing.T, tenant, runAs, author shared.ID, keyID string) automation.Rule {
	t.Helper()
	id := freshID(t)
	rule, err := automation.NewRule(automation.NewRuleInput{
		ID: id, TenantID: tenant, Name: freshName(t),
		Scope: automation.Scope{Type: automation.ScopeTenant}, RunAs: runAs,
		Trigger: automation.Trigger{Kind: automation.TriggerEvent, EventType: event.ItemOverdue},
		Actions: []automation.Action{{
			Kind: automation.ActionHTTPRequest,
			Params: map[string]any{
				"method": "POST", "url": "https://hooks.example/reseal",
				"secret_header_name": "X-Token",
				"secret_header_sealed": automation.SealedSecret{
					Ciphertext: base64.StdEncoding.EncodeToString([]byte("sealed-not-a-secret")),
					KeyID:      keyID,
					Purpose:    "automation.rule.http:" + id.String(),
				}.Document(),
			},
		}},
		Throttle:  automation.Throttle{MaxRunsPerHour: 100},
		OnError:   automation.OnErrorContinue,
		CreatedBy: author, Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	return rule
}

// Gate SG-3: RuleSealings.SealedNotUnder, RuleSealings.RewrapActions.
func TestARuleIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)
	rule := sealedHTTPRule(t, tenantA, runAs, authorA, "k1")
	rules := automationRules()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return rules.Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	rewrapped := rule.Actions
	rewrapped[0].Params["secret_header_sealed"] = automation.SealedSecret{
		Ciphertext: base64.StdEncoding.EncodeToString([]byte("rewrapped-not-a-secret")),
		KeyID:      "k2",
		Purpose:    "automation.rule.http:" + rule.ID.String(),
	}.Document()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		listed, err := rules.SealedNotUnder(ctx, "k2")
		for _, candidate := range listed {
			if candidate.ID == rule.ID {
				t.Error("another tenant's rule was listed for re-sealing")
			}
		}
		if err != nil {
			t.Error(err)
		}
		if ok, err := rules.RewrapActions(ctx, rule.ID, rewrapped, rule.Version); err != nil || ok {
			t.Errorf("another tenant's rule was rewrapped from here (%v, %v)", ok, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		listed, err := rules.SealedNotUnder(ctx, "k2")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, candidate := range listed {
			found = found || candidate.ID == rule.ID
		}
		if !found {
			t.Fatal("the rule to re-seal was not listed")
		}
		if ok, err := rules.RewrapActions(ctx, rule.ID, rewrapped, rule.Version); err != nil || !ok {
			t.Fatalf("rewrapping (%v, %v)", ok, err)
		}
		listed, err = rules.SealedNotUnder(ctx, "k2")
		if err != nil {
			t.Fatal(err)
		}
		for _, candidate := range listed {
			if candidate.ID == rule.ID {
				t.Error("a rewrapped rule is still listed")
			}
		}
		stored, err := rules.Find(ctx, rule.ID)
		if err != nil || stored.Version != rule.Version {
			t.Errorf("a rewrap moved the version to %d (%v)", stored.Version, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Gate SG-3: Census.CountByKey - the census is the transaction's tenant's and nobody else's.
func TestTheCensusCountsOneTenantsValuesOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, cryptoport.Sealed{KeyID: "census-key", Ciphertext: []byte("x")})
	census := postgres.NewSealingRepository()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		counts, err := census.CountByKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if counts["census-key"] != 0 {
			t.Errorf("another tenant's values were counted: %v", counts)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		counts, err := census.CountByKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if counts["census-key"] < 1 {
			t.Errorf("the tenant's own value was not counted: %v", counts)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
