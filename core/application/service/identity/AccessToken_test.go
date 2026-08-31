// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The scopes this build would declare. Two of them, because the point of the check is that a
// third one is refused - not that the real catalogue happens to contain any particular name.
var buildScopes = []string{"accounts:read", "accounts:write", "items:read"}

var (
	serviceAccountID = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	tokenRowID       = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	strangerID       = shared.ID("01936f2a-7c1e-7000-8000-0000000000b3")
)

// holder is the ordinary caller: a person acting for themselves, through a credential that
// carries the account scopes.
func holder() appshared.ActorContext {
	actor := admin()
	actor.Scopes = []string{accountRead, accountsRead}
	return actor
}

func tokenWriter(t *testing.T, accounts *accountStore, auth *authorizer, sink *auditSink) AccessTokenWriter {
	t.Helper()
	return AccessTokenWriter{
		Tokens: &tokens{}, Accounts: accounts, Authorizer: auth, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: tokenRowID},
		Entropy: clock.FixedEntropy{Seed: 7}, KnownScopes: buildScopes,
	}
}

func validCommand() CreateAccessTokenCommand {
	return CreateAccessTokenCommand{
		Name:      "the nightly export",
		Scopes:    []string{"items:read"},
		ExpiresAt: now.Add(30 * 24 * time.Hour),
	}
}

// The whole point of the task: a credential comes into being through the API, is answered once,
// and what is kept is a row that cannot produce it again.
func TestAMintAnswersTheCredentialOnceAndStoresARowWithout(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	minted, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), holder(), validCommand())
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}

	presented := minted.Secret.Reveal()
	if !strings.HasPrefix(presented, domain.TokenPrefix) {
		t.Errorf("the credential does not carry the scanning prefix: %q", presented)
	}
	// The tenant travels inside the credential, because the lookup needs one before it can
	// happen (multi-tenancy.md §3).
	parsed, err := domain.ParseToken(presented)
	if err != nil {
		t.Fatalf("the minted credential does not parse: %v", err)
	}
	if parsed.TenantID() != tenant {
		t.Errorf("the credential names tenant %s, want %s", parsed.TenantID(), tenant)
	}

	store := writer.Tokens.(*tokens)
	if len(store.minted) != 1 {
		t.Fatalf("stored %d rows, want one", len(store.minted))
	}
	row := store.minted[0]
	if row.AccountID != adminID || row.Name != "the nightly export" {
		t.Errorf("the row is not the caller's: %+v", row)
	}
	// The stored half carries no field the plaintext could be recovered from. The adapter hashes
	// what it was handed; nothing inwards of it ever held a storable value.
	if strings.Contains(row.Name, presented) {
		t.Error("the credential reached the stored row")
	}
	if store.presented.Secret() != presented {
		t.Error("the adapter was handed a different credential from the one answered")
	}
}

// Rule 10, at the one place it is easiest to break: the audit trail records what the credential
// may do and for how long, and never the credential or its owner's free text.
func TestTheMintingEntryCarriesNoCredentialAndNoName(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	minted, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), holder(), validCommand())
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}
	if len(sink.entries) != 1 {
		t.Fatalf("wrote %d audit entries, want one", len(sink.entries))
	}

	entry := sink.entries[0]
	if entry.Action != TokenCreatedAction || entry.TargetType != tokenTarget {
		t.Errorf("entry = %s on %s", entry.Action, entry.TargetType)
	}
	if entry.TargetID != tokenRowID {
		t.Errorf("the entry names %s rather than the token", entry.TargetID)
	}

	// Changes is already masked per field, so this reads what would actually be written.
	if _, present := entry.Changes["name"]; present {
		t.Error("the owner's own words reached the audit trail")
	}
	if rendered := fmt.Sprintf("%v", entry.Changes); strings.Contains(rendered, minted.Secret.Reveal()) {
		t.Fatalf("the credential reached the audit trail: %s", rendered)
	}
	for _, field := range []string{"account_id", "scopes", "expires_at"} {
		if _, present := entry.Changes[field]; !present {
			t.Errorf("the entry does not record %s, which is what an auditor asks of a mint", field)
		}
	}
}

// A scope nothing checks is a bound nothing applies, so the request is refused rather than
// quietly stored - and the refusal names the field and the value, because a caller who cannot see
// which of five scopes was wrong cannot fix it.
func TestAScopeTheCatalogueDoesNotDeclareIsRefusedByName(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	command := validCommand()
	command.Scopes = []string{"items:read", "items:teleport"}

	_, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), holder(), command)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation error", err)
	}

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error = %v, want a domain error", err)
	}
	if len(domainErr.Fields) != 1 {
		t.Fatalf("fields = %v, want one", domainErr.Fields)
	}
	if got := domainErr.Fields[0]; got.Path != "/scopes/1" || got.Params["scope"] != "items:teleport" {
		t.Errorf("field = %+v, want the second scope named", got)
	}
	if len(writer.Tokens.(*tokens).minted) != 0 {
		t.Error("a token was stored despite the refusal")
	}
}

// The self-service half of the split: one's own credentials need the account scope and no
// permission, exactly as one's own preferences do.
func TestOnesOwnTokensNeedNoPermission(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	if _, err := (CreateAccessToken{Writer: writer}).Execute(t.Context(), holder(), validCommand()); err != nil {
		t.Fatalf("minting one's own failed: %v", err)
	}
	if _, err := (ListAccessTokens{Writer: writer}).Execute(t.Context(), holder(), adminID); err != nil {
		t.Fatalf("listing one's own failed: %v", err)
	}
	if len(auth.requests) != 0 {
		t.Errorf("the authoriser was asked %d times about the caller's own", len(auth.requests))
	}

	// And a credential without the scope is refused, whatever the role: the scope is the second
	// bound and both have to allow it (ADR-0005).
	unscoped := holder()
	unscoped.Scopes = nil
	_, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), unscoped, validCommand())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error = %v, want forbidden for a token without the scope", err)
	}
}

// The administered half: a service account is nothing but access, so whoever answers for access
// administers it - and the permission is asked before the account is read.
func TestAServiceAccountsTokensNeedMemberManagement(t *testing.T) {
	machine := domain.Account{
		ID: serviceAccountID, TenantID: tenant, Kind: domain.AccountServiceAccount,
		DisplayName: "the nightly export", Status: domain.AccountActive,
	}
	accounts, auth, sink := newAccounts(machine), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	command := validCommand()
	command.AccountID = serviceAccountID

	if _, err := (CreateAccessToken{Writer: writer}).Execute(t.Context(), holder(), command); err != nil {
		t.Fatalf("minting for a service account failed: %v", err)
	}
	if len(auth.requests) != 1 {
		t.Fatalf("the authoriser was asked %d times, want once", len(auth.requests))
	}
	if got := auth.requests[0].Permission; got != service.PermissionManageMembers {
		t.Errorf("permission = %s, want %s", got, service.PermissionManageMembers)
	}

	// And a refusal from the authoriser is the answer, rather than a second opinion here.
	auth.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")
	if _, err := (ListAccessTokens{Writer: writer}).Execute(t.Context(), holder(), serviceAccountID); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error = %v, want the authoriser's refusal", err)
	}
}

// Nobody administers a person's credentials, whatever their role. An administrator can disable
// the account or revoke its memberships - both of which stop it acting - and cannot hold or
// enumerate what it authenticates with.
func TestAnotherPersonsTokensAreRefusedEvenToAnAdministrator(t *testing.T) {
	stranger := domain.Account{
		ID: strangerID, TenantID: tenant, Kind: domain.AccountUser,
		DisplayName: "Bo", Status: domain.AccountActive,
	}
	accounts, auth, sink := newAccounts(stranger), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	_, err := ListAccessTokens{Writer: writer}.Execute(t.Context(), holder(), strangerID)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error = %v, want forbidden", err)
	}

	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "access.token_not_administrable" {
		t.Errorf("detail = %q", domainErr.DetailCode)
	}
	// The permission was asked first and granted; what refused is the kind of account. A test
	// that let the authoriser refuse would prove nothing about this rule.
	if len(auth.requests) != 1 {
		t.Errorf("the authoriser was asked %d times, want once", len(auth.requests))
	}
}

// Revoking twice is somebody making sure, not a second event - and the trail must not claim
// otherwise.
func TestRevokingTwiceStampsOnceAndRecordsOnce(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	if _, err := (CreateAccessToken{Writer: writer}).Execute(t.Context(), holder(), validCommand()); err != nil {
		t.Fatalf("minting failed: %v", err)
	}
	revoke := RevokeAccessToken{Writer: writer}

	if err := revoke.Execute(t.Context(), holder(), tokenRowID); err != nil {
		t.Fatalf("the first revocation failed: %v", err)
	}
	if err := revoke.Execute(t.Context(), holder(), tokenRowID); err != nil {
		t.Fatalf("the second revocation was an error: %v", err)
	}

	// One minting entry and one revocation, and no third.
	if len(sink.entries) != 2 {
		t.Fatalf("wrote %d audit entries, want two", len(sink.entries))
	}
	if sink.entries[1].Action != TokenRevokedAction {
		t.Errorf("the second entry is %s", sink.entries[1].Action)
	}
	stored := writer.Tokens.(*tokens).minted[0]
	if stored.RevokedAt != now {
		t.Errorf("revoked at %v, want the first withdrawal at %v", stored.RevokedAt, now)
	}
}

// T-04: somebody else's is not found rather than forbidden. A 403 would confirm that the token
// exists, which is the one thing a stranger should not learn from asking.
func TestRevokingATokenThatIsNotThereIsNotFound(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	err := RevokeAccessToken{Writer: writer}.Execute(t.Context(), holder(), tokenRowID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error = %v, want not found", err)
	}
}

// The projection every channel reads. If the credential were in it, "shown once" would be a
// promise in prose rather than a property of the code.
func TestTheProjectionCarriesNoCredential(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	minted, err := CreateAccessToken{Writer: writer}.Execute(t.Context(), holder(), validCommand())
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}

	out := tokenOutput(minted.Token)
	if _, present := out["token"]; present {
		t.Fatal("the projection carries the credential")
	}
	for field, value := range out {
		if text, isString := value.(string); isString && strings.Contains(text, minted.Secret.Reveal()) {
			t.Fatalf("the credential is in the projection, under %s", field)
		}
	}
	if out["revoked_at"] != nil || out["last_used_at"] != nil {
		t.Error("a fresh token reports a revocation or a use")
	}
}

// The catalogue path, which is the one every channel actually uses: REST, MCP and a rule all
// reach a use case through its descriptor, so a handler that works only when called directly
// works nowhere.
func TestTheThreeReachTheirWorkThroughTheirDescriptors(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	create := CreateAccessToken{Writer: writer}.Descriptor()
	list := ListAccessTokens{Writer: writer}.Descriptor()
	revoke := RevokeAccessToken{Writer: writer}.Descriptor()

	// Every declared input has to be accepted by the registry's own validation, or the use case
	// is unreachable from any channel (the descriptor refuses a key it does not declare).
	minting := usecase.Input{
		"name":       "the nightly export",
		"scopes":     []any{"items:read"},
		"expires_at": now.Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}
	if err := create.ValidateInput(minting); err != nil {
		t.Fatalf("the mint's own input is refused by its declaration: %v", err)
	}

	out, err := create.Handler.Invoke(t.Context(), holder(), minting)
	if err != nil {
		t.Fatalf("minting through the descriptor failed: %v", err)
	}
	if out.String("token") == "" {
		t.Error("the mint answered no credential")
	}

	listed, err := list.Handler.Invoke(t.Context(), holder(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing through the descriptor failed: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("listed %d tokens, want one", len(rows))
	}
	if _, present := rows[0]["token"]; present {
		t.Error("a listed token carries the credential")
	}

	if _, err := revoke.Handler.Invoke(t.Context(), holder(), usecase.Input{
		"token_id": tokenRowID.String(),
	}); err != nil {
		t.Fatalf("revoking through the descriptor failed: %v", err)
	}

	// The three declarations SG-13 and the parity gate read.
	if !create.Audit.Required || !revoke.Audit.Required {
		t.Error("a write does not declare its audit obligation")
	}
	if !list.ReadOnly || !revoke.Destructive {
		t.Error("the MCP annotations do not match what the use cases do")
	}
}

// The expiry arrives as text on every channel, so an unreadable one is a field error rather than
// a zero time - "no expiry was given" and "what you gave is not a date" are different refusals.
func TestAnUnreadableExpiryIsItsOwnRefusal(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	_, err := CreateAccessToken{Writer: writer}.Descriptor().Handler.Invoke(
		t.Context(), holder(), usecase.Input{
			"name":       "the nightly export",
			"scopes":     []any{"items:read"},
			"expires_at": "next Tuesday",
		})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation error", err)
	}

	var domainErr *shared.Error
	if errors.As(err, &domainErr) && domainErr.DetailCode != "access.token_expiry_malformed" {
		t.Errorf("detail = %q", domainErr.DetailCode)
	}

	// And an omitted one reaches the domain as "none given", which is the refusal that says
	// there is no default.
	_, err = CreateAccessToken{Writer: writer}.Descriptor().Handler.Invoke(
		t.Context(), holder(), usecase.Input{"name": "x", "scopes": []any{"items:read"}})
	if errors.As(err, &domainErr) && domainErr.DetailCode != "access.token_expiry_required" {
		t.Errorf("detail = %q, want the mandatory-expiry refusal", domainErr.DetailCode)
	}
}

// A token that names an account which is not there at all: not found rather than forbidden, for
// the reason every other read of something absent is (T-04).
func TestAnAbsentServiceAccountIsNotFound(t *testing.T) {
	accounts, auth, sink := newAccounts(), &authorizer{}, &auditSink{}
	writer := tokenWriter(t, accounts, auth, sink)

	_, err := ListAccessTokens{Writer: writer}.Execute(t.Context(), holder(), serviceAccountID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error = %v, want not found", err)
	}
}
