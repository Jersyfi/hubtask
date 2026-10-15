// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	automationdomain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-PRV-03 check 10, the restriction half (data-protection.md §4.1, Decision 7 of milestone PH):
// what a legal hold keeps from an erasure is restricted - out of AI and out of automation - and
// stays in the workspace export and the person's own Art. 15 copy. Under a hold on the workspace
// the account is kept RESTRICTED; under a hold on a hub it is anonymised and the kept rows stay
// attributed to it, restricted until the last kept part is erased.

const keptSays = "KEPTSAYS the order went out on Monday"

// keptColleague is a person who stays: an owner with an entry the erased person commented on.
type keptColleague struct {
	account, entry shared.ID
}

func seedColleagueOf(ctx context.Context, t *testing.T, s person) keptColleague {
	t.Helper()
	c := keptColleague{account: freshID(t), entry: freshID(t)}
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, $3, 'Olga Owner')`,
			[]any{c.account.String(), s.tenant.String(), "olga-" + c.account.String() + "@example.test"}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), s.tenant.String(), c.account.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the colleague: %v\n%s", err, statement.sql)
		}
	}
	if err := write(ctx, t, s.tenant, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(s.tenant, c.account, s.collection, c.entry, "OLGATASK supplier review", "a5"))
	}); err != nil {
		t.Fatalf("seeding the colleague's entry: %v", err)
	}
	for author, body := range map[shared.ID]string{c.account: "OLGASAYS thanks", s.account: keptSays} {
		if _, err := admin.Exec(ctx,
			`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, $5)`,
			freshID(t).String(), s.tenant.String(), c.entry.String(), author.String(), body); err != nil {
			t.Fatalf("seeding a comment: %v", err)
		}
	}
	return c
}

func (c keptColleague) actor(s person) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: s.tenant, AccountID: c.account, AccountName: "Olga",
		Locale: "en", TimeZone: "UTC",
		Scopes: []string{"items:read", "containers:read", "comments:read", "items:write", "suggestions:write"},
	}
}

// askAboutThread asks for a summary of the colleague's thread and answers what reached the adapter.
func askAboutThread(ctx context.Context, t *testing.T, h keptHarness, s person, c keptColleague) string {
	t.Helper()
	if _, err := h.produce.Ask(ctx, c.actor(s), suggestion.Request{
		TargetType: suggestiondomain.TargetWorkItem, TargetID: c.entry,
		Kind: suggestiondomain.KindFields, PromptID: "summarize-thread",
	}); err != nil {
		t.Logf("asking: %v", err)
	}
	return strings.Join(h.recorder.take(), "\n")
}

type oneEvent struct{ envelope event.Envelope }

func (o oneEvent) FindEvent(context.Context, shared.ID) (event.Envelope, error) {
	return o.envelope, nil
}

// ruleAbout runs a rule once on an event about the erased person's kept entry, and answers how it
// ended.
func ruleAbout(ctx context.Context, t *testing.T, s person) automationdomain.Run {
	t.Helper()
	rule := seedRule(ctx, t, s.tenant, nil)
	if err := write(ctx, t, s.tenant, func(ctx context.Context) error {
		return automationRules().SetEnabled(ctx, rule.ID, true, rule.Version, time.Now().UTC())
	}); err != nil {
		t.Fatalf("enabling: %v", err)
	}
	actions := &noActions{}
	envelope := event.Envelope{
		ID: freshID(t), Type: event.ItemOverdue, TenantID: s.tenant, Subject: "item/" + s.item.String(),
		OccurredAt: time.Now().UTC(),
	}
	engine := automationservice.RunRule{
		Rules: automationRules(), Runs: automationRuns(), Failures: automationRuns(),
		Events: postgres.NewOutbox(jobQueue(t)), Source: oneEvent{envelope},
		Dispatcher: actions, Scopes: actions, Entries: itemRepo(), Restrictions: privacyRepo(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: clockadapter.System{}, IDs: generator{t},
	}
	var run automationdomain.Run
	if err := write(ctx, t, s.tenant, func(ctx context.Context) error {
		var err error
		run, err = engine.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: s.tenant},
			automationservice.Command{RuleID: rule.ID, Trigger: automationdomain.TriggerEvent, EventID: envelope.ID})
		return err
	}); err != nil {
		t.Fatalf("running the rule: %v", err)
	}
	if actions.calls != 0 && run.Status == automationdomain.RunSkipped {
		t.Errorf("a withheld run performed %d actions", actions.calls)
	}
	return run
}

// keptScopes are the holds UC-PRV-03 check 10 names, each placed so that it covers the colleague's
// entry the erased person commented on. Under every hold but the workspace's the erasure is a full
// deletion, which would take the comment were the hold not there.
var keptScopes = []struct {
	name  string
	scope string
	at    func(person, keptColleague) shared.ID
	mode  domain.ErasureMode
}{
	{"a hold on the workspace", "TENANT", func(person, keptColleague) shared.ID { return "" }, domain.ModeAnonymize},
	{"a hold on the hub", "CONTAINER", func(s person, _ keptColleague) shared.ID { return s.hub }, domain.ModeFullDelete},
	{"a hold on the collection", "CONTAINER", func(s person, _ keptColleague) shared.ID { return s.collection }, domain.ModeFullDelete},
	{"a hold on the entry", "ITEM", func(_ person, c keptColleague) shared.ID { return c.entry }, domain.ModeFullDelete},
	{"a hold on the account", "ACCOUNT", func(s person, _ keptColleague) shared.ID { return s.account }, domain.ModeFullDelete},
}

const lostSays = "LOSTSAYS the old hub is closed"

// seedUncovered gives the person a comment in a hub of its own, which only a hold on the workspace
// or on the account covers.
func seedUncovered(ctx context.Context, t *testing.T, s person) shared.ID {
	t.Helper()
	hub, collection, entry, comment := freshID(t), freshID(t), freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by) VALUES ($1, $2, 'HUB', 'Old', 'a5', $3)`,
			[]any{hub.String(), s.tenant.String(), s.account.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Archive', 'a0', $4)`,
			[]any{collection.String(), s.tenant.String(), hub.String(), s.account.String()}},
		{`INSERT INTO work_item (id, tenant_id, collection_id, type, title, path, depth, order_key, created_by)
		  VALUES ($1, $2, $3, 'TASK', 'Close the old hub', $4, 1, 'a0', $5)`,
			[]any{entry.String(), s.tenant.String(), collection.String(), entry.String(), s.account.String()}},
		{`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, $5)`,
			[]any{comment.String(), s.tenant.String(), entry.String(), s.account.String(), lostSays}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding outside the hold: %v\n%s", err, statement.sql)
		}
	}
	return comment
}

func TestWhatAHoldKeepsStaysOutOfAiAndAutomation(t *testing.T) {
	for _, testCase := range keptScopes {
		t.Run(testCase.name, func(t *testing.T) {
			r := newRemainderRig(t)
			ctx, s := r.ctx, r.subject
			c := seedColleagueOf(ctx, t, s)
			uncovered := seedUncovered(ctx, t, s)
			h := newKeptHarness(ctx, t)
			if sent := askAboutThread(ctx, t, h, s, c); !strings.Contains(sent, "KEPTSAYS") {
				t.Fatalf("before the erasure the comment did not reach the adapter, so the test proves nothing:\n%s", sent)
			}

			hold := holdOn(ctx, t, s.tenant, testCase.scope, testCase.at(s, c))
			request := erasureRequest(ctx, t, s, testCase.mode)
			if _, err := performerFor(t).Perform(ctx, privacyservice.PerformInput{
				RequestID: request.ID, TenantID: s.tenant,
			}); err != nil {
				t.Fatalf("carrying out the case: %v", err)
			}
			if countIn(ctx, t, `SELECT count(*) FROM comment WHERE body = $1 AND tenant_id = $2`, keptSays, s.tenant.String()) != 1 {
				t.Fatal("the hold did not keep the comment")
			}

			// What no hold covers is erased as usual.
			if countIn(ctx, t, `SELECT count(*) FROM access_token WHERE id = $1`, s.token.String()) != 0 {
				t.Error("the token outlived the erasure")
			}
			widest := testCase.scope == "TENANT" || testCase.scope == "ACCOUNT"
			if left := countIn(ctx, t, `SELECT count(*) FROM comment WHERE id = $1`, uncovered.String()); (left == 1) != widest {
				t.Errorf("the comment outside the hold: %d left, want kept %v", left, widest)
			}

			// Out of AI.
			sent := askAboutThread(ctx, t, h, s, c)
			if strings.Contains(sent, "KEPTSAYS") || !strings.Contains(sent, "OLGASAYS") {
				t.Errorf("after the erasure the adapter received:\n%s", sent)
			}
			// Out of automation: a rule on an event about the person's entry does nothing.
			if run := ruleAbout(ctx, t, s); run.Status != automationdomain.RunSkipped ||
				run.ErrorCode != automationservice.CodeSubjectRestricted {
				t.Errorf("a rule about the person's entry ended %s / %q", run.Status, run.ErrorCode)
			}
			// No rule puts the person on an entry: assigning under a rule and the automatic draw ask
			// this port first. Asked directly, because the visibility check in front of it already
			// refuses a person whose memberships the erasure took.
			if err := read(ctx, t, s.tenant, func(ctx context.Context) error {
				restricted, err := postgres.NewAccountRepository().Restricted(ctx, []shared.ID{s.account})
				if err == nil && !restricted[s.account] {
					t.Error("a rule may still assign the person")
				}
				return err
			}); err != nil {
				t.Fatalf("reading: %v", err)
			}

			if testCase.scope != "CONTAINER" || testCase.at(s, c) != s.hub {
				return
			}
			// The hold is lifted and the rest erased: what anonymised stays is the workspace's again.
			// In a full deletion the comment goes with the rest; the former user is out of the set.
			r.release(hold)
			r.runJobs(request.ID)
			if err := read(ctx, t, s.tenant, func(ctx context.Context) error {
				kept, err := keptOutExclusion().Read(ctx, s.tenant)
				if err == nil && kept.Has(s.account) {
					t.Error("the former user is still kept out after the last kept part was erased")
				}
				return err
			}); err != nil {
				t.Fatalf("reading: %v", err)
			}
		})
	}
}

// Decision 7: the kept data stays in backups, the workspace export and the person's own Art. 15
// copy - a restriction allows storage, and no export is filtered.
func TestWhatAHoldKeepsStaysInTheExports(t *testing.T) {
	for _, testCase := range keptScopes {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			s := seedSubject(ctx, t, "exported-"+freshID(t).String()+"@example.test")
			c := seedColleagueOf(ctx, t, s)
			holdOn(ctx, t, s.tenant, testCase.scope, testCase.at(s, c))
			request := erasureRequest(ctx, t, s, testCase.mode)
			if _, err := performerFor(t).Perform(ctx, privacyservice.PerformInput{RequestID: request.ID, TenantID: s.tenant}); err != nil {
				t.Fatalf("carrying out the case: %v", err)
			}
			keptInTheExports(ctx, t, s)
		})
	}
}

func keptInTheExports(ctx context.Context, t *testing.T, s person) {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	written := func(write func(store backupstorage.LocalStore)) string {
		t.Helper()
		root := t.TempDir()
		store, err := backupstorage.NewLocalStore(root, "archives")
		if err != nil {
			t.Fatalf("opening the store: %v", err)
		}
		write(store)
		everything := &bytes.Buffer{}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			content, err := os.ReadFile(path)
			everything.Write(content)
			return err
		}); err != nil {
			t.Fatalf("walking the archive: %v", err)
		}
		return everything.String()
	}

	workspace := written(func(store backupstorage.LocalStore) {
		archivist := adminservice.TenantExportArchivist{
			Targets: exportTargetStore{store: store}, Rows: postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
			Objects: exportObjects{key: "none"}, Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
			Snapshot: unitOfWork, Clock: clockadapter.System{}, SchemaVersion: "test", ProductVersion: "test",
		}
		if _, err := archivist.Write(ctx, adminservice.ExportArchiveRequest{
			ExportID: freshID(t), TenantID: s.tenant, TargetID: freshID(t),
		}); err != nil {
			t.Fatalf("writing the workspace export: %v", err)
		}
	})
	if !strings.Contains(workspace, "KEPTSAYS") {
		t.Error("the workspace export left out what the hold keeps")
	}

	own := written(func(store backupstorage.LocalStore) {
		exporter := privacyservice.Exporter{
			Requests: privacyRepo(), Subjects: privacyRepo(), Targets: exportTargetStore{store: store},
			Rows: postgres.NewBackupExportRepository(postgres.DefaultExportBatch), Objects: exportObjects{key: "none"},
			Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}), Audit: postgres.NewAuditSink(generator{t}),
			Snapshot: unitOfWork, UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: generator{t},
			SchemaVersion: "test", ProductVersion: "test",
		}
		if _, err := exporter.Export(ctx, appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: s.tenant},
			domain.Request{
				ID: freshID(t), Kind: domain.KindAccess, Scope: domain.ScopeTenant,
				SubjectAccountID: s.account, TargetID: freshID(t),
			}); err != nil {
			t.Fatalf("writing the person's copy: %v", err)
		}
	})
	if !strings.Contains(own, "KEPTSAYS") {
		t.Error("the person's own copy left out what the hold keeps")
	}

	// A backup taken after the erasure is the backup run's own path, not the workspace export's
	// archivist: the two share only the row source.
	backup := written(func(store backupstorage.LocalStore) {
		target, err := backupdomain.NewTarget(backupdomain.NewTargetInput{
			ID: freshID(t), TenantID: s.tenant, Name: "Nightly", Kind: backupdomain.KindS3,
			Config:         backupdomain.TargetConfig{"bucket": "hubtask-backups", "region": "eu-central-1"},
			EncryptionMode: backupdomain.EncryptionNone, InsecureAcknowledged: true,
			CreatedBy: s.account, Now: created, Text: text.Composing{},
		})
		if err != nil {
			t.Fatalf("building the target: %v", err)
		}
		insertTarget(ctx, t, s.tenant, target, crypto.Sealed{})
		performer := backupservice.Performer{
			Runs: runRepo(), Targets: targetRepo(), Export: postgres.NewBackupExportRepository(postgres.DefaultExportBatch),
			Opener: foreignTarget{store: store}, Cipher: cryptoadapter.NewStream(clockadapter.CryptoRandom{}),
			Objects: exportObjects{key: "none"}, Snapshot: unitOfWork, UnitOfWork: unitOfWork,
			Clock: clockadapter.System{}, IDs: generator{t}, Entropy: clockadapter.CryptoRandom{},
			SchemaVersion: "test", ProductVersion: "test",
		}
		if _, err := performer.Perform(ctx, backupservice.PerformInput{
			RunID: freshID(t), TargetID: target.ID, TenantID: s.tenant,
			Mode: backupdomain.ModeFull, Trigger: backupdomain.TriggerManual,
		}); err != nil {
			t.Fatalf("taking the backup: %v", err)
		}
	})
	if !strings.Contains(backup, "KEPTSAYS") {
		t.Error("the backup left out what the hold keeps")
	}
}
