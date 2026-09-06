// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	integrationrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	"github.com/Jersyfi/hubtask/core/application/service/sealing"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The rotation drill of ADR-0045, run against the real boundary rather than against a fake.
//
// A rotation nobody has run is a hypothesis - A-20's logic, applied to keys - and the part worth
// proving is not that AES works but that the *stored* shape survives one: the key identifier is a
// column beside the ciphertext, so a rotation is only real if a value written under the old key
// comes back through the repository and opens under a ring whose current key is a different one.
//
// It ends on the refusal, deliberately. Removing a key from the ring while a row still names it is
// what an operator would do next if nothing stopped them, and this is where the system says what
// happens: a refusal an operator can read, not a silent loss - and the reason the procedure in
// security.md §8.1 ends by counting rather than by waiting.

const (
	drillKeyOne = "rotation-drill-key-one-not-a-real-secret"
	drillKeyTwo = "rotation-drill-key-two-not-a-real-secret"
)

// ring builds a keyring the way the environment would, current first.
func ring(t *testing.T, entries ...crypto.KeyMaterial) crypto.Envelope {
	t.Helper()
	keyring, err := crypto.NewKeyring(entries)
	if err != nil {
		t.Fatalf("building the keyring: %v", err)
	}
	return crypto.NewEnvelope(keyring, clockadapter.CryptoRandom{})
}

func keyOne() crypto.KeyMaterial {
	return crypto.KeyMaterial{ID: "k1", Material: secret.New(drillKeyOne)}
}

func keyTwo() crypto.KeyMaterial {
	return crypto.KeyMaterial{ID: "k2", Material: secret.New(drillKeyTwo)}
}

func TestARotationRollsForwardAndLeavesTheOldValuesReadable(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	purpose := cryptoport.Purpose("account_mfa.secret:" + sessionAccountA.String())
	plaintext := "the-second-factor-of-an-account"

	// Before the rotation: one value sealed under the only key the installation has.
	before := ring(t, keyOne())
	sealed, err := before.Seal(ctx, secret.New(plaintext), purpose)
	if err != nil {
		t.Fatalf("sealing under k1: %v", err)
	}
	if sealed.KeyID != "k1" {
		t.Fatalf("sealed under %q, want k1", sealed.KeyID)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, sealed, now); err != nil {
			t.Fatalf("storing the enrolment: %v", err)
		}
		return nil
	})

	// The rotation itself: the new key goes in front, every predecessor stays in the ring.
	after := ring(t, keyTwo(), keyOne())
	if after.ActiveKeyID() != "k2" {
		t.Fatalf("the active key is %q, want k2", after.ActiveKeyID())
	}

	// What was written before the rotation still comes back out of the database, and still opens.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		stored, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading the enrolment back: %v", err)
		}
		if stored.Secret.KeyID != "k1" {
			t.Fatalf("the stored value names %q, want the key it was sealed under", stored.Secret.KeyID)
		}
		opened, err := after.Open(ctx, stored.Secret, purpose)
		if err != nil {
			t.Fatalf("opening a k1 value under the rotated ring: %v", err)
		}
		if opened.Reveal() != plaintext {
			t.Fatal("the value that came back is not the value that went in")
		}
		return nil
	})

	// And what is written from now on names the new key, without anything having been rewritten.
	next, err := after.Seal(ctx, secret.New(plaintext), purpose)
	if err != nil {
		t.Fatalf("sealing under the rotated ring: %v", err)
	}
	if next.KeyID != "k2" {
		t.Fatalf("a value sealed after the rotation names %q, want k2", next.KeyID)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, next, now); err != nil {
			t.Fatalf("storing the re-sealed enrolment: %v", err)
		}
		stored, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading it back: %v", err)
		}
		if stored.Secret.KeyID != "k2" {
			t.Fatalf("the stored value names %q after the rewrite, want k2", stored.Secret.KeyID)
		}
		return nil
	})
}

func TestRetiringAKeyARowStillNamesIsARefusalRatherThanALoss(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	mfa, uow := mfaStores(ctx, t)
	now := time.Now().UTC()

	purpose := cryptoport.Purpose("account_mfa.secret:" + sessionAccountA.String())

	sealed, err := ring(t, keyOne()).Seal(ctx, secret.New("still-under-the-old-key"), purpose)
	if err != nil {
		t.Fatalf("sealing under k1: %v", err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, sealed, now); err != nil {
			t.Fatalf("storing the enrolment: %v", err)
		}
		return nil
	})

	// The operator removes k1 from the ring while a row still names it - the step the procedure
	// puts a count in front of.
	retired := ring(t, keyTwo())

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		stored, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatalf("reading the enrolment back: %v", err)
		}
		_, err = retired.Open(ctx, stored.Secret, purpose)
		if err == nil {
			t.Fatal("a value sealed under a retired key opened anyway")
		}
		if !errors.Is(err, shared.ErrUnavailable) {
			t.Fatalf("opening under a retired key answered %v, want an unavailability", err)
		}
		return nil
	})
}

// The completion the first two drills could not reach (#368): values sealed under k1 in all five
// stores, one round under [k2 k1], the census answering zero for k1, and every value opening under
// a ring that holds k2 alone. This is the moment step 4 of security.md §8.1 waits for.

type drillTrail struct{ entries []audit.Entry }

func (d *drillTrail) Append(_ context.Context, entry audit.Entry) error {
	d.entries = append(d.entries, entry)
	return nil
}

// sealedHTTPRuleUnder is a rule whose HTTP_REQUEST action carries a header secret the envelope
// really sealed, under the purpose sealActions binds it to.
func sealedHTTPRuleUnder(
	ctx context.Context, t *testing.T, sealer crypto.Envelope, tenant, runAs, author shared.ID,
) automation.Rule {
	t.Helper()
	id := freshID(t)
	purpose := cryptoport.Purpose("automation.rule.http:" + id.String())
	sealed, err := sealer.Seal(ctx, secret.New("hook-token"), purpose)
	if err != nil {
		t.Fatalf("sealing the header secret: %v", err)
	}
	rule, err := automation.NewRule(automation.NewRuleInput{
		ID: id, TenantID: tenant, Name: freshName(t),
		Scope: automation.Scope{Type: automation.ScopeTenant}, RunAs: runAs,
		Trigger: automation.Trigger{Kind: automation.TriggerEvent, EventType: event.ItemOverdue},
		Actions: []automation.Action{{
			Kind: automation.ActionHTTPRequest,
			Params: map[string]any{
				"method": "POST", "url": "https://hooks.example/rotation",
				"secret_header_name": "X-Token",
				"secret_header_sealed": automation.SealedSecret{
					Ciphertext: base64.StdEncoding.EncodeToString(sealed.Ciphertext),
					KeyID:      sealed.KeyID, Purpose: string(purpose),
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

func TestARotationCompletesWhenTheCensusReachesZero(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	seedContainerTenants(ctx, t)
	runAs := seedServiceAccount(ctx, t, tenantA)
	mfa, uow := mfaStores(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	subscriptions := postgres.NewWebhookSubscriptionRepository()
	targets := targetRepo()
	rules := automationRules()
	census := postgres.NewSealingRepository()
	now := time.Now().UTC()

	// Keys of this drill's own: the database is shared with every other test in the package, and
	// their fixtures write fake ciphertexts under names like k1. A round finds those, and under a
	// key the ring holds a fake ciphertext is corruption rather than a foreign key - so the drill
	// rings keys nothing else names, and everything else is counted as skipped, which is exactly
	// what a value under a key the ring does not hold should be.
	suffix := freshID(t).String()
	suffix = suffix[len(suffix)-8:]
	oldKey := crypto.KeyMaterial{ID: "rot_a_" + suffix, Material: secret.New(drillKeyOne)}
	newKey := crypto.KeyMaterial{ID: "rot_b_" + suffix, Material: secret.New(drillKeyTwo)}

	// Before the rotation: one value in each of the five stores, every one sealed under the old
	// key with the purpose its owning service binds it to.
	before := ring(t, oldKey)
	seal := func(value string, purpose cryptoport.Purpose) cryptoport.Sealed {
		sealed, err := before.Seal(ctx, secret.New(value), purpose)
		if err != nil {
			t.Fatalf("sealing under k1: %v", err)
		}
		return sealed
	}
	mfaPurpose := cryptoport.Purpose("account_mfa.secret:" + sessionAccountA.String())
	idpPurpose := cryptoport.Purpose("identity_provider.client_secret:" + tenantA.String())
	provider, err := identity.NewIdentityProvider(identity.NewIdentityProviderInput{
		TenantID: tenantA, Issuer: "https://login.rotation.example", ClientID: "hubtask",
		AllowedEmailDomains: []string{"rotation.example"}, Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the provider: %v", err)
	}
	subscription := seedSubscription(ctx, t, tenantA)
	webhookPurpose := integrationservice.SecretPurpose(subscription.ID)
	target := targetIn(t, tenantA, authorA, freshName(t))
	targetPurpose := cryptoport.Purpose("backup_target.credential:" + target.ID.String())
	rule := sealedHTTPRuleUnder(ctx, t, before, tenantA, runAs, authorA)

	webhookSecret := seal("signing-secret", webhookPurpose)
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if _, err := mfa.Upsert(ctx, sessionAccountA, seal("totp-secret", mfaPurpose), now); err != nil {
			t.Fatalf("storing the enrolment: %v", err)
		}
		if _, err := providers.Upsert(ctx, provider, seal("client-secret", idpPurpose), now); err != nil {
			t.Fatalf("storing the provider: %v", err)
		}
		// The seeded subscription carries a fake secret; the drill needs one the envelope sealed.
		ok, err := subscriptions.Rewrap(ctx, subscription.ID,
			integrationrepo.SealedSecret{KeyID: webhookSecret.KeyID, Ciphertext: webhookSecret.Ciphertext},
			integrationrepo.SealedSecret{}, subscription.Version)
		if err != nil || !ok {
			t.Fatalf("giving the subscription a real secret (%v, %v)", ok, err)
		}
		if err := targets.Insert(ctx, target, seal("s3-secret-key", targetPurpose)); err != nil {
			t.Fatalf("storing the target: %v", err)
		}
		if err := rules.Insert(ctx, rule); err != nil {
			t.Fatalf("storing the rule: %v", err)
		}
		counts, err := census.CountByKey(ctx)
		if err != nil || counts[oldKey.ID] != 5 {
			t.Fatalf("the census before the rotation: %v (%v)", counts, err)
		}
		return nil
	})

	// The rotation, and the round that finishes it: the new key in front, the old one kept, and
	// every store's own resealer run inside the tenant's transaction the way the queue would.
	after := ring(t, newKey, oldKey)
	trail := &drillTrail{}
	round := sealing.RunReseal{
		Resealers: []sealing.Resealer{
			identityservice.MfaResealer{Enrollments: mfa, Encryptor: after},
			identityservice.IdentityProviderResealer{Providers: providers, Encryptor: after},
			integrationservice.WebhookResealer{Subscriptions: subscriptions, Encryptor: after},
			backupservice.TargetResealer{Targets: targets, Encryptor: after},
			automationservice.RuleResealer{Rules: rules, Encryptor: after},
		},
		Encryptor: after, Audit: trail, Clock: clockadapter.System{},
	}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantA}
	var outcome sealing.Outcome
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		moved, err := round.Execute(ctx, system)
		if err != nil {
			t.Fatalf("the round: %v", err)
		}
		outcome = moved
		return nil
	})
	// Exactly this drill's five moved; whatever other tests left under keys this ring does not
	// hold was skipped and counted rather than failed on.
	if outcome.Rewrapped != 5 {
		t.Fatalf("the round moved %d and skipped %d, want exactly 5 moved", outcome.Rewrapped, outcome.Skipped)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != sealing.ResealedAction {
		t.Errorf("the round left %+v in the trail", trail.entries)
	}

	// The census is what step 4 reads: the old key names nothing, and the round is idempotent.
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		counts, err := census.CountByKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if counts[oldKey.ID] != 0 || counts[newKey.ID] != 5 {
			t.Fatalf("after the round the census answers %v", counts)
		}
		again, err := round.Execute(ctx, system)
		if err != nil || again.Rewrapped != 0 {
			t.Errorf("a second round moved %d (%v)", again.Rewrapped, err)
		}
		return nil
	})

	// And the retirement: a ring holding the new key alone opens every one of the five, intact.
	retired := ring(t, newKey)
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		enrollment, err := mfa.Find(ctx, sessionAccountA)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := retired.Open(ctx, enrollment.Secret, mfaPurpose)
		if err != nil || opened.Reveal() != "totp-secret" {
			t.Errorf("the second factor after retirement: %v", err)
		}
		_, sealed, err := providers.FindWithSecret(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if opened, err := retired.Open(ctx, sealed, idpPurpose); err != nil || opened.Reveal() != "client-secret" {
			t.Errorf("the client secret after retirement: %v", err)
		}
		stored, err := subscriptions.Find(ctx, subscription.ID)
		if err != nil {
			t.Fatal(err)
		}
		if opened, err := retired.Open(ctx, cryptoport.Sealed{KeyID: stored.Secret.KeyID, Ciphertext: stored.Secret.Ciphertext}, webhookPurpose); err != nil || opened.Reveal() != "signing-secret" {
			t.Errorf("the signing secret after retirement: %v", err)
		}
		credential, err := targets.Credential(ctx, target.ID)
		if err != nil {
			t.Fatal(err)
		}
		if opened, err := retired.Open(ctx, credential, targetPurpose); err != nil || opened.Reveal() != "s3-secret-key" {
			t.Errorf("the target credential after retirement: %v", err)
		}
		found, err := rules.Find(ctx, rule.ID)
		if err != nil {
			t.Fatal(err)
		}
		request, err := automation.ReadHTTPRequest(found.Actions[0].Params, "/actions/0")
		if err != nil || request.Sealed == nil {
			t.Fatalf("reading the rule's action back: %v", err)
		}
		ciphertext, _ := base64.StdEncoding.DecodeString(request.Sealed.Ciphertext)
		if opened, err := retired.Open(ctx, cryptoport.Sealed{KeyID: request.Sealed.KeyID, Ciphertext: ciphertext},
			cryptoport.Purpose("automation.rule.http:"+rule.ID.String())); err != nil || opened.Reveal() != "hook-token" {
			t.Errorf("the header secret after retirement: %v", err)
		}
		if found.Version != rule.Version {
			t.Errorf("the round moved the rule's version to %d", found.Version)
		}
		return nil
	})
}
