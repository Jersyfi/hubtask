// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RecordAttestation reminds the owners of objects an attestation round still waits for
// (UC-AST-10), once per person per call. ATTESTATION's switch decides. The record names the round
// and the writer who reminds - nothing else (rule 10); the round's name and how many objects wait
// are read at delivery. Who is told was judged by the application before (rule 2).
type RecordAttestation struct {
	Notifications repository.Notifications
	Accounts      identityrepo.Accounts
	Preferences   repository.Preferences
	Jobs          Queue
	Clock         clock.Clock
	IDs           clock.IDGenerator
	Signals       Signals
}

// AttestationReminder writes one record per owner, naming the writer who reminds.
func (r RecordAttestation) AttestationReminder(ctx context.Context, tenant, writer, campaign shared.ID, owners []shared.ID) error {
	at := r.Clock.Now()
	for _, person := range owners {
		if person.IsZero() || person == writer {
			continue
		}
		written, err := domain.New(domain.NewInput{
			ID: r.IDs.NewID(), TenantID: tenant, RecipientID: person, Category: domain.CategoryAttestation,
			Channel: domain.ChannelEmail, CampaignID: campaign, ActorID: writer, At: at,
		})
		if err != nil {
			return err
		}
		decision, err := decideFor(ctx, r.Accounts, r.Preferences, written, person, domain.CategoryAttestation)
		if err != nil {
			return err
		}
		if !decision.Send {
			written = written.Suppress(decision.Reason)
		}
		first, err := r.Notifications.Insert(ctx, written)
		if err != nil {
			return err
		}
		if !first {
			continue
		}
		if r.Signals != nil {
			r.Signals.NotificationRecorded(ctx, written.Category.String(), written.Channel.String(), written.State.String())
		}
		if !decision.Send {
			continue
		}
		if _, err := r.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindAttestationEmail, TenantID: written.TenantID, DedupeKey: written.ID.String(),
			RunAt: at, MaxAttempts: queue.PageReviewAttempts,
			Payload: map[string]any{"notification_id": written.ID.String()},
		}); err != nil {
			return err
		}
	}
	return nil
}
