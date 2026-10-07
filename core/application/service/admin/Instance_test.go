// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// instanceStore is the installation's level, in memory.
type instanceStore struct {
	level     identityrepo.InstanceLevel
	writes    []identityrepo.InstanceLevel
	writtenBy []shared.ID
}

func newInstanceStore() *instanceStore {
	return &instanceStore{level: identityrepo.InstanceLevel{
		Policy: identity.PolicyLayer{Locks: map[identity.PolicySwitch]bool{}},
		Legal:  identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
		Source: "database",
	}}
}

func (s *instanceStore) Read(context.Context) (identityrepo.InstanceLevel, error) {
	return s.level, nil
}

func (s *instanceStore) Write(
	_ context.Context, level identityrepo.InstanceLevel, by shared.ID, _ time.Time,
) error {
	s.writes = append(s.writes, level)
	s.writtenBy = append(s.writtenBy, by)
	kept := level
	kept.Source = s.level.Source
	s.level = kept
	return nil
}

// registerStore is the operator register, in memory. An empty one answers true, which is the
// private installation - and the test that asserts it is the one that matters most here.
type registerStore struct {
	accounts map[shared.ID]bool
	order    []shared.ID
	// addresses is what the register's narrow door answers: "slug\x00email" to an account.
	addresses map[string]shared.ID
}

func newRegister(held ...shared.ID) *registerStore {
	store := &registerStore{accounts: map[shared.ID]bool{}}
	for _, id := range held {
		store.accounts[id] = true
		store.order = append(store.order, id)
	}
	return store
}

func (s *registerStore) Resolve(_ context.Context, slug, email string) (shared.ID, error) {
	return s.addresses[slug+"\x00"+email], nil
}

func (s *registerStore) Holds(_ context.Context, accountID shared.ID) (bool, error) {
	if len(s.accounts) == 0 {
		return true, nil
	}
	return s.accounts[accountID], nil
}

func (s *registerStore) List(context.Context) ([]identityrepo.Operator, error) {
	register := make([]identityrepo.Operator, 0, len(s.order))
	for _, id := range s.order {
		if s.accounts[id] {
			register = append(register, identityrepo.Operator{
				TenantID: operatorHome, AccountID: id, AddedAt: fixed,
			})
		}
	}
	return register, nil
}

func (s *registerStore) Add(_ context.Context, accountID, _ shared.ID) (bool, error) {
	if accountID == unknownAccount {
		// What the statement answers for an account nobody holds: nothing was inserted.
		return false, nil
	}
	if s.accounts[accountID] {
		return false, nil
	}
	s.accounts[accountID] = true
	s.order = append(s.order, accountID)
	return true, nil
}

func (s *registerStore) Remove(_ context.Context, accountID shared.ID) (bool, error) {
	if len(s.accounts) <= 1 || !s.accounts[accountID] {
		return false, nil
	}
	delete(s.accounts, accountID)
	return true, nil
}

const (
	unknownAccount = shared.ID("01936f2a-7c1e-7000-8000-00000000dead")
	secondOperator = shared.ID("01936f2a-7c1e-7000-8000-00000000f00d")
)

var fixed = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func newInstanceWriter(register *registerStore) (InstanceWriter, *instanceStore, *journalStore) {
	settings := newInstanceStore()
	journal := &journalStore{}
	return InstanceWriter{
		Settings: settings, Operators: register, Journal: journal,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(fixed), IDs: &ids{},
	}, settings, journal
}

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.ID("01936f2a-7c1e-7000-8000-00000000000" + string(rune('0'+i.n)))
}

// The scope alone is not enough: the register is checked again where the scope is exercised,
// because a token minted last month by somebody since removed still carries it.
func TestTheInstanceOperationsDemandTheRegisterAsWellAsTheScope(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(secondOperator))

	if _, err := (ReadInstanceSettings{Writer: writer}).Execute(t.Context(), operator()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader outside the register answered %v", err)
	}
	if _, err := (ListOperators{Writer: writer}).Execute(t.Context(), operator()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a listing outside the register answered %v", err)
	}
	if err := (AddOperator{Writer: writer}).Execute(t.Context(), operator(), unknownAccount); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a write outside the register answered %v", err)
	}
	if _, err := (ReadInstanceSettings{Writer: writer}).Execute(t.Context(), anonymousActor()); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("a caller with no credential answered %v", err)
	}
}

// An empty register is the private installation: nothing configured, and the owner is the
// operator exactly as they were before it existed.
func TestAnEmptyRegisterIsThePrivateInstallation(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister())

	if _, err := (ReadInstanceSettings{Writer: writer}).Execute(t.Context(), operator()); err != nil {
		t.Fatalf("the owner of a private installation was refused: %v", err)
	}
}

// The level is replaced whole, the journal names what moved, and it names no value.
func TestWritingTheLevelReplacesItAndNamesWhatMoved(t *testing.T) {
	writer, settings, journal := newInstanceWriter(newRegister(operatorID))
	settings.level.Policy.Patch = identity.PolicyPatch{MinLength: intOf(12)}

	written, err := WriteInstanceSettings{Writer: writer}.Execute(t.Context(), operator(),
		identityrepo.InstanceLevel{
			Policy: identity.PolicyLayer{
				Patch: identity.PolicyPatch{MinLength: intOf(18), HistoryCount: intOf(4)},
				Locks: map[identity.PolicySwitch]bool{identity.SwitchMinLength: true},
			},
			Legal: identity.LegalLayer{
				Links: identity.LegalLinks{ImprintURL: "https://host.example/imprint"},
				Locks: map[identity.LegalLink]bool{identity.LinkImprint: true},
			},
			BlocklistFile: "/etc/hubtask/passwords.txt",
		})
	if err != nil {
		t.Fatalf("the write was refused: %v", err)
	}

	if written.Policy.Patch.MinLength == nil || *written.Policy.Patch.MinLength != 18 {
		t.Errorf("the level answered %v", written.Policy.Patch.MinLength)
	}
	if len(settings.writtenBy) != 1 || settings.writtenBy[0] != operatorID {
		t.Errorf("the write was recorded against %v", settings.writtenBy)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("%d journal entries", len(journal.entries))
	}
	moved, _ := journal.entries[0].Details["switches"].([]any)
	held := map[string]bool{}
	for _, name := range moved {
		held[name.(string)] = true
	}
	for _, name := range []string{"min_length", "history_count", "imprint_url", "blocklist_file"} {
		if !held[name] {
			t.Errorf("%q is not named in %v", name, moved)
		}
	}
	// And no value of any of them, because a journal is a place nothing ever deletes from.
	for _, name := range moved {
		if text, isString := name.(string); isString && (text == "18" || text == "/etc/hubtask/passwords.txt") {
			t.Errorf("the journal carries a value: %v", moved)
		}
	}
}

// Adding is idempotent; an account nobody holds is the one refusal.
func TestAddingAnOperatorIsIdempotentAndChecksTheAccount(t *testing.T) {
	writer, _, journal := newInstanceWriter(newRegister(operatorID))

	if err := (AddOperator{Writer: writer}).Execute(t.Context(), operator(), secondOperator); err != nil {
		t.Fatalf("adding was refused: %v", err)
	}
	if err := (AddOperator{Writer: writer}).Execute(t.Context(), operator(), secondOperator); err != nil {
		t.Errorf("adding somebody who is already an operator answered %v", err)
	}
	if len(journal.entries) != 1 {
		t.Errorf("%d journal entries, want one - the second add changed nothing", len(journal.entries))
	}
	if err := (AddOperator{Writer: writer}).Execute(t.Context(), operator(), unknownAccount); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an account nobody holds answered %v", err)
	}
	if err := (AddOperator{Writer: writer}).Execute(t.Context(), operator(), ""); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an empty identifier answered %v", err)
	}
}

// An operator is named the way a person can name one, because an identifier is not something they
// can look up: `account` is behind row level security, so no screen may list accounts across
// workspaces and none can offer one to pick.
func TestAnOperatorIsRegisteredByWorkspaceAndAddress(t *testing.T) {
	register := newRegister(operatorID)
	register.addresses = map[string]shared.ID{"acme\x00ada@acme.example": secondOperator}
	writer, _, journal := newInstanceWriter(register)

	if err := (AddOperator{Writer: writer}).
		ExecuteByAddress(t.Context(), operator(), "acme", "ada@acme.example"); err != nil {
		t.Fatalf("registering by address was refused: %v", err)
	}
	if len(journal.entries) != 1 {
		t.Errorf("%d journal entries, want one", len(journal.entries))
	}

	// A pair that matches nothing answers the same refusal a wrong identifier does - deliberately
	// the same, because whether an address exists in a workspace is what somebody probing wants to
	// learn, and they could already learn as much by trying the identifier form.
	err := (AddOperator{Writer: writer}).
		ExecuteByAddress(t.Context(), operator(), "acme", "nobody@acme.example")
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an address nobody holds answered %v", err)
	}
	if code := shared.AsError(err).DetailCode; code != "admin.operator_unknown_account" {
		t.Errorf("refused with %q, want the same code a wrong identifier gives", code)
	}

	// And neither half on its own names anybody.
	for _, missing := range [][2]string{{"", "ada@acme.example"}, {"acme", ""}} {
		if err := (AddOperator{Writer: writer}).
			ExecuteByAddress(t.Context(), operator(), missing[0], missing[1]); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("half a pair (%q, %q) answered %v", missing[0], missing[1], err)
		}
	}
}

// The last operator cannot be removed: an installation with none is one nobody can operate.
func TestTheLastOperatorCannotBeRemoved(t *testing.T) {
	writer, _, journal := newInstanceWriter(newRegister(operatorID, secondOperator))

	if err := (RemoveOperator{Writer: writer}).Execute(t.Context(), operator(), secondOperator); err != nil {
		t.Fatalf("removing the second was refused: %v", err)
	}
	if err := (RemoveOperator{Writer: writer}).Execute(t.Context(), operator(), operatorID); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("removing the last answered %v", err)
	}
	if len(journal.entries) != 1 {
		t.Errorf("%d journal entries, want one", len(journal.entries))
	}
}

// The listing is what a screen draws.
func TestTheRegisterIsListed(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID, secondOperator))

	register, err := ListOperators{Writer: writer}.Execute(t.Context(), operator())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(register) != 2 {
		t.Fatalf("%d operators", len(register))
	}
	if out := operatorOutput(register[0]); out.String("account_id") != operatorID.String() {
		t.Errorf("the projection reads %v", out)
	}
}

// The document a `PUT` carries is read into the level, and a value of the wrong kind is refused
// against its own field rather than silently dropped.
func TestTheLevelIsReadFromTheDocument(t *testing.T) {
	level, err := instanceLevelFrom(usecase.Input{
		"sign_in": map[string]any{
			"min_length":       map[string]any{"value": float64(18), "locked": true},
			"common_passwords": map[string]any{"value": true, "locked": false},
			"mfa_required_for": map[string]any{"value": "EVERYONE", "locked": false},
			"methods":          map[string]any{"value": []any{"PASSWORD"}, "locked": false},
			// A key this build does not know is skipped rather than refused, so a newer client
			// loses the switch it has and not the save.
			"what_i_had_for_lunch": map[string]any{"value": float64(1), "locked": false},
		},
		"legal": map[string]any{
			"imprint_url": map[string]any{"value": "https://host.example/imprint", "locked": true},
		},
		"blocklist_file": "/etc/hubtask/passwords.txt",
	})
	if err != nil {
		t.Fatalf("the document was refused: %v", err)
	}
	if level.Policy.Patch.MinLength == nil || *level.Policy.Patch.MinLength != 18 {
		t.Errorf("min_length read as %v", level.Policy.Patch.MinLength)
	}
	if !level.Policy.Locks[identity.SwitchMinLength] || level.Policy.Locks[identity.SwitchCommonPasswords] {
		t.Errorf("the locks read %v", level.Policy.Locks)
	}
	if level.Legal.Links.ImprintURL != "https://host.example/imprint" || !level.Legal.Locks[identity.LinkImprint] {
		t.Errorf("the links read %+v", level.Legal)
	}
	if level.BlocklistFile != "/etc/hubtask/passwords.txt" {
		t.Errorf("the file reads %q", level.BlocklistFile)
	}

	for _, wrong := range []usecase.Input{
		{"sign_in": map[string]any{"min_length": map[string]any{"value": "eighteen"}}},
		{"sign_in": map[string]any{"common_passwords": map[string]any{"value": float64(1)}}},
		{"sign_in": map[string]any{"mfa_required_for": map[string]any{"value": "SOMETIMES"}}},
		{"sign_in": map[string]any{"methods": map[string]any{"value": "PASSWORD"}}},
		{"sign_in": map[string]any{"min_length": "eighteen"}},
		{"legal": map[string]any{"imprint_url": map[string]any{"value": "javascript:alert(1)"}}},
	} {
		if _, err := instanceLevelFrom(wrong); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%v answered %v", wrong, err)
		}
	}
}

// The projection answers what was decided and nothing else: a level that answered the product's
// defaults would be one nobody could tell apart from an operator who had chosen them.
// The projection answers the whole catalogue, and says which of it was decided.
//
// It answered only the decided switches until SI-17's walk: four rows on a screen the concept gives
// eighteen switches, with no way for a reader to learn that the other fourteen exist or that this
// installation has left them to each workspace. Those are different facts, and `set` is what tells
// them apart — an undecided entry carries no `value` at all, because a zero is a decision.
func TestTheProjectionAnswersTheWholeCatalogueAndWhatWasDecided(t *testing.T) {
	out := instanceLevelOutput(identityrepo.InstanceLevel{
		Policy: identity.PolicyLayer{
			Patch: identity.PolicyPatch{MinLength: intOf(18)},
			Locks: map[identity.PolicySwitch]bool{identity.SwitchMinLength: true},
		},
		Legal:  identity.LegalLayer{Locks: map[identity.LegalLink]bool{}},
		Source: "database",
	})

	settings, _ := out["sign_in"].(usecase.Output)
	if len(settings) != len(identity.PolicySwitches()) {
		t.Fatalf("the projection answers %d switches, want every one of the %d",
			len(settings), len(identity.PolicySwitches()))
	}
	for _, name := range identity.PolicySwitches() {
		if _, held := settings[string(name)]; !held {
			t.Errorf("%s is missing from the projection", name)
		}
	}

	decided, _ := settings["min_length"].(usecase.Output)
	if decided["set"] != true || decided["value"] != 18 || decided["locked"] != true {
		t.Errorf("min_length reads %v", decided)
	}

	// And an undecided one is undecided rather than zero: a screen drawing `0` would be showing a
	// decision nobody made, and the resolver acts on the difference.
	undecided, _ := settings["min_digits"].(usecase.Output)
	if undecided["set"] != false {
		t.Errorf("min_digits reads %v, want it undecided", undecided)
	}
	if _, held := undecided["value"]; held {
		t.Errorf("an undecided switch carries a value: %v", undecided)
	}

	// The four legal links, for the same reason: a screen drawing only what somebody filled in
	// never mentions terms or an accessibility statement.
	legal, _ := out["legal"].(usecase.Output)
	if len(legal) != len(identity.LegalLinkNames()) {
		t.Errorf("the projection answers %d links, want %d", len(legal), len(identity.LegalLinkNames()))
	}

	if _, held := out["blocklist_file"]; held {
		t.Error("a file nobody configured was answered")
	}
	if out["source"] != "database" {
		t.Errorf("the source reads %v", out["source"])
	}
}

func intOf(value int) *int { return &value }

// Every one of the five reaches its use case through the catalogue, which is the only way a
// client ever calls it: a descriptor whose handler nothing exercised would be a route that
// compiles and answers nothing.
func TestTheInstanceUseCasesAnswerThroughTheRegistry(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID, secondOperator))

	registry, err := usecase.NewRegistry(nil,
		ReadInstanceSettings{Writer: writer}.Descriptor(),
		WriteInstanceSettings{Writer: writer}.Descriptor(),
		ListOperators{Writer: writer}.Descriptor(),
		AddOperator{Writer: writer}.Descriptor(),
		RemoveOperator{Writer: writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("the five are not registrable: %v", err)
	}

	read, err := registry.Invoke(t.Context(), ReadInstanceSettingsName, operator(), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if _, held := read["sign_in"]; !held {
		t.Errorf("the read answered %v", read)
	}

	if _, err := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), usecase.Input{
		"sign_in": map[string]any{"min_length": map[string]any{"value": float64(18), "locked": true}},
	}); err != nil {
		t.Fatalf("writing: %v", err)
	}

	listed, err := registry.Invoke(t.Context(), ListOperatorsName, operator(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if rows, _ := listed["data"].([]usecase.Output); len(rows) != 2 {
		t.Errorf("the listing answered %v", listed)
	}

	if _, err := registry.Invoke(t.Context(), AddOperatorName, operator(), usecase.Input{
		"account_id": string(operatorID),
	}); err != nil {
		t.Fatalf("adding: %v", err)
	}
	if _, err := registry.Invoke(t.Context(), RemoveOperatorName, operator(), usecase.Input{
		"account_id": string(secondOperator),
	}); err != nil {
		t.Fatalf("removing: %v", err)
	}

	// And an identifier that is not one is refused at the edge rather than in the store.
	if _, err := registry.Invoke(t.Context(), AddOperatorName, operator(), usecase.Input{
		"account_id": "not an identifier",
	}); err == nil {
		t.Error("a malformed identifier was accepted")
	}
}

// The `PUT` refuses a document it cannot read, through the registry as well as under it.
func TestTheWriteRefusesADocumentItCannotRead(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID))
	registry, err := usecase.NewRegistry(nil, WriteInstanceSettings{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	if _, err := registry.Invoke(t.Context(), WriteInstanceSettingsName, operator(), usecase.Input{
		"sign_in": map[string]any{"min_length": map[string]any{"value": "eighteen"}},
	}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a value of the wrong kind answered %v", err)
	}
}
