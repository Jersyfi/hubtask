// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	automationrepo "github.com/Jersyfi/hubtask/core/application/repository/automation"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	integrationrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	"github.com/Jersyfi/hubtask/core/domain/event"
	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The re-seal's five views of the stores and the census over them (ADR-0045), against the
// real boundary. Gate SG-3: one negative per port method - a re-seal reaches every table that
// holds a credential to something outside this system, and it must reach only its own tenant's.

var (
	_ identityrepo.MfaSealings             = postgres.MfaRepository{}
	_ identityrepo.IdentityProviderSealing = postgres.IdentityProviderRepository{}
	_ integrationrepo.WebhookSealings      = postgres.WebhookSubscriptionRepository{}
	_ backuprepo.CredentialSealings        = postgres.BackupTargetRepository{}
	_ automationrepo.RuleSealings          = postgres.AutomationRuleRepository{}
	_ automationrepo.CallSealings          = postgres.CallSealingRepository{}
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

// Gate SG-3: IdentityProviderSealing.ListSealed and RewrapSecret.
func TestAProviderSecretIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()
	row := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe51")

	configured, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		ID: row, TenantID: idpTenantA, Issuer: "https://login.reseal.example", ClientID: "hubtask-a",
		AllowedEmailDomains: []string{"a.example"}, Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the configuration: %v", err)
	}
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		_, err := providers.Insert(ctx, configured, sealedSecret())
		return err
	})

	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		// The census does not name it either, which is what keeps a rotation next door from ever
		// reaching for it.
		listed, err := providers.ListSealed(ctx)
		if err != nil {
			t.Fatalf("B's census: %v", err)
		}
		for _, entry := range listed {
			if entry.ProviderID == row {
				t.Error("another tenant's provider secret is in this tenant's census")
			}
		}
		if moved, err := providers.RewrapSecret(ctx, row, movedSecret("k2"), "k1"); err != nil || moved {
			t.Errorf("another tenant's provider secret was rewrapped from here (%v, %v)", moved, err)
		}
		return nil
	})

	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if moved, err := providers.RewrapSecret(ctx, row, movedSecret("k2"), "k1"); err != nil || !moved {
			t.Fatalf("rewrapping (%v, %v)", moved, err)
		}
		_, sealed, err := providers.FindWithSecret(ctx, row)
		if err != nil || sealed.KeyID != "k2" {
			t.Errorf("the provider still names %q after the rewrap (%v)", sealed.KeyID, err)
		}
		// The guard is the key the row named when it was read: a second pass with the stale
		// expectation moves nothing.
		if moved, err := providers.RewrapSecret(ctx, row, movedSecret("k3"), "k1"); err != nil || moved {
			t.Errorf("a rewrap guarded on a stale key moved the row (%v, %v)", moved, err)
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

// Gate SG-3: Census.Count - the census is the transaction's tenant's and nobody else's.
func TestTheCensusCountsOneTenantsValuesOnly(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := targetIn(t, tenantA, authorA, freshName(t))
	insertTarget(ctx, t, tenantA, target, cryptoport.Sealed{KeyID: "census-key", Ciphertext: []byte("x")})
	census := postgres.NewSealingRepository()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		counts, err := byKey(census.Count(ctx))
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
		counts, err := byKey(census.Count(ctx))
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

// An installation provider's secret belongs to no workspace, and the read policy shows it to every
// one of them. A workspace's census that counted it would count it once per workspace - an
// installation of a hundred workspaces would report a hundred values under a key that seals one.
func TestAWorkspacesCensusLeavesTheInstallationsProviderOut(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	census := postgres.NewSealingRepository()
	now := time.Now().UTC()
	row := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe5c")
	const keyID = "census-installation-only"

	offered, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		ID: row, Issuer: "https://id.census.example", ClientID: "hubtask-census",
		Provisioning: string(identity.ProvisionInvitedOnly), Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the installation's provider: %v", err)
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := providers.Insert(ctx, offered, cryptoport.Sealed{KeyID: keyID, Ciphertext: []byte("x")})
		return err
	}); err != nil {
		t.Fatalf("writing the installation's provider: %v", err)
	}
	t.Cleanup(func() {
		_ = uow.Within(context.Background(), persistence.SystemScope(), func(ctx context.Context) error {
			_, err := providers.Delete(ctx, row, now)
			return err
		})
	})

	for _, tenant := range []shared.ID{idpTenantA, idpTenantB} {
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			counts, err := byKey(census.Count(ctx))
			if err != nil {
				t.Fatalf("%s's census: %v", tenant, err)
			}
			if counts[keyID] != 0 {
				t.Errorf("%s counted the installation's provider secret as its own: %d", tenant, counts[keyID])
			}
			return nil
		})
	}
}

// byKey is a census's totals per key, for the tests that ask only about one key.
func byKey(counted sealingrepo.Counts, err error) (map[string]int64, error) {
	return counted.ByKey(), err
}

// seedCallJob writes an HTTP call's job the way HttpRequest enqueues one, in the given state, with
// a header secret sealed under keyID - or none when keyID is empty - and removes it when the test
// ends: the job table is shared by every test in the package.
func seedCallJob(ctx context.Context, t *testing.T, tenant shared.ID, kind, state, keyID string) (job, request shared.ID) {
	t.Helper()
	request = freshID(t)
	var sealed map[string]any
	if keyID != "" {
		sealed = automation.SealedSecret{
			Ciphertext: base64.StdEncoding.EncodeToString([]byte("sealed-not-a-secret")),
			KeyID:      keyID,
			Purpose:    "automation.http:" + request.String(),
		}.Document()
	}
	return seedCallJobCarrying(ctx, t, tenant, kind, state, request, sealed), request
}

// seedCallJobCarrying is seedCallJob with the sealed document given, for a test that seals a real
// value; nil carries none.
func seedCallJobCarrying(
	ctx context.Context, t *testing.T, tenant shared.ID, kind, state string, request shared.ID, sealed map[string]any,
) shared.ID {
	t.Helper()
	job := freshID(t)
	payload := map[string]any{
		"request_id": request.String(), "method": "POST", "url": "https://hooks.example/call",
	}
	if sealed != nil {
		payload["secret_header_name"] = "X-Token"
		payload["secret_header_sealed"] = sealed
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding the payload: %v", err)
	}
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`INSERT INTO job (id, tenant_id, kind, payload, state) VALUES ($1, $2, $3, $4, $5)`,
		job.String(), tenant.String(), kind, encoded, state); err != nil {
		t.Fatalf("seeding the job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `DELETE FROM job WHERE id = $1`, job.String())
	})
	return job
}

// UC-INS-16 check 1: an HTTP call still on its retry ladder holds its header secret in its job,
// and the census counts it - only while the job may still run, and only in its own workspace. A
// key retired under such a call would make its next attempt fail.
func TestTheCensusCountsAPendingCallsHeaderSecret(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	const keyID = "census-call-key"
	for _, state := range []string{"PENDING", "RUNNING", "SUCCEEDED", "DEAD_LETTER", "CANCELLED"} {
		seedCallJob(ctx, t, tenantA, "automation.http", state, keyID)
	}
	seedCallJob(ctx, t, tenantA, "automation.http", "PENDING", "")
	seedCallJob(ctx, t, tenantA, "webhook.deliver", "PENDING", keyID)
	census := postgres.NewSealingRepository()

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		counts, err := census.Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n := counts[keyID][sealingrepo.StoreAutomationHTTPJob]; n != 0 {
			t.Errorf("another tenant's pending calls were counted: %d", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		counts, err := census.Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// The pending and the running call; not the finished ones, not the one without a secret,
		// not a job of another kind.
		if got := counts[keyID]; got[sealingrepo.StoreAutomationHTTPJob] != 2 || len(got) != 1 {
			t.Errorf("the census under %s is %v, want 2 pending call secrets and nothing else", keyID, got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func callListed(listed []automationrepo.PendingCall, job shared.ID) (automationrepo.PendingCall, bool) {
	for _, call := range listed {
		if call.JobID == job {
			return call, true
		}
	}
	return automationrepo.PendingCall{}, false
}

// callSecretKey reads the key a job's header secret names, and the request identity beside it, as
// stored - the field the worker opens it from.
func callSecretKey(ctx context.Context, t *testing.T, job shared.ID) (keyID, requestID string) {
	t.Helper()
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT payload #>> '{secret_header_sealed,key_id}', payload->>'request_id' FROM job WHERE id = $1`,
		job.String()).Scan(&keyID, &requestID); err != nil {
		t.Fatalf("reading the job back: %v", err)
	}
	return keyID, requestID
}

// Gate SG-3: CallSealings.SealedNotUnder, CallSealings.RewrapCall - the job table has no row level
// security, so the statements' own tenant condition is the only boundary.
func TestAPendingCallIsReSealedOnlyFromItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	pending, request := seedCallJob(ctx, t, tenantA, "automation.http", "PENDING", "k1")
	finished, _ := seedCallJob(ctx, t, tenantA, "automation.http", "SUCCEEDED", "k1")
	calls := postgres.NewCallSealingRepository()
	moved := automation.SealedSecret{
		Ciphertext: base64.StdEncoding.EncodeToString([]byte("rewrapped-not-a-secret")),
		KeyID:      "k2", Purpose: "automation.http:" + request.String(),
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		listed, err := calls.SealedNotUnder(ctx, "k2")
		if err != nil {
			t.Fatal(err)
		}
		if _, found := callListed(listed, pending); found {
			t.Error("another tenant's pending call was listed for re-sealing")
		}
		if ok, err := calls.RewrapCall(ctx, pending, moved, "k1"); err != nil || ok {
			t.Errorf("another tenant's pending call was rewrapped from here (%v, %v)", ok, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		listed, err := calls.SealedNotUnder(ctx, "k2")
		if err != nil {
			t.Fatal(err)
		}
		call, found := callListed(listed, pending)
		if !found || call.Sealed.KeyID != "k1" || call.RequestID != request.String() ||
			call.Sealed.Purpose != "automation.http:"+request.String() {
			t.Fatalf("the pending call to re-seal was not listed as stored: %+v (%v)", call, found)
		}
		if _, found := callListed(listed, finished); found {
			t.Error("a finished call was listed for re-sealing")
		}
		if ok, err := calls.RewrapCall(ctx, finished, moved, "k1"); err != nil || ok {
			t.Errorf("a finished call was rewrapped (%v, %v)", ok, err)
		}
		// Guarded by the key the job named: a stale expectation moves nothing.
		if ok, err := calls.RewrapCall(ctx, pending, moved, "k0"); err != nil || ok {
			t.Errorf("a stale rewrap changed the job (%v, %v)", ok, err)
		}
		if ok, err := calls.RewrapCall(ctx, pending, moved, "k1"); err != nil || !ok {
			t.Fatalf("rewrapping (%v, %v)", ok, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if keyID, requestID := callSecretKey(ctx, t, pending); keyID != "k2" || requestID != request.String() {
		t.Errorf("after the rewrap the job names %q for request %q", keyID, requestID)
	}
	if keyID, _ := callSecretKey(ctx, t, finished); keyID != "k1" {
		t.Errorf("the finished job was touched: %q", keyID)
	}
}
