// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The one thing an access export changes about a backup: which rows go in.
//
// A filter over the ordinary source rather than a second reader. The archive's shape, its manifest,
// its checksums and its restorability are then the backup's - which is what `backup-restore.md` §9
// means by "without a second format coming into existence" - and what this file decides is only
// the question "is this row about the person".

// subjectColumns is the column that makes a row the person's, per table.
//
// This map **is** the export's completeness, and it is written out rather than derived for the same
// reason the role matrix is: derivation would be shorter and wrong. It is reconciled against the
// data catalogue by the privacy gate PG-3, which is what stops it drifting as tables are
// added - and a table that is not here contributes nothing, which is the safe direction for the
// workspace and the unsafe one for the export, so the gate matters.
var subjectColumns = map[string][]string{
	// The person themselves, and everything that names them as a member.
	"account":              {"id"},
	"membership":           {"account_id"},
	"account_group_member": {"account_id"},
	"item_member":          {"account_id"},
	// What they were told, what they asked to be told, and what they agreed to.
	"notification_preference": {"account_id"},
	"consent_record":          {"account_id"},
	// Their credentials, which are in the archive as rows about them rather than as usable
	// secrets: the token's hash is what is stored, and §8.4 keeps a restore from making it valid
	// again in any case.
	"calendar_feed": {"account_id"},
	// Their passkeys by name and date; the copy is redacted, so no identifier or key leaves.
	"webauthn_credential": {"account_id"},
	// Their own words and their own work.
	"comment":          {"author_id"},
	"activity_entry":   {"actor_id"},
	"work_item":        {"created_by", "assignee_id"},
	"container":        {"created_by"},
	"media_object":     {"created_by"},
	"saved_view":       {"owner_id"},
	"structure_preset": {"owner_id"},
	"jumble_entry":     {"created_by"},
	"automation_rule":  {"created_by", "run_as"},
	"legal_hold":       {"placed_by", "released_by"},
	// The emergency accesses they opened, with the reason they wrote (ADR-0073 §4).
	"private_hub_access": {"opened_by"},
	// The workspace's own roles they hold, and their own settings: a person's level of the settings
	// catalogue is the one whose scope is their account.
	"custom_role_grant": {"account_id"},
	"setting_value":     {"scope_id"},
	// The configuration they published, linked, proposed, decided or wrote a template of.
	"workflow":   {"published_by"},
	"entry_link": {"created_by"},
	// The baselines of a plan they took (M2).
	"plan_baseline": {"taken_by"},
	// The sprints they planned and the estimates they gave (M2).
	"sprint": {"created_by"},
	// The time they recorded, their running timer, their weeks and the weeks they decided, and the
	// time policy they set (M7).
	"time_record": {"account_id"},
	"time_timer":  {"account_id"},
	"timesheet":   {"account_id", "decided_by"},
	"time_policy": {"updated_by"},
	// Their capacity and absences, the ones they recorded for others, and the rates and budgets
	// they set or that price them (M7).
	"time_capacity":  {"account_id", "updated_by"},
	"time_absence":   {"account_id", "created_by"},
	"time_cost_rate": {"account_id", "updated_by"},
	"time_budget":    {"updated_by"},
	// The goals they own or set, the entries they linked, their check-ins, the portfolios they
	// own and the updates they wrote, and their dashboards (M8).
	"goal_period":      {"created_by", "closed_by"},
	"goal":             {"owner_id", "created_by", "updated_by"},
	"goal_entry":       {"created_by"},
	"goal_check_in":    {"author_id"},
	"portfolio":        {"owner_id", "created_by", "updated_by"},
	"portfolio_update": {"author_id"},
	"dashboard":        {"owner_id", "updated_by"},
	"dashboard_digest": {"account_id"},
	"entry_estimate":   {"updated_by"},
	// The pages they wrote, own or checked, their versions and their comments (M3).
	"page":         {"created_by", "updated_by", "owner_id", "verified_by"},
	"page_version": {"author_id"},
	"page_comment": {"author_id", "resolved_by"},
	// The whiteboards they made or last drew on (M4).
	"whiteboard":         {"created_by", "updated_by"},
	"whiteboard_version": {"author_id"},
	// The updates they wrote to a live page or board (F-8); an erased author leaves none.
	"realtime_update": {"author_id"},
	// The configuration items they made, changed or own, the relations they made, the
	// reconciliations they ran and the holds and duplicates they closed (M6).
	"cmdb_ci":        {"created_by", "updated_by", "owner_account_id"},
	"cmdb_relation":  {"created_by"},
	"cmdb_run":       {"actor_id"},
	"cmdb_hold":      {"resolved_by"},
	"cmdb_duplicate": {"resolved_by"},
	// The connectors that run with their rights (M6).
	"cmdb_connector":  {"run_as"},
	"config_revision": {"actor_id", "proposed_by"},
	"config_proposal": {"proposed_by", "decided_by"},
	"config_template": {"created_by"},
	// The trail's own entries about them. An access request that left these out would be
	// incomplete: they are personal data, and they are the record of what was done with the
	// person's data.
	"audit_log": {"actor_id", "on_behalf_of_id"},
}

// subjectEmailColumns is the same question where a row names a person by address rather than by
// identifier - somebody who wrote in before they ever had an account.
var subjectEmailColumns = map[string][]string{
	"jumble_entry": {"sender"},
}

// notThePersons is every table an archive carries that is deliberately **not** in a subject export,
// and why.
//
// A map rather than a comment, for the reason `archive.NotRestored` is one: the reasons are read by
// gate PG-3, which reconciles this file against the data catalogue, and by anybody wondering why
// their workspace's buckets are not in the copy of their data. A table that is in neither map is
// what PG-3 fails on - the safe direction for the workspace is the unsafe one for the person, so
// silence must not be the answer.
var notThePersons = map[string]string{
	"tenant":                  "the workspace itself: its name is not a person's data, and the archive needs the row to be restorable",
	"bucket":                  "a column of a board is the workspace's shape",
	"label":                   "the workspace's vocabulary",
	"item_label":              "which of the workspace's labels sits on an entry",
	"custom_field_definition": "the workspace's own fields",
	"auto_assign_policy":      "how a collection hands work out, which is a rule rather than a person",
	"account_group":           "a team is the workspace's structure; the person's membership of one is in the map above",
	"item_attachment":         "which file hangs off which entry - the file itself is in media_objects, filtered by who uploaded it",
	"set_element":             "the ordering of a set, which names no person",
	"recurrence_rule":         "how a series repeats",
	"reminder":                "a reminder names its recipients in an array rather than a column; a person's own reminders arrive with the entry they are on",
	"template":                "a shape other people stamp out; it carries no author column",
	"retention_policy":        "the workspace's periods",
	"webhook_subscription":    "an integration of the workspace's",
	"container_milestone":     "the moment a collection or hub was finished, which names nobody",
	"custom_role":             "a role of the workspace's own is its vocabulary of permissions; whom it is granted to is in the map above",
	"entry_type":              "the workspace's own kinds of entries",
	"workflow_scheme":         "which workflow a hub or collection runs for a type, which names nobody",
	"link_type":               "the workspace's vocabulary of links",
	"config_template_use":     "which template the workspace took at which version, which names nobody",
	"realtime_document":       "a live document's head - its last place and its snapshot's - which names nobody",
	"realtime_snapshot":       "a compaction of many authors' updates into one state, which names nobody; each author's own updates are in the map above",
	"whiteboard_media":        "which media object lies on which board - the file itself is in media_objects, filtered by who uploaded it",
	"cmdb_class":              "the workspace's own classes of configuration items",
	"cmdb_relation_type":      "the workspace's vocabulary of relations between configuration items",
	"cmdb_source":             "a source of the inventory is an integration of the workspace's, not a person",
	"cmdb_ci_identifier":      "the identifying values of a configuration item, which arrive with nobody's name; the CI itself is in the map above",
	"cmdb_alert":              "a monitoring program's problem bound to a configuration item, which names nobody; its entry is the entry's",
	"business_calendar":       "the workspace's working hours and holidays",
	"sla_policy":              "the workspace's promised response and resolution times",
	"entry_sla":               "how an entry's promised times stood, which names nobody; the entry is the entry's",
	"reply_template":          "the workspace's canned replies, text the team wrote once for many requests",
	"entry_schedule":          "whether the scheduler may move an entry, which names nobody; the entry is the entry's",
	"plan_baseline_entry":     "an entry's dates as a baseline holds them, which name nobody; the baseline names who took it",
	"sprint_entry":            "an entry's membership in a sprint, which names nobody; the sprint names who planned it",
	"time_work_type":          "the workspace's kinds of work, which name nobody",
	"satisfaction_rating":     "a score of a resolved request, which names nobody; the rater is who raised the entry, and their words are a comment of theirs",
}

// SubjectTables answers which tables a subject export reads and which it deliberately leaves out,
// so that gate PG-3 can reconcile both against the data catalogue rather than against a reading of
// this file.
func SubjectTables() (byPerson map[string][]string, byAddress map[string][]string, excluded map[string]string) {
	return subjectColumns, subjectEmailColumns, notThePersons
}

// kudosSection is the file of a person's copy that holds the kudos they gave and received.
//
// A section rather than an entity of the archive: an archive of a workspace is an administrator's
// to read, and who acknowledged whom is the business of the two people alone (UC-WRK-29 check 6).
// The person's copy is theirs, and an access request answered without what they gave and received
// would be short (data-protection.md §4). No restore reads it.
var kudosSection = archive.Entity{
	Name: "kudos", Table: "kudos", Whole: true, Keys: []string{"item_id", "given_by", "given_to"},
}

// subjectSource hands the writer only what belongs to the person.
type subjectSource struct {
	inner   archive.Source
	subject shared.ID
	email   string
	// counted is how many records went in, for the record the export leaves behind.
	counted *int
	// kudos reads the person's kudos section; the archive's own source never carries any.
	kudos repository.OwnKudos
}

var _ archive.Source = subjectSource{}

// Records filters one entity down to the person's rows.
//
// A table with no column naming a person contributes nothing at all - the workspace's buckets,
// labels and custom fields are not somebody's personal data, and an export that carried them would
// be handing one member a copy of the workspace.
func (s subjectSource) Records(
	ctx context.Context, entity archive.Entity, since time.Time, yield func(archive.Record) error,
) error {
	if entity.Name == kudosSection.Name {
		return s.ownKudos(ctx, yield)
	}
	byID, hasID := subjectColumns[entity.Table]
	byEmail, hasEmail := subjectEmailColumns[entity.Table]
	if !hasID && !hasEmail {
		return nil
	}

	return s.inner.Records(ctx, entity, since, func(record archive.Record) error {
		if !s.owns(record, byID, byEmail) {
			return nil
		}
		if s.counted != nil {
			*s.counted++
		}
		return yield(record)
	})
}

// ownKudos writes the kudos section: each the person gave or received, both sides named by account,
// with the entry and the moment.
func (s subjectSource) ownKudos(ctx context.Context, yield func(archive.Record) error) error {
	if s.kudos == nil {
		return shared.ErrInternal.WithDetail("privacy.export_kudos_unwired").
			WithCause(errors.New("the person's copy has no reader for their kudos"))
	}
	return s.kudos.KudosOf(ctx, s.subject, func(kudos work.Kudos) error {
		if s.counted != nil {
			*s.counted++
		}
		return yield(archive.Record{
			ID: kudos.ItemID.String() + "/" + kudos.GivenBy.String() + "/" + kudos.GivenTo.String(),
			Op: archive.OpUpsert, UpdatedAt: kudos.GivenAt,
			Data: map[string]any{
				"item_id": kudos.ItemID.String(), "given_by": kudos.GivenBy.String(),
				"given_to": kudos.GivenTo.String(), "given_at": kudos.GivenAt.UTC().Format(time.RFC3339Nano),
			},
		})
	})
}

// owns decides whether one record is about the person.
//
// A deletion marker is carried through unconditionally when the entity is one of theirs: a record
// the archive says was deleted names an identifier and nothing else, so there is no column to
// compare - and leaving it out would produce an archive whose restore recreated a row this
// installation deleted.
func (s subjectSource) owns(record archive.Record, byID, byEmail []string) bool {
	if record.Op == archive.OpDelete {
		return true
	}

	for _, column := range byID {
		if !s.subject.IsZero() && matches(record.Data[column], s.subject.String()) {
			return true
		}
	}
	for _, column := range byEmail {
		if s.email != "" && matchesFold(record.Data[column], s.email) {
			return true
		}
	}
	return false
}

func matches(value any, wanted string) bool {
	text, ok := value.(string)
	return ok && text == wanted
}

func matchesFold(value any, wanted string) bool {
	text, ok := value.(string)
	return ok && strings.EqualFold(text, wanted)
}
