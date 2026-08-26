// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
// data catalogue by the privacy gates (E-11's PG-3), which is what stops it drifting as tables are
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
	// Their own words and their own work.
	"comment":         {"author_id"},
	"activity_entry":  {"actor_id"},
	"work_item":       {"created_by", "assignee_id"},
	"container":       {"created_by"},
	"media_object":    {"created_by"},
	"saved_view":      {"owner_id"},
	"jumble_entry":    {"created_by"},
	"automation_rule": {"created_by", "run_as"},
	"legal_hold":      {"placed_by", "released_by"},
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
}

// SubjectTables answers which tables a subject export reads and which it deliberately leaves out,
// so that gate PG-3 can reconcile both against the data catalogue rather than against a reading of
// this file.
func SubjectTables() (byPerson map[string][]string, byAddress map[string][]string, excluded map[string]string) {
	return subjectColumns, subjectEmailColumns, notThePersons
}

// subjectSource hands the writer only what belongs to the person.
type subjectSource struct {
	inner   archive.Source
	subject shared.ID
	email   string
	// counted is how many records went in, for the record the export leaves behind.
	counted *int
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
