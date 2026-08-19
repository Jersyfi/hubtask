// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// pending answers every operation the specification declares and this installation does not serve
// yet. RestController embeds it and overrides one method per use case that lands, so this file is
// the visible remainder of the milestone: what is still missing is a list, not a guess.
//
// The routes are registered all the same. That is what lets the contract test compare the router
// against api/openapi.yaml as a whole (ADR-0004), and it keeps the metric labels bounded to the
// route templates the specification defines rather than to whatever a client asks for
// (observability-reliability.md §3.2).
type pending struct{}

var _ openapi.ServerInterface = pending{}

// notAvailable is the answer: 404 with the detail code saying which kind of 404 it is.
//
// Not 501. The status mapping in api-guidelines.md §6 knows no 501, and an adapter does not get
// to invent a contract status - a client that reads `not_found` treats the resource as absent,
// which is exactly what it is on this installation. The detail code separates "not built yet"
// from "no such object" for anyone debugging.
func notAvailable(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, shared.ErrNotFound.WithDetail("route.operation_not_available"),
		correlation.RequestIDFrom(r.Context()))
}

// The identity operations. They land one by one as B-02 registers each use case; until then the
// route exists because the contract declares it, and answers that this installation does not
// serve it yet.
func (pending) InviteAccount(w http.ResponseWriter, r *http.Request, _ openapi.InviteAccountParams) {
	notAvailable(w, r)
}

func (pending) UpdateAccountPreferences(w http.ResponseWriter, r *http.Request, _ openapi.AccountId) {
	notAvailable(w, r)
}

func (pending) GrantMembership(w http.ResponseWriter, r *http.Request, _ openapi.GrantMembershipParams) {
	notAvailable(w, r)
}

func (pending) RevokeMembership(w http.ResponseWriter, r *http.Request, _ openapi.MembershipId) {
	notAvailable(w, r)
}

func (pending) CreateGroup(w http.ResponseWriter, r *http.Request, _ openapi.CreateGroupParams) {
	notAvailable(w, r)
}

func (pending) UpdateGroup(w http.ResponseWriter, r *http.Request, _ openapi.GroupId, _ openapi.UpdateGroupParams) {
	notAvailable(w, r)
}

func (pending) DeleteGroup(w http.ResponseWriter, r *http.Request, _ openapi.GroupId) {
	notAvailable(w, r)
}

func (pending) ListAuditEntries(w http.ResponseWriter, r *http.Request, _ openapi.ListAuditEntriesParams) {
	notAvailable(w, r)
}

func (pending) VerifyAuditChain(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) CreateBackupSchedule(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ListBackupTargets(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) CreateBackupTarget(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ListBackupsAtTarget(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID, _ openapi.ListBackupsAtTargetParams) {
	notAvailable(w, r)
}

func (pending) TestBackupTarget(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

func (pending) StartBackup(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) VerifyBackup(w http.ResponseWriter, r *http.Request, _ string) { notAvailable(w, r) }

// ListContainers is overridden by RestController, for the reason given at CreateContainer.
func (pending) ListContainers(w http.ResponseWriter, r *http.Request, _ openapi.ListContainersParams) {
	notAvailable(w, r)
}

// CreateContainer is overridden by RestController. It stays here because the compile-time
// assertion above is on `pending` itself: the set has to be complete, and an operation that lands
// is one this file no longer answers rather than one it stops declaring.
func (pending) CreateContainer(w http.ResponseWriter, r *http.Request, _ openapi.CreateContainerParams) {
	notAvailable(w, r)
}

func (pending) TrashContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.TrashContainerParams) {
	notAvailable(w, r)
}

// GetContainer is overridden by RestController, for the reason given at CreateContainer.
func (pending) GetContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId) {
	notAvailable(w, r)
}

// The container lifecycle. All of these are overridden by RestController; they stay here for the
// reason CreateContainer does.
func (pending) RenameContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.RenameContainerParams) {
	notAvailable(w, r)
}

func (pending) UpdateContainerPolicies(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.UpdateContainerPoliciesParams) {
	notAvailable(w, r)
}

func (pending) MoveContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.MoveContainerParams) {
	notAvailable(w, r)
}

func (pending) ArchiveContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.ArchiveContainerParams) {
	notAvailable(w, r)
}

func (pending) UnarchiveContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.UnarchiveContainerParams) {
	notAvailable(w, r)
}

// ListBuckets and CreateBucket are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) ListBuckets(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId) {
	notAvailable(w, r)
}

func (pending) CreateBucket(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId) {
	notAvailable(w, r)
}

func (pending) UpdateBucket(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.BucketId, _ openapi.UpdateBucketParams) {
	notAvailable(w, r)
}

func (pending) ReorderBucket(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.BucketId, _ openapi.ReorderBucketParams) {
	notAvailable(w, r)
}

func (pending) DeleteBucket(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.BucketId, _ openapi.DeleteBucketParams) {
	notAvailable(w, r)
}

// ListLabels and CreateLabel are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) ListLabels(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId) {
	notAvailable(w, r)
}

func (pending) CreateLabel(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId) {
	notAvailable(w, r)
}

func (pending) UpdateLabel(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.LabelId, _ openapi.UpdateLabelParams) {
	notAvailable(w, r)
}

func (pending) DeleteLabel(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.LabelId, _ openapi.DeleteLabelParams) {
	notAvailable(w, r)
}

// AddLabel and RemoveLabel are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) AddLabel(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.LabelId) {
	notAvailable(w, r)
}

func (pending) RemoveLabel(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.LabelId) {
	notAvailable(w, r)
}

// CreateWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) CreateWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.CreateWorkItemParams) {
	notAvailable(w, r)
}

// ListWorkItems is overridden by RestController, for the reason given at CreateContainer.
func (pending) ListWorkItems(w http.ResponseWriter, r *http.Request, _ openapi.ListWorkItemsParams) {
	notAvailable(w, r)
}

func (pending) TrashWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.TrashWorkItemParams) {
	notAvailable(w, r)
}

// GetWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) GetWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.GetWorkItemParams) {
	notAvailable(w, r)
}

// UpdateWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) UpdateWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.UpdateWorkItemParams) {
	notAvailable(w, r)
}

func (pending) ListComments(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ListCommentsParams) {
	notAvailable(w, r)
}

func (pending) AddComment(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.AddCommentParams) {
	notAvailable(w, r)
}

// CompleteWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) CompleteWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.CompleteWorkItemParams) {
	notAvailable(w, r)
}

// ReopenWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) ReopenWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ReopenWorkItemParams) {
	notAvailable(w, r)
}

// MoveWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) MoveWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId) {
	notAvailable(w, r)
}

// ReorderWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) ReorderWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ReorderWorkItemParams) {
	notAvailable(w, r)
}

func (pending) RetainItem(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

func (pending) BulkItems(w http.ResponseWriter, r *http.Request, _ openapi.BulkItemsParams) {
	notAvailable(w, r)
}

func (pending) QueryItems(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) GetCapabilities(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) GetHealthReport(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) StartRestore(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ListRetentionPolicies(w http.ResponseWriter, r *http.Request, _ openapi.ListRetentionPoliciesParams) {
	notAvailable(w, r)
}

func (pending) CreateRetentionPolicy(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) PreviewRetentionPolicy(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

func (pending) ListSyncDevices(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) SyncPull(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) SyncPush(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }
