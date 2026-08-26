// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// Data subject rights, stored (E-10, data-protection.md §4). The tables have stood since
// `0001_init` and these are the first statements over them.
//
// Nothing here names a tenant: the transaction the caller opened decided that, and row level
// security applies it to every statement (ADR-0010). That holds for the installation-wide case as
// well - what crosses the boundary is a list of tenant identifiers, and the collection that follows
// opens one ordinary transaction per tenant.
type PrivacyRepository struct {
	cursors security.CursorCodec
}

func NewPrivacyRepository(cursors security.CursorCodec) PrivacyRepository {
	return PrivacyRepository{cursors: cursors}
}

var (
	_ repository.Requests   = PrivacyRepository{}
	_ repository.Consents   = PrivacyRepository{}
	_ repository.Subjects   = PrivacyRepository{}
	_ repository.Pseudonyms = PrivacyRepository{}
	_ repository.Erasure    = PrivacyRepository{}
)

// Insert records a new case.
func (r PrivacyRepository) Insert(ctx context.Context, request domain.Request) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(request.ID)
	if err != nil {
		return err
	}
	subject, err := optionalUUID(request.SubjectAccountID)
	if err != nil {
		return err
	}
	handler, err := optionalUUID(request.HandledBy)
	if err != nil {
		return err
	}
	target, err := optionalUUID(request.TargetID)
	if err != nil {
		return err
	}

	if err := queries.InsertDataSubjectRequest(ctx, sqlc.InsertDataSubjectRequestParams{
		ID: id, SubjectAccountID: subject, SubjectEmail: optionalText(request.SubjectEmail),
		Kind: string(request.Kind), Status: string(request.Status), Scope: string(request.Scope),
		ErasureMode: optionalText(string(request.ErasureMode)),
		ReceivedAt:  timestampOf(request.ReceivedAt), DueAt: timestampOf(request.DueAt),
		HandledBy: handler, TargetID: target, Notes: optionalText(request.Notes),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording a data subject request: %w", err))
	}
	return nil
}

// Find answers one case.
func (r PrivacyRepository) Find(ctx context.Context, id shared.ID) (domain.Request, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Request{}, err
	}

	row, err := queries.FindDataSubjectRequest(ctx, key)
	switch {
	case IsNoRows(err):
		return domain.Request{}, shared.ErrNotFound.
			WithDetail(domain.CodeRequestNotFound).
			WithParams(map[string]string{"request_id": id.String()})
	case err != nil:
		return domain.Request{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a data subject request: %w", err))
	}
	return requestFrom(sqlc.ListDataSubjectRequestsRow(row))
}

// Save writes a case back after the domain has moved it.
func (r PrivacyRepository) Save(ctx context.Context, request domain.Request) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	id, err := uuidOf(request.ID)
	if err != nil {
		return false, err
	}
	handler, err := optionalUUID(request.HandledBy)
	if err != nil {
		return false, err
	}
	target, err := optionalUUID(request.TargetID)
	if err != nil {
		return false, err
	}
	subject, err := optionalUUID(request.SubjectAccountID)
	if err != nil {
		return false, err
	}

	rows, err := queries.UpdateDataSubjectRequest(ctx, sqlc.UpdateDataSubjectRequestParams{
		ID: id, Status: string(request.Status),
		ErasureMode:      optionalText(string(request.ErasureMode)),
		HandledBy:        handler,
		RejectionReason:  optionalText(request.RejectionReason),
		CompletedAt:      optionalInstant(request.CompletedAt),
		TargetID:         target,
		ResultArchive:    optionalText(request.ResultArchive),
		SubjectAccountID: subject,
		Notes:            optionalText(request.Notes),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing a data subject request: %w", err))
	}
	return rows > 0, nil
}

// List answers one page of the cases, soonest deadline first.
func (r PrivacyRepository) List(
	ctx context.Context, filter repository.Filter,
) (repository.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Page{}, err
	}
	boundary, err := requestCursor(r.cursors, filter.Cursor)
	if err != nil {
		return repository.Page{}, err
	}

	rows, err := queries.ListDataSubjectRequests(ctx, sqlc.ListDataSubjectRequestsParams{
		IncludeClosed: filter.IncludeClosed,
		Status:        optionalText(string(filter.Status)),
		Kind:          optionalText(string(filter.Kind)),
		DueBefore:     timestampOf(filter.DueBefore),
		CursorDueAt:   boundary.dueAt,
		CursorID:      boundary.id,
		PageSize:      pageProbe(filter.Size),
	})
	if err != nil {
		return repository.Page{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the data subject requests: %w", err))
	}

	requests := make([]domain.Request, 0, len(rows))
	for _, row := range rows {
		request, err := requestFrom(row)
		if err != nil {
			return repository.Page{}, err
		}
		requests = append(requests, request)
	}

	kept, info := pageOf(requests, filter.Size, r.cursors, func(request domain.Request) security.Position {
		return security.At(request.DueAt.UTC().Format(time.RFC3339Nano), request.ID)
	})
	return repository.Page{
		Requests: kept,
		Info:     repository.PageInfo{NextCursor: info.NextCursor, HasMore: info.HasMore},
	}, nil
}

// Deadlines is the reading behind alert A-19.
func (r PrivacyRepository) Deadlines(
	ctx context.Context, now time.Time,
) (repository.Deadlines, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Deadlines{}, err
	}

	row, err := queries.OverdueRequestCount(ctx, timestampOf(now))
	if err != nil {
		return repository.Deadlines{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the open data subject requests: %w", err))
	}

	deadlines := repository.Deadlines{Overdue: int(row.Overdue), Open: int(row.OpenCases)}
	if next, ok := row.NextDueAt.(time.Time); ok {
		deadlines.NextDueAt = next.UTC()
	}
	return deadlines, nil
}

// Withdraw ends every standing consent of an account for a purpose.
func (r PrivacyRepository) Withdraw(
	ctx context.Context, accountID shared.ID, purpose string, at time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := optionalUUID(accountID)
	if err != nil {
		return 0, err
	}

	rows, err := queries.RevokeConsent(ctx, sqlc.RevokeConsentParams{
		RevokedAt: timestampOf(at), Purpose: purpose, AccountID: account,
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("withdrawing a consent: %w", err))
	}
	return int(rows), nil
}

// Record writes one consent record.
func (r PrivacyRepository) Record(ctx context.Context, consent domain.Consent) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(consent.ID)
	if err != nil {
		return err
	}
	account, err := optionalUUID(consent.AccountID)
	if err != nil {
		return err
	}

	if err := queries.InsertConsentRecord(ctx, sqlc.InsertConsentRecordParams{
		ID: id, AccountID: account, Purpose: consent.Purpose, Granted: consent.Granted,
		GrantedAt: timestampOf(consent.GrantedAt), RevokedAt: optionalInstant(consent.RevokedAt),
		Source: optionalText(consent.Source),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording a consent: %w", err))
	}
	return nil
}

// Latest answers the most recent record for an account and a purpose.
func (r PrivacyRepository) Latest(
	ctx context.Context, accountID shared.ID, purpose string,
) (domain.Consent, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Consent{}, err
	}
	account, err := optionalUUID(accountID)
	if err != nil {
		return domain.Consent{}, err
	}

	row, err := queries.LatestConsent(ctx, sqlc.LatestConsentParams{
		Purpose: purpose, AccountID: account,
	})
	switch {
	case IsNoRows(err):
		return domain.Consent{}, shared.ErrNotFound.WithDetail(domain.CodeConsentNotFound)
	case err != nil:
		return domain.Consent{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a consent: %w", err))
	}

	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Consent{}, err
	}
	consentAccount, err := optionalID(row.AccountID)
	if err != nil {
		return domain.Consent{}, err
	}
	return domain.Consent{
		ID: id, AccountID: consentAccount, Purpose: row.Purpose, Granted: row.Granted,
		GrantedAt: timeFrom(row.GrantedAt), RevokedAt: timeFrom(row.RevokedAt),
		Source: stringFrom(row.Source),
	}, nil
}

// SetStatus writes an account's status.
func (r PrivacyRepository) SetStatus(
	ctx context.Context, id shared.ID, status string, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	rows, err := queries.SetAccountStatus(ctx, sqlc.SetAccountStatusParams{
		ID: key, Status: sqlc.AccountStatus(status), UpdatedAt: timestampOf(at),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("setting an account status: %w", err))
	}
	return rows > 0, nil
}

// Tenants answers the workspaces in which one address is a member.
//
// The one cross-tenant read in this adapter, and it is a function call rather than a query: it
// answers identifiers and nothing else, and `SET LOCAL app.tenant_id` is not relaxed for it
// (db/migrations/0044_privacy_requests.sql).
func (r PrivacyRepository) Tenants(ctx context.Context, email string) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.SubjectTenants(ctx, email)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the workspaces of a data subject: %w", err))
	}

	tenants := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		tenant, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		tenants = append(tenants, tenant)
	}
	return tenants, nil
}

// Assign maps one actor to one pseudonym.
func (r PrivacyRepository) Assign(
	ctx context.Context, actorID shared.ID, pseudonym, reason string, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	actor, err := uuidOf(actorID)
	if err != nil {
		return err
	}

	if err := queries.InsertAuditPseudonym(ctx, sqlc.InsertAuditPseudonymParams{
		ActorID: actor, Pseudonym: pseudonym, Reason: reason, CreatedAt: timestampOf(at),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording an audit pseudonym: %w", err))
	}
	return nil
}

// For answers the pseudonyms of the actors named.
func (r PrivacyRepository) For(
	ctx context.Context, actorIDs []shared.ID,
) (map[shared.ID]string, error) {
	pseudonyms := map[shared.ID]string{}
	if len(actorIDs) == 0 {
		return pseudonyms, nil
	}

	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	keys := make([]pgtype.UUID, 0, len(actorIDs))
	for _, actorID := range actorIDs {
		key, err := uuidOf(actorID)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	rows, err := queries.AuditPseudonyms(ctx, keys)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the audit pseudonyms: %w", err))
	}
	for _, row := range rows {
		actor, err := idFrom(row.ActorID)
		if err != nil {
			return nil, err
		}
		pseudonyms[actor] = row.Pseudonym
	}
	return pseudonyms, nil
}

// requestBoundary is a decoded cursor: the deadline and the case the page continues after.
type requestBoundary struct {
	dueAt pgtype.Timestamptz
	id    pgtype.UUID
}

func requestCursor(cursors security.CursorCodec, cursor string) (requestBoundary, error) {
	if cursor == "" {
		return requestBoundary{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return requestBoundary{}, err
	}
	dueAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return requestBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return requestBoundary{}, err
	}
	return requestBoundary{dueAt: timestampOf(dueAt), id: id}, nil
}

// requestFrom maps one row onto the case.
func requestFrom(row sqlc.ListDataSubjectRequestsRow) (domain.Request, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Request{}, err
	}
	subject, err := optionalID(row.SubjectAccountID)
	if err != nil {
		return domain.Request{}, err
	}
	handler, err := optionalID(row.HandledBy)
	if err != nil {
		return domain.Request{}, err
	}
	target, err := optionalID(row.TargetID)
	if err != nil {
		return domain.Request{}, err
	}

	return domain.Request{
		ID: id, Kind: domain.Kind(row.Kind), Status: domain.Status(row.Status),
		Scope: domain.Scope(row.Scope), SubjectAccountID: subject,
		SubjectEmail: stringFrom(row.SubjectEmail),
		ErasureMode:  domain.ErasureMode(stringFrom(row.ErasureMode)),
		ReceivedAt:   timeFrom(row.ReceivedAt), DueAt: timeFrom(row.DueAt),
		CompletedAt: timeFrom(row.CompletedAt), HandledBy: handler,
		RejectionReason: stringFrom(row.RejectionReason),
		TargetID:        target, ResultArchive: stringFrom(row.ResultArchive),
		Notes: stringFrom(row.Notes),
	}, nil
}

// optionalInstant is timestampOf for a moment that may be absent: the zero time reaches the column
// as NULL rather than as a moment in 1970.
func optionalInstant(value time.Time) pgtype.Timestamptz {
	if value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return timestampOf(value)
}

// The erasure, one storage location at a time (E-10, data-protection.md §5).

// Anonymise keeps the row and takes everything of the person's out of it.
func (r PrivacyRepository) Anonymise(
	ctx context.Context, accountID shared.ID, marker string, at time.Time,
) (bool, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return false, err
	}

	rows, err := queries.AnonymiseAccount(ctx, sqlc.AnonymiseAccountParams{
		ID: id, Marker: marker, UpdatedAt: timestampOf(at),
	})
	if err != nil {
		return false, erasureFailed("anonymising an account", err)
	}
	return rows > 0, nil
}

// Delete removes the account and lets the cascades take the rest.
func (r PrivacyRepository) Delete(ctx context.Context, accountID shared.ID) (bool, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return false, err
	}

	rows, err := queries.DeleteAccount(ctx, id)
	if err != nil {
		return false, erasureFailed("deleting an account", err)
	}
	return rows > 0, nil
}

// RevokeCredentials removes every token, feed and device of the person.
func (r PrivacyRepository) RevokeCredentials(
	ctx context.Context, accountID shared.ID,
) (int, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}

	removed := int64(0)
	for what, remove := range map[string]func(context.Context, pgtype.UUID) (int64, error){
		"the access tokens":  queries.DeleteCredentialsOfAccount,
		"the calendar feeds": queries.DeleteFeedsOfAccount,
		"the sync devices":   queries.DeleteDevicesOfAccount,
	} {
		rows, err := remove(ctx, id)
		if err != nil {
			return 0, erasureFailed("removing "+what+" of an account", err)
		}
		removed += rows
	}
	return int(removed), nil
}

// DiscardNotifications removes what was sent to them and what was about to be.
func (r PrivacyRepository) DiscardNotifications(
	ctx context.Context, accountID shared.ID,
) (int, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}

	rows, err := queries.DeleteNotificationsOfAccount(ctx, id)
	if err != nil {
		return 0, erasureFailed("removing the notifications of an account", err)
	}
	return int(rows), nil
}

// ReleaseAssignments hands the work back to nobody.
func (r PrivacyRepository) ReleaseAssignments(
	ctx context.Context, accountID shared.ID, at time.Time,
) (int, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}

	rows, err := queries.ClearAssignmentsOfAccount(ctx, sqlc.ClearAssignmentsOfAccountParams{
		AccountID: id, UpdatedAt: timestampOf(at),
	})
	if err != nil {
		return 0, erasureFailed("releasing the assignments of an account", err)
	}
	return int(rows), nil
}

// AuthoredComments answers the person's own contributions.
func (r PrivacyRepository) AuthoredComments(
	ctx context.Context, accountID shared.ID,
) ([]repository.Authored, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.CommentsAuthoredBy(ctx, id)
	if err != nil {
		return nil, erasureFailed("reading the comments of an account", err)
	}

	authored := make([]repository.Authored, 0, len(rows))
	for _, row := range rows {
		commentID, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		itemID, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		authored = append(authored, repository.Authored{ID: commentID, ItemID: itemID})
	}
	return authored, nil
}

// DeleteAuthoredComments removes them.
func (r PrivacyRepository) DeleteAuthoredComments(
	ctx context.Context, accountID shared.ID,
) (int, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return 0, err
	}

	rows, err := queries.DeleteCommentsAuthoredBy(ctx, id)
	if err != nil {
		return 0, erasureFailed("removing the comments of an account", err)
	}
	return int(rows), nil
}

// OrphanedMedia answers the uploads nothing points at any more.
func (r PrivacyRepository) OrphanedMedia(
	ctx context.Context, accountID shared.ID,
) ([]repository.Medium, error) {
	queries, id, err := r.accountQuery(ctx, accountID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.MediaUploadedBy(ctx, id)
	if err != nil {
		return nil, erasureFailed("reading the media of an account", err)
	}

	media := make([]repository.Medium, 0, len(rows))
	for _, row := range rows {
		mediaID, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		media = append(media, repository.Medium{ID: mediaID, StorageKey: row.StorageKey})
	}
	return media, nil
}

// DiscardMedium removes one medium's row.
func (r PrivacyRepository) DiscardMedium(ctx context.Context, mediaID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(mediaID)
	if err != nil {
		return err
	}

	if _, err := queries.DeleteMediaObject(ctx, id); err != nil {
		return erasureFailed("removing a medium", err)
	}
	return nil
}

// accountQuery is the two lines every statement above starts with.
func (r PrivacyRepository) accountQuery(
	ctx context.Context, accountID shared.ID,
) (*sqlc.Queries, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return queries, id, nil
}

// erasureFailed keeps the driver's message out of the answer, for the reason rule 10 gives: a
// message from a driver carries the values of the statement, and here those are a person's.
func erasureFailed(what string, cause error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s: %w", what, cause))
}
