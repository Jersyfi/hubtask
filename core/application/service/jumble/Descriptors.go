// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// Descriptor is the catalogue entry.
func (h SubmitJumbleEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SubmitJumbleEntryName,
		Summary: "Puts something in the jumble over a near channel - a quick capture or a plain " +
			"API call - and announces the arrival, which is what fires a JUMBLE_ENTRY automation " +
			"rule. The far channels EMAIL and WEBHOOK belong to the intakes that authenticate " +
			"their own way and cannot be claimed here. Attachments name media objects already " +
			"sealed through the media pipeline.",
		SideEffects: "Stores the entry, counts its attachments as references, publishes " +
			"jumble.entry_received, and writes an audit entry naming the channel - never the " +
			"content.",
		TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{
				Name: "channel", Kind: usecase.KindString,
				Enum:        []string{string(domain.ChannelQuickCapture), string(domain.ChannelAPI)},
				Description: "How this arrived. API when absent.",
			},
			{
				Name: "sender", Kind: usecase.KindString,
				Description: "Optional provenance text. Data, never an identity.",
			},
			{Name: "raw_subject", Kind: usecase.KindString, Description: "The short half."},
			{Name: "raw_body", Kind: usecase.KindString, Description: "The long half."},
			{
				Name: "attachments", Kind: usecase.KindIDList,
				Description: "Sealed media objects to carry.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntrySubmittedAction, TargetType: entryTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An entry is not a work item yet; the conversion's item write is what the " +
				"activity stream records.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ListJumbleEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListJumbleEntriesName,
		Summary: "The jumble, newest first: everything that arrived and what became of it. The " +
			"raw content is answered here - to a credential that may read the workspace's " +
			"entries - and nowhere that travels.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsReadScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "status", Kind: usecase.KindString,
				Enum: []string{
					string(domain.StatusNew), string(domain.StatusProcessed),
					string(domain.StatusDismissed),
				},
				Description: "Narrow to one state. NEW is the inbox.",
			},
			{
				Name: "channel", Kind: usecase.KindString,
				Enum: []string{
					string(domain.ChannelEmail), string(domain.ChannelWebhook),
					string(domain.ChannelQuickCapture), string(domain.ChannelAPI),
				},
				Description: "Narrow to one way of arriving.",
			},
			{Name: "cursor", Kind: usecase.KindString, Description: "Where the last page stopped."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many at most. Fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryReadAction, TargetType: entryTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SubmitJumbleEntry) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	// The body is read untrimmed: its whitespace is content - a pasted mail, a code block - and
	// the domain's own trim decides only whether there is anything in it.
	body, _ := in["raw_body"].(string)
	cmd := SubmitCommand{
		Channel:    domain.Channel(in.String("channel")),
		Sender:     in.String("sender"),
		RawSubject: in.String("raw_subject"),
		RawBody:    body,
	}
	if in.Present("attachments") {
		attachments, err := in.IDList("attachments")
		if err != nil {
			return nil, err
		}
		cmd.Attachments = attachments
	}

	entry, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return entryOutput(entry), nil
}

func (h ListJumbleEntries) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	query := repository.Query{
		Status:  domain.Status(in.String("status")),
		Channel: domain.Channel(in.String("channel")),
		Cursor:  in.String("cursor"), Size: in.Int("size"),
	}
	if query.Status != "" && !query.Status.Valid() {
		return nil, shared.ErrValidation.
			WithDetail("jumble.status_unknown").
			WithParams(map[string]string{"status": string(query.Status)}).
			WithFields(shared.FieldError{Path: "/status", Code: "jumble.status_unknown"})
	}
	if query.Channel != "" && !query.Channel.Valid() {
		return nil, shared.ErrValidation.
			WithDetail("jumble.channel_unknown").
			WithParams(map[string]string{"channel": query.Channel.String()}).
			WithFields(shared.FieldError{Path: "/channel", Code: "jumble.channel_unknown"})
	}

	page, err := h.Execute(ctx, actor, query)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Entries))
	for _, entry := range page.Entries {
		rows = append(rows, entryOutput(entry))
	}
	state := map[string]any{"next_cursor": nil, "has_more": page.HasMore}
	if page.NextCursor != "" {
		state["next_cursor"] = page.NextCursor
	}
	return usecase.Output{"data": rows, "page": state}, nil
}

// Descriptor is the catalogue entry - and the automation action CONVERT_JUMBLE_ENTRY with it,
// which is what "the basis for automatic conversion" means: a rule calls it, and the conversion
// runs as the rule's run_as account with its real rights.
func (h ConvertJumbleEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConvertJumbleEntryName,
		Summary: "Turns a jumble entry into a work item at a named destination - through the " +
			"same use case a plain create goes through, with the destination's rights checked " +
			"there - records the provenance on both sides, settles the entry as PROCESSED and " +
			"announces jumble.entry_converted. Exactly once: a second conversion of the same " +
			"entry is refused, and two racing produce one item and one refusal.",
		SideEffects: "Creates the item, settles the entry, publishes the event, and writes audit " +
			"entries for both halves.",
		TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{
				Name: "entry_id", Kind: usecase.KindID, Required: true,
				Description: "Which entry. A rule leaves this out and the run supplies the " +
					"entry it is about.",
			},
			{
				Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The destination collection.",
			},
			{Name: "bucket_id", Kind: usecase.KindID, Description: "The board column, when the destination has a board."},
			{
				Name: "title", Kind: usecase.KindString,
				Description: "The item's title. Defaults to the entry's subject, then to the " +
					"first line of its body; required for an entry with no text.",
			},
			{
				Name: "type", Kind: usecase.KindString,
				Enum:        []string{"TASK", "WORK_PACKAGE", "ACTIVITY"},
				Description: "What the entry becomes. TASK when absent.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryConvertedAction, TargetType: entryTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The item create inside the conversion writes the activity step; a second " +
				"one for the entry would show one act twice.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry - and DISMISS_JUMBLE_ENTRY.
func (h DismissJumbleEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DismissJumbleEntryName,
		Summary: "Decides against a jumble entry. A state, not a deletion: the entry stays " +
			"readable, and the retention engine ages it out by rule rather than by hand. Only a " +
			"NEW entry - an entry is decided about exactly once.",
		SideEffects: "Settles the entry and writes an audit entry.",
		TokenScope:  itemsWriteScope,
		Input: []usecase.Field{
			{
				Name: "entry_id", Kind: usecase.KindID, Required: true,
				Description: "Which entry. A rule leaves this out and the run supplies the " +
					"entry it is about.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: EntryDismissedAction, TargetType: entryTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An entry is not a work item; nothing in a workspace's history changed.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConvertJumbleEntry) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	entryID, err := in.ID("entry_id")
	if err != nil {
		return nil, err
	}
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	cmd := ConvertCommand{
		EntryID: entryID, CollectionID: collectionID,
		Title: in.String("title"), Type: in.String("type"),
	}
	if in.Present("bucket_id") {
		bucketID, err := in.ID("bucket_id")
		if err != nil {
			return nil, err
		}
		cmd.BucketID = bucketID
	}

	entry, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return entryOutput(entry), nil
}

func (h DismissJumbleEntry) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	entryID, err := in.ID("entry_id")
	if err != nil {
		return nil, err
	}
	entry, err := h.Execute(ctx, actor, entryID)
	if err != nil {
		return nil, err
	}
	return entryOutput(entry), nil
}
