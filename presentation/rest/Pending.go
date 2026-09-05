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

func (pending) GetOwnAccount(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

// GetAccount is overridden by RestController, for the reason given at CreateContainer.
func (pending) GetAccount(w http.ResponseWriter, r *http.Request, _ openapi.AccountId) {
	notAvailable(w, r)
}

func (pending) UpdateAccountPreferences(w http.ResponseWriter, r *http.Request, _ openapi.AccountId) {
	notAvailable(w, r)
}

func (pending) GrantMembership(w http.ResponseWriter, r *http.Request, _ openapi.GrantMembershipParams) {
	notAvailable(w, r)
}

func (pending) RevokeMembership(w http.ResponseWriter, r *http.Request, _ openapi.MembershipId, _ openapi.RevokeMembershipParams) {
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

// The session operations of H-01. They land one by one as the use cases register; until then the
// route exists because the contract declares it, and answers that this installation does not
// serve it yet.
func (pending) SignIn(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) RefreshSession(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ListSessions(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) RevokeSession(w http.ResponseWriter, r *http.Request, _ openapi.SessionId) {
	notAvailable(w, r)
}

func (pending) RevokeAllSessions(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) RedeemInvitation(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// The MFA operations of H-02, pending until each use case lands.
func (pending) CompleteSignIn(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) EnrollTotp(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ConfirmTotp(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) DisableTotp(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) StepUp(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// The control plane of H-06, pending until each use case lands.
func (pending) ListTenants(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ProvisionTenant(w http.ResponseWriter, r *http.Request, _ openapi.ProvisionTenantParams) {
	notAvailable(w, r)
}

func (pending) SuspendTenant(w http.ResponseWriter, r *http.Request, _ openapi.AdminTenantId) {
	notAvailable(w, r)
}

func (pending) ResumeTenant(w http.ResponseWriter, r *http.Request, _ openapi.AdminTenantId) {
	notAvailable(w, r)
}

func (pending) RequestTenantDeletion(w http.ResponseWriter, r *http.Request, _ openapi.AdminTenantId, _ openapi.RequestTenantDeletionParams) {
	notAvailable(w, r)
}

// The tenant export of H-07, pending until the use case lands.
func (pending) ExportTenant(w http.ResponseWriter, r *http.Request, _ openapi.AdminTenantId) {
	notAvailable(w, r)
}

// The quota surface of H-08, pending until each use case lands.
func (pending) ReadQuotas(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) UpdateTenantQuotas(w http.ResponseWriter, r *http.Request, _ openapi.AdminTenantId) {
	notAvailable(w, r)
}

// The OAuth2 provider of H-05, pending until each use case lands.
func (pending) ListOauthClients(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) RegisterOauthClient(w http.ResponseWriter, r *http.Request, _ openapi.RegisterOauthClientParams) {
	notAvailable(w, r)
}

func (pending) DeleteOauthClient(w http.ResponseWriter, r *http.Request, _ openapi.OauthClientId) {
	notAvailable(w, r)
}

func (pending) AuthorizeOauthClient(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ExchangeOauthCode(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ListOauthGrants(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) RevokeOauthGrant(w http.ResponseWriter, r *http.Request, _ openapi.OauthGrantId) {
	notAvailable(w, r)
}

// The five credential operations of G-01. All of them are overridden by RestController; they stay
// here because the compile-time check above needs every method of the interface.
func (pending) ListAccessTokens(w http.ResponseWriter, r *http.Request, _ openapi.ListAccessTokensParams) {
	notAvailable(w, r)
}

func (pending) CreateAccessToken(w http.ResponseWriter, r *http.Request, _ openapi.CreateAccessTokenParams) {
	notAvailable(w, r)
}

func (pending) RevokeAccessToken(w http.ResponseWriter, r *http.Request, _ openapi.TokenId) {
	notAvailable(w, r)
}

func (pending) ListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

// The webhook subscriptions of G-03. All eight are overridden by RestController; they stay here
// because the compile-time check above needs every method of the interface.
func (pending) ListWebhookSubscriptions(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

func (pending) CreateWebhookSubscription(w http.ResponseWriter, r *http.Request, _ openapi.CreateWebhookSubscriptionParams) {
	notAvailable(w, r)
}

func (pending) GetWebhookSubscription(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId) {
	notAvailable(w, r)
}

func (pending) UpdateWebhookSubscription(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId, _ openapi.UpdateWebhookSubscriptionParams) {
	notAvailable(w, r)
}

func (pending) DeleteWebhookSubscription(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId) {
	notAvailable(w, r)
}

func (pending) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId, _ openapi.ListWebhookDeliveriesParams) {
	notAvailable(w, r)
}

func (pending) ReplayWebhookDelivery(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId, _ openapi.DeliveryId) {
	notAvailable(w, r)
}

func (pending) RotateWebhookSecret(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId, _ openapi.RotateWebhookSecretParams) {
	notAvailable(w, r)
}

func (pending) SendWebhook(w http.ResponseWriter, r *http.Request, _ openapi.WebhookId) {
	notAvailable(w, r)
}

//nolint:revive // the method name is oapi-codegen's, derived from the operationId.
func (pending) HttpRequest(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

// The automation rules. RestController overrides every one of them; they are here because pending
// implements the whole interface, which is what lets the contract test compare the router against
// the specification as a whole.
func (pending) ListRules(w http.ResponseWriter, r *http.Request, _ openapi.ListRulesParams) {
	notAvailable(w, r)
}

func (pending) CreateRule(w http.ResponseWriter, r *http.Request, _ openapi.CreateRuleParams) {
	notAvailable(w, r)
}

func (pending) GetRule(w http.ResponseWriter, r *http.Request, _ openapi.RuleId) {
	notAvailable(w, r)
}

func (pending) UpdateRule(w http.ResponseWriter, r *http.Request, _ openapi.RuleId) {
	notAvailable(w, r)
}

func (pending) DeleteRule(w http.ResponseWriter, r *http.Request, _ openapi.RuleId) {
	notAvailable(w, r)
}

func (pending) EnableRule(w http.ResponseWriter, r *http.Request, _ openapi.RuleId, _ openapi.EnableRuleParams) {
	notAvailable(w, r)
}

func (pending) DisableRule(w http.ResponseWriter, r *http.Request, _ openapi.RuleId, _ openapi.DisableRuleParams) {
	notAvailable(w, r)
}

func (pending) TestRule(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

// The jumble (G-10). RestController overrides them as the steps land.
func (pending) ListJumbleEntries(w http.ResponseWriter, r *http.Request, _ openapi.ListJumbleEntriesParams) {
	notAvailable(w, r)
}

func (pending) SubmitJumbleEntry(w http.ResponseWriter, r *http.Request, _ openapi.SubmitJumbleEntryParams) {
	notAvailable(w, r)
}

func (pending) ConvertJumbleEntry(
	w http.ResponseWriter, r *http.Request, _ openapi_types.UUID, _ openapi.ConvertJumbleEntryParams,
) {
	notAvailable(w, r)
}

func (pending) DismissJumbleEntry(
	w http.ResponseWriter, r *http.Request, _ openapi_types.UUID, _ openapi.DismissJumbleEntryParams,
) {
	notAvailable(w, r)
}

func (pending) RotateJumbleIntake(w http.ResponseWriter, r *http.Request, _ openapi.RotateJumbleIntakeParams) {
	notAvailable(w, r)
}

// The jumble's public door, overridden only when the composition root wires the intake - exactly
// as the inbound trigger's is.
func (pending) StartJumbleIntake(w http.ResponseWriter, r *http.Request, _ string) {
	notAvailable(w, r)
}

// The mail door beside it (G-11), and pending for the same reason: an installation that does not
// serve it tells the internet nothing about why.
func (pending) DeliverMail(w http.ResponseWriter, r *http.Request, _ string) {
	notAvailable(w, r)
}

func (pending) TriggerRuleManually(
	w http.ResponseWriter, r *http.Request, _ openapi.RuleId, _ openapi.TriggerRuleManuallyParams,
) {
	notAvailable(w, r)
}

func (pending) RotateInboundTrigger(
	w http.ResponseWriter, r *http.Request, _ openapi.RuleId, _ openapi.RotateInboundTriggerParams,
) {
	notAvailable(w, r)
}

// The public inbound route, which RestController overrides only when the composition root wires
// the starter - exactly as the calendar feed's is.
func (pending) StartInboundRun(w http.ResponseWriter, r *http.Request, _ string) {
	notAvailable(w, r)
}

// The run log. RestController overrides both; they are here because pending implements the whole
// interface, which is what lets the contract test compare the router against the specification.
func (pending) ListRuleRuns(w http.ResponseWriter, r *http.Request, _ openapi.ListRuleRunsParams) {
	notAvailable(w, r)
}

func (pending) GetRuleRun(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

func (pending) ReplayRuleRun(
	w http.ResponseWriter, r *http.Request, _ openapi_types.UUID, _ openapi.ReplayRuleRunParams,
) {
	notAvailable(w, r)
}

func (pending) PollTriggerEvents(w http.ResponseWriter, r *http.Request, _ openapi.EventType, _ openapi.PollTriggerEventsParams) {
	notAvailable(w, r)
}

func (pending) CreateServiceAccount(w http.ResponseWriter, r *http.Request, _ openapi.CreateServiceAccountParams) {
	notAvailable(w, r)
}

// ListAuditEntries is overridden by RestController; it stays here because the compile-time
// assertion is on `pending` itself.
func (pending) ListAuditEntries(w http.ResponseWriter, r *http.Request, _ openapi.ListAuditEntriesParams) {
	notAvailable(w, r)
}

// VerifyAuditChain is overridden by RestController, for the reason ListAuditEntries is.
func (pending) VerifyAuditChain(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// ExportAuditTrail is overridden by RestController, for the reason ListAuditEntries is.
func (pending) ExportAuditTrail(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// The privacy operations (E-10). Overridden by RestController where a use case serves them; they
// stay here because the compile-time assertion is on `pending` itself.
func (pending) ListDataSubjectRequests(w http.ResponseWriter, r *http.Request, _ openapi.ListDataSubjectRequestsParams) {
	notAvailable(w, r)
}

func (pending) CreateDataSubjectRequest(w http.ResponseWriter, r *http.Request, _ openapi.CreateDataSubjectRequestParams) {
	notAvailable(w, r)
}

func (pending) UpdateDataSubjectRequest(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

// RestrictProcessing and WithdrawConsent are overridden by RestController, for the reason
// ListDataSubjectRequests is.
func (pending) RestrictProcessing(w http.ResponseWriter, r *http.Request, _ openapi.AccountId) {
	notAvailable(w, r)
}

func (pending) WithdrawConsent(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// The three target operations. Overridden by RestController; they stay here because the
// compile-time assertion is on `pending` itself.
func (pending) ListBackupTargets(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) CreateBackupTarget(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) TestBackupTarget(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

// The backup run operations. Overridden by RestController; they stay here for the reason the job
// operations do - the compile-time assertion is on `pending` itself.
func (pending) ListBackupsAtTarget(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID, _ openapi.ListBackupsAtTargetParams) {
	notAvailable(w, r)
}

func (pending) CreateBackupSchedule(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) StartBackup(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) GetBackupRun(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

func (pending) VerifyBackup(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

// The two job operations. Overridden by RestController; they stay here because the compile-time
// assertion is on `pending` itself.
func (pending) GetJob(w http.ResponseWriter, r *http.Request, _ openapi.JobId) {
	notAvailable(w, r)
}

func (pending) CancelJob(w http.ResponseWriter, r *http.Request, _ openapi.JobId, _ openapi.CancelJobParams) {
	notAvailable(w, r)
}

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

func (pending) ReorderContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.ReorderContainerParams) {
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

// AssignWorkItem and UnassignWorkItem are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) AssignWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.AssignWorkItemParams) {
	notAvailable(w, r)
}

func (pending) UnassignWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.UnassignWorkItemParams) {
	notAvailable(w, r)
}

func (pending) AutoAssignWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.AutoAssignWorkItemParams) {
	notAvailable(w, r)
}

// AddMember and RemoveMember are overridden by RestController, for the reason given at
// CreateContainer.

func (pending) AddMember(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.AccountId) {
	notAvailable(w, r)
}

func (pending) RemoveMember(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.AccountId) {
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

// TrashWorkItem is overridden by RestController, for the reason given at CreateContainer.
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

// ListActivity is overridden by RestController, for the reason given at CreateContainer.
func (pending) ListActivity(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ListActivityParams) {
	notAvailable(w, r)
}

func (pending) ListComments(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ListCommentsParams) {
	notAvailable(w, r)
}

func (pending) AddComment(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.AddCommentParams) {
	notAvailable(w, r)
}

func (pending) EditComment(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.CommentId, _ openapi.EditCommentParams) {
	notAvailable(w, r)
}

func (pending) DeleteComment(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.CommentId, _ openapi.DeleteCommentParams) {
	notAvailable(w, r)
}

// The custom field operations land one by one as C-07 registers each use case.
func (pending) ListCustomFields(w http.ResponseWriter, r *http.Request, _ openapi.ListCustomFieldsParams) {
	notAvailable(w, r)
}

func (pending) DefineCustomField(w http.ResponseWriter, r *http.Request, _ openapi.DefineCustomFieldParams) {
	notAvailable(w, r)
}

func (pending) UpdateCustomField(w http.ResponseWriter, r *http.Request, _ openapi.CustomFieldId, _ openapi.UpdateCustomFieldParams) {
	notAvailable(w, r)
}

func (pending) DeleteCustomField(w http.ResponseWriter, r *http.Request, _ openapi.CustomFieldId, _ openapi.DeleteCustomFieldParams) {
	notAvailable(w, r)
}

func (pending) SetCustomField(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.CustomFieldKey, _ openapi.SetCustomFieldParams) {
	notAvailable(w, r)
}

// SetDueDate and ClearDueDate are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) SetDueDate(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.SetDueDateParams) {
	notAvailable(w, r)
}

func (pending) ClearDueDate(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ClearDueDateParams) {
	notAvailable(w, r)
}

// The media operations land one by one as C-06 registers each use case.
func (pending) SetCover(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.SetCoverParams) {
	notAvailable(w, r)
}

func (pending) ClearCover(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ClearCoverParams) {
	notAvailable(w, r)
}

func (pending) ListAttachments(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ListAttachmentsParams) {
	notAvailable(w, r)
}

func (pending) AttachMedia(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.MediaId) {
	notAvailable(w, r)
}

func (pending) DetachMedia(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.MediaId) {
	notAvailable(w, r)
}

func (pending) RequestMediaUpload(w http.ResponseWriter, r *http.Request, _ openapi.RequestMediaUploadParams) {
	notAvailable(w, r)
}

func (pending) GetMedia(w http.ResponseWriter, r *http.Request, _ openapi.MediaId) {
	notAvailable(w, r)
}

func (pending) DeleteMedia(w http.ResponseWriter, r *http.Request, _ openapi.MediaId) {
	notAvailable(w, r)
}

func (pending) ConfirmMediaUpload(w http.ResponseWriter, r *http.Request, _ openapi.MediaId, _ openapi.ConfirmMediaUploadParams) {
	notAvailable(w, r)
}

func (pending) UploadMediaContent(w http.ResponseWriter, r *http.Request, _ openapi.MediaId, _ openapi.UploadMediaContentParams) {
	notAvailable(w, r)
}

func (pending) DownloadMediaContent(w http.ResponseWriter, r *http.Request, _ openapi.MediaId, _ openapi.DownloadMediaContentParams) {
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
func (pending) MoveWorkItem(
	w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.MoveWorkItemParams,
) {
	notAvailable(w, r)
}

// ReorderWorkItem is overridden by RestController, for the reason given at CreateContainer.
func (pending) ReorderWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ReorderWorkItemParams) {
	notAvailable(w, r)
}

// The lifecycle actions are overridden by RestController, for the reason given at CreateContainer.
func (pending) RestoreWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.RestoreWorkItemParams) {
	notAvailable(w, r)
}

func (pending) RestoreContainer(w http.ResponseWriter, r *http.Request, _ openapi.ContainerId, _ openapi.RestoreContainerParams) {
	notAvailable(w, r)
}

func (pending) ListTrash(w http.ResponseWriter, r *http.Request, _ openapi.ListTrashParams) {
	notAvailable(w, r)
}

func (pending) PurgeWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.PurgeWorkItemParams) {
	notAvailable(w, r)
}

func (pending) EmptyTrash(w http.ResponseWriter, r *http.Request, _ openapi.EmptyTrashParams) {
	notAvailable(w, r)
}

// ArchiveWorkItem and UnarchiveWorkItem are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) ArchiveWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ArchiveWorkItemParams) {
	notAvailable(w, r)
}

func (pending) UnarchiveWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.UnarchiveWorkItemParams) {
	notAvailable(w, r)
}

// Overridden by RestController; it stays here for the reason the others do - the compile-time
// assertion is on `pending` itself.
func (pending) RetainItem(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

// The saved view operations are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) ListSavedViews(w http.ResponseWriter, r *http.Request, _ openapi.ListSavedViewsParams) {
	notAvailable(w, r)
}

func (pending) CreateSavedView(w http.ResponseWriter, r *http.Request, _ openapi.CreateSavedViewParams) {
	notAvailable(w, r)
}

func (pending) GetSavedView(w http.ResponseWriter, r *http.Request, _ openapi.ViewId) {
	notAvailable(w, r)
}

func (pending) UpdateSavedView(w http.ResponseWriter, r *http.Request, _ openapi.ViewId, _ openapi.UpdateSavedViewParams) {
	notAvailable(w, r)
}

func (pending) DeleteSavedView(w http.ResponseWriter, r *http.Request, _ openapi.ViewId, _ openapi.DeleteSavedViewParams) {
	notAvailable(w, r)
}

func (pending) ShareSavedView(w http.ResponseWriter, r *http.Request, _ openapi.ViewId, _ openapi.ShareSavedViewParams) {
	notAvailable(w, r)
}

// SkipOccurrence is overridden by RestController, for the reason given at CreateContainer.
func (pending) SkipOccurrence(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.SkipOccurrenceParams) {
	notAvailable(w, r)
}

// The template operations are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) ListTemplates(w http.ResponseWriter, r *http.Request, _ openapi.ListTemplatesParams) {
	notAvailable(w, r)
}

func (pending) CreateTemplate(w http.ResponseWriter, r *http.Request, _ openapi.CreateTemplateParams) {
	notAvailable(w, r)
}

func (pending) GetTemplate(w http.ResponseWriter, r *http.Request, _ openapi.TemplateId) {
	notAvailable(w, r)
}

func (pending) UpdateTemplate(w http.ResponseWriter, r *http.Request, _ openapi.TemplateId, _ openapi.UpdateTemplateParams) {
	notAvailable(w, r)
}

func (pending) DeleteTemplate(w http.ResponseWriter, r *http.Request, _ openapi.TemplateId, _ openapi.DeleteTemplateParams) {
	notAvailable(w, r)
}

func (pending) InstantiateTemplate(w http.ResponseWriter, r *http.Request, _ openapi.TemplateId, _ openapi.InstantiateTemplateParams) {
	notAvailable(w, r)
}

// The recurrence operations are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) GetRecurrence(w http.ResponseWriter, r *http.Request, _ openapi.ItemId) {
	notAvailable(w, r)
}

func (pending) SetRecurrence(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.SetRecurrenceParams) {
	notAvailable(w, r)
}

func (pending) RemoveRecurrence(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.RemoveRecurrenceParams) {
	notAvailable(w, r)
}

// The reminder operations are overridden by RestController, for the reason given at
// CreateContainer.
func (pending) ListReminders(w http.ResponseWriter, r *http.Request, _ openapi.ItemId) {
	notAvailable(w, r)
}

func (pending) CreateReminder(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.CreateReminderParams) {
	notAvailable(w, r)
}

func (pending) UpdateReminder(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ReminderId, _ openapi.UpdateReminderParams) {
	notAvailable(w, r)
}

func (pending) DeleteReminder(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.ReminderId, _ openapi.DeleteReminderParams) {
	notAvailable(w, r)
}

// QueryItems is overridden by RestController, for the reason given at CreateContainer.
func (pending) QueryItems(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// SearchItems is overridden by RestController, for the same reason.
func (pending) SearchItems(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// BulkUpdateWorkItems and DuplicateWorkItem are overridden by RestController, for the reason given
// at CreateContainer.
func (pending) BulkUpdateWorkItems(w http.ResponseWriter, r *http.Request, _ openapi.BulkUpdateWorkItemsParams) {
	notAvailable(w, r)
}

func (pending) DuplicateWorkItem(w http.ResponseWriter, r *http.Request, _ openapi.ItemId, _ openapi.DuplicateWorkItemParams) {
	notAvailable(w, r)
}

func (pending) GetCapabilities(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) GetHealthReport(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

// The restore operations. Overridden by RestController; they stay here for the reason the others
// do - the compile-time assertion is on `pending` itself.
func (pending) StartRestore(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) GetRestoreRun(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

// The legal hold operations. Overridden by RestController; they stay here for the reason the
// others do - the compile-time assertion is on `pending` itself.
func (pending) ListLegalHolds(w http.ResponseWriter, r *http.Request, _ openapi.ListLegalHoldsParams) {
	notAvailable(w, r)
}

func (pending) PlaceLegalHold(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) ReleaseLegalHold(w http.ResponseWriter, r *http.Request, _ openapi_types.UUID) {
	notAvailable(w, r)
}

// The three retention operations. Overridden by RestController; they stay here for the reason the
// others do - the compile-time assertion is on `pending` itself.
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

func (pending) StreamChanges(w http.ResponseWriter, r *http.Request, _ openapi.StreamChangesParams) {
	notAvailable(w, r)
}

// The three feed operations are overridden by RestController, for the reason given at
// CreateContainer. The fetch and the export answer as pending until the steps that serve them.
func (pending) ListCalendarFeeds(w http.ResponseWriter, r *http.Request) { notAvailable(w, r) }

func (pending) CreateCalendarFeed(w http.ResponseWriter, r *http.Request, _ openapi.CreateCalendarFeedParams) {
	notAvailable(w, r)
}

func (pending) RevokeCalendarFeed(w http.ResponseWriter, r *http.Request, _ openapi.FeedId) {
	notAvailable(w, r)
}

func (pending) GetCalendarFeedDocument(w http.ResponseWriter, r *http.Request, _ string) {
	notAvailable(w, r)
}

func (pending) ExportView(w http.ResponseWriter, r *http.Request, _ openapi.ViewId, _ openapi.ExportViewParams) {
	notAvailable(w, r)
}

// The relying party (H-04). RestController serves all five of its routes; these stand behind
// them, as they do behind every operation this installation does serve.
func (pending) StartOidcSignIn(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

func (pending) CompleteOidcSignIn(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

func (pending) ReadIdentityProvider(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

func (pending) ConfigureIdentityProvider(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}

func (pending) RemoveIdentityProvider(w http.ResponseWriter, r *http.Request) {
	notAvailable(w, r)
}
