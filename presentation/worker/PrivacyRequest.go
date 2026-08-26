// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"

	service "github.com/Jersyfi/hubtask/core/application/service/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PrivacyRequest carries out a data subject request that has been started (E-10,
// data-protection.md §4): the archive an access or portability case produces, or the erasure an
// erasure case is.
//
// Detached, for the reason the backup run is: an erasure serves every storage location in the data
// catalogue and an export streams a person's whole presence to somebody else's machine, and doing
// either inside the runner's own transaction would hold one open for minutes on the pool the API
// shares.
//
// Safe to repeat, which is what makes it a job. Each step of an erasure is idempotent on its own
// terms - a credential removed twice is removed once, a pseudonym is recorded once - and an export
// writes the same archive under a name derived from the case. A case that has already been answered
// is left alone.
type PrivacyRequest struct {
	Performer service.Performer
}

var (
	_ queue.Handler  = PrivacyRequest{}
	_ queue.Detached = PrivacyRequest{}
)

// OwnsItsTransactions is the assertion queue.Detached asks for. See the type's comment.
func (h PrivacyRequest) OwnsItsTransactions() {}

// Run carries out one case.
func (h PrivacyRequest) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("privacy: a request job without a tenant")
	}

	in, err := service.RequestOf(job.Payload, job.TenantID)
	if err != nil {
		return queue.Result{}, err
	}

	request, err := h.Performer.Perform(ctx, in)
	if err != nil {
		return queue.Result{}, err
	}

	// The kind and the state, and nothing about the person: a log line is not the audit trail, and
	// what it is for is an operator seeing that the work happened (rule 10).
	slog.InfoContext(ctx, "a data subject request was carried out",
		slog.String("request_id", in.RequestID.String()),
		slog.String("kind", string(request.Kind)),
		slog.String("status", string(request.Status)))
	return queue.Result{}, nil
}
