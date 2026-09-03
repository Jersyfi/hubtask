// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package catalogue is the list of every use case this build has, once.
//
// It is not the registry. The registry is built in the composition root with real dependencies and
// is what a request runs through; this is the same list with none - `Descriptor()` reads no
// dependency of a use case, so the zero value answers what it is called, what it declares and what
// it records. That is what lets a gate and a generator ask the catalogue a question without a
// database, an HTTP server or a wiring fixture.
//
// The list is maintained by hand, and deliberately so: a list derived from the source would grow a
// use case that was written and never registered, which is exactly what the parity gate exists to
// notice. Three things keep it honest - the gate that finds a `Descriptor()` missing from here, the
// gate that finds one missing from `cmd/server`, and the generated event matrix, which is read by
// people.
//
//go:generate go run github.com/Jersyfi/hubtask/tools/eventmatrix
package catalogue

import (
	"slices"
	"strings"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/integration"
	jobservice "github.com/Jersyfi/hubtask/core/application/service/job"
	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// Descriptors is every use case, in the order the composition root registers them.
func Descriptors() []usecase.Descriptor {
	return []usecase.Descriptor{
		work.CreateContainer{}.Descriptor(),
		work.CreateWorkItem{}.Descriptor(),
		work.UpdateWorkItem{}.Descriptor(),
		work.RenameContainer{}.Descriptor(),
		work.UpdateContainerPolicies{}.Descriptor(),
		work.ArchiveContainer{}.Descriptor(),
		work.UnarchiveContainer{}.Descriptor(),
		work.MoveContainer{}.Descriptor(),
		work.ReorderContainer{}.Descriptor(),
		work.TrashContainer{}.Descriptor(),
		work.RestoreContainer{}.Descriptor(),
		work.CreateBucket{}.Descriptor(),
		work.ListBuckets{}.Descriptor(),
		work.UpdateBucket{}.Descriptor(),
		work.ReorderBucket{}.Descriptor(),
		work.DeleteBucket{}.Descriptor(),
		work.CreateLabel{}.Descriptor(),
		work.ListLabels{}.Descriptor(),
		work.UpdateLabel{}.Descriptor(),
		work.DeleteLabel{}.Descriptor(),
		work.AddLabel{}.Descriptor(),
		work.RemoveLabel{}.Descriptor(),
		work.AssignWorkItem{}.Descriptor(),
		work.UnassignWorkItem{}.Descriptor(),
		work.AutoAssignWorkItem{}.Descriptor(),
		work.AddMember{}.Descriptor(),
		work.RemoveMember{}.Descriptor(),
		work.AddComment{}.Descriptor(),
		work.ListComments{}.Descriptor(),
		work.EditComment{}.Descriptor(),
		work.DeleteComment{}.Descriptor(),
		work.GetContainer{}.Descriptor(),
		work.ListContainers{}.Descriptor(),
		work.GetWorkItem{}.Descriptor(),
		work.ListWorkItems{}.Descriptor(),
		work.QueryItems{}.Descriptor(),
		work.SearchItems{}.Descriptor(),
		work.ListActivity{}.Descriptor(),
		work.CompleteWorkItem{}.Descriptor(),
		work.ReopenWorkItem{}.Descriptor(),
		work.MoveWorkItem{}.Descriptor(),
		work.DuplicateWorkItem{}.Descriptor(),
		work.BulkUpdateWorkItems{}.Descriptor(),
		work.ReorderWorkItem{}.Descriptor(),
		work.ArchiveWorkItem{}.Descriptor(),
		work.UnarchiveWorkItem{}.Descriptor(),
		work.TrashWorkItem{}.Descriptor(),
		work.RestoreWorkItem{}.Descriptor(),
		work.ListTrash{}.Descriptor(),
		lifecycle.PurgeWorkItem{}.Descriptor(),
		lifecycle.EmptyTrash{}.Descriptor(),
		mediaservice.RequestMediaUpload{}.Descriptor(),
		mediaservice.ConfirmMediaUpload{}.Descriptor(),
		work.SetCover{}.Descriptor(),
		work.ClearCover{}.Descriptor(),
		work.SetDueDate{}.Descriptor(),
		work.ClearDueDate{}.Descriptor(),
		work.CreateReminder{}.Descriptor(),
		work.ListReminders{}.Descriptor(),
		work.UpdateReminder{}.Descriptor(),
		work.DeleteReminder{}.Descriptor(),
		work.SetRecurrence{}.Descriptor(),
		work.RemoveRecurrence{}.Descriptor(),
		work.SkipOccurrence{}.Descriptor(),
		work.CreateTemplate{}.Descriptor(),
		work.ListTemplates{}.Descriptor(),
		work.GetTemplate{}.Descriptor(),
		work.UpdateTemplate{}.Descriptor(),
		work.DeleteTemplate{}.Descriptor(),
		work.InstantiateTemplate{}.Descriptor(),
		work.GetRecurrence{}.Descriptor(),
		work.AttachMedia{}.Descriptor(),
		work.DefineCustomField{}.Descriptor(),
		work.ListCustomFields{}.Descriptor(),
		work.UpdateCustomField{}.Descriptor(),
		work.DeleteCustomField{}.Descriptor(),
		work.ExportView{}.Descriptor(),
		work.CreateCalendarFeed{}.Descriptor(),
		work.ListCalendarFeeds{}.Descriptor(),
		work.RevokeCalendarFeed{}.Descriptor(),
		work.CreateSavedView{}.Descriptor(),
		work.ListSavedViews{}.Descriptor(),
		work.GetSavedView{}.Descriptor(),
		work.UpdateSavedView{}.Descriptor(),
		work.DeleteSavedView{}.Descriptor(),
		work.ShareSavedView{}.Descriptor(),
		work.SetCustomField{}.Descriptor(),
		work.DetachMedia{}.Descriptor(),
		mediaservice.ListAttachments{}.Descriptor(),
		mediaservice.GetMedia{}.Descriptor(),
		mediaservice.DeleteMedia{}.Descriptor(),
		backupservice.CreateBackupTarget{}.Descriptor(),
		backupservice.ListBackupTargets{}.Descriptor(),
		backupservice.TestBackupTarget{}.Descriptor(),
		backupservice.StartBackup{}.Descriptor(),
		backupservice.GetBackupRun{}.Descriptor(),
		backupservice.VerifyBackup{}.Descriptor(),
		backupservice.ListBackupsAtTarget{}.Descriptor(),
		backupservice.StartRestore{}.Descriptor(),
		backupservice.GetRestoreRun{}.Descriptor(),
		lifecycle.CreateRetentionPolicy{}.Descriptor(),
		lifecycle.ListRetentionPolicies{}.Descriptor(),
		lifecycle.PreviewRetentionPolicy{}.Descriptor(),
		lifecycle.RetainItem{}.Descriptor(),
		lifecycle.PlaceLegalHold{}.Descriptor(),
		lifecycle.ReleaseLegalHold{}.Descriptor(),
		lifecycle.ListLegalHolds{}.Descriptor(),
		auditservice.ListAuditEntries{}.Descriptor(),
		auditservice.VerifyAuditChain{}.Descriptor(),
		auditservice.ExportAuditTrail{}.Descriptor(),
		privacyservice.CreateDataSubjectRequest{}.Descriptor(),
		privacyservice.ListDataSubjectRequests{}.Descriptor(),
		privacyservice.UpdateDataSubjectRequest{}.Descriptor(),
		privacyservice.RestrictProcessing{}.Descriptor(),
		privacyservice.WithdrawConsent{}.Descriptor(),
		backupservice.CreateBackupSchedule{}.Descriptor(),
		jobservice.GetJob{}.Descriptor(),
		jobservice.CancelJob{}.Descriptor(),
		identity.InviteAccount{}.Descriptor(),
		identity.GetOwnAccount{}.Descriptor(),
		identity.UpdateAccountPreferences{}.Descriptor(),
		identity.GrantMembership{}.Descriptor(),
		identity.RevokeMembership{}.Descriptor(),
		identity.CreateGroup{}.Descriptor(),
		identity.UpdateGroup{}.Descriptor(),
		identity.DeleteGroup{}.Descriptor(),
		identity.SignIn{}.Descriptor(),
		identity.RefreshSession{}.Descriptor(),
		identity.ListSessions{}.Descriptor(),
		identity.RevokeSession{}.Descriptor(),
		identity.RevokeAllSessions{}.Descriptor(),
		identity.RedeemInvitation{}.Descriptor(),
		identity.CompleteSignIn{}.Descriptor(),
		identity.EnrollTotp{}.Descriptor(),
		identity.ConfirmTotp{}.Descriptor(),
		identity.DisableTotp{}.Descriptor(),
		identity.StepUp{}.Descriptor(),
		identity.RegisterOauthClient{}.Descriptor(),
		identity.ListOauthClients{}.Descriptor(),
		identity.DeleteOauthClient{}.Descriptor(),
		identity.AuthorizeOauthClient{}.Descriptor(),
		identity.ExchangeOauthCode{}.Descriptor(),
		identity.ListOauthGrants{}.Descriptor(),
		identity.RevokeOauthGrant{}.Descriptor(),
		identity.ConfigureIdentityProvider{}.Descriptor(),
		identity.ReadIdentityProvider{}.Descriptor(),
		identity.RemoveIdentityProvider{}.Descriptor(),
		identity.StartOidcSignIn{}.Descriptor(),
		identity.CompleteOidcSignIn{}.Descriptor(),
		identity.CreateAccessToken{}.Descriptor(),
		identity.ListAccessTokens{}.Descriptor(),
		identity.RevokeAccessToken{}.Descriptor(),
		identity.CreateServiceAccount{}.Descriptor(),
		identity.ListServiceAccounts{}.Descriptor(),
		integration.CreateWebhookSubscription{}.Descriptor(),
		integration.GetWebhookSubscription{}.Descriptor(),
		integration.ListWebhookSubscriptions{}.Descriptor(),
		integration.UpdateWebhookSubscription{}.Descriptor(),
		integration.DeleteWebhookSubscription{}.Descriptor(),
		integration.ListWebhookDeliveries{}.Descriptor(),
		integration.ReplayWebhookDelivery{}.Descriptor(),
		integration.SendWebhook{}.Descriptor(),
		integration.RotateWebhookSecret{}.Descriptor(),
		integration.PollTriggerEvents{}.Descriptor(),
		automationservice.CreateRule{}.Descriptor(),
		automationservice.GetRule{}.Descriptor(),
		automationservice.ListRules{}.Descriptor(),
		automationservice.UpdateRule{}.Descriptor(),
		automationservice.EnableRule{}.Descriptor(),
		automationservice.DisableRule{}.Descriptor(),
		automationservice.DeleteRule{}.Descriptor(),
		automationservice.TriggerRuleManually{}.Descriptor(),
		automationservice.RotateInboundTrigger{}.Descriptor(),
		automationservice.ListRuleRuns{}.Descriptor(),
		automationservice.GetRuleRun{}.Descriptor(),
		automationservice.HttpRequest{}.Descriptor(),
		automationservice.TestRule{}.Descriptor(),
		automationservice.ReplayRuleRun{}.Descriptor(),
		jumbleservice.SubmitJumbleEntry{}.Descriptor(),
		jumbleservice.ListJumbleEntries{}.Descriptor(),
		jumbleservice.ConvertJumbleEntry{}.Descriptor(),
		jumbleservice.DismissJumbleEntry{}.Descriptor(),
		jumbleservice.RotateJumbleIntake{}.Descriptor(),
		adminservice.ProvisionTenant{}.Descriptor(),
		adminservice.ListTenants{}.Descriptor(),
		adminservice.SuspendTenant{}.Descriptor(),
		adminservice.ResumeTenant{}.Descriptor(),
		adminservice.RequestTenantDeletion{}.Descriptor(),
		adminservice.ExportTenant{}.Descriptor(),
		adminservice.UpdateTenantQuotas{}.Descriptor(),
		quotaservice.ReadQuotas{}.Descriptor(),
	}
}

// Scopes is every token scope this build declares, sorted and without repeats.
//
// Derived from the descriptors rather than written down, because a list somebody maintains beside
// them is a list that grows a scope no operation checks - and a scope no operation checks is a
// bound a token's holder believes in and nothing applies. It is what CreateAccessToken validates
// a request against, handed to it by the composition root: a use case cannot read the catalogue
// it is itself part of (ADR-0001).
func Scopes() []string {
	seen := make(map[string]bool)
	scopes := make([]string, 0, len(Descriptors()))
	for _, descriptor := range Descriptors() {
		if descriptor.TokenScope == "" || seen[descriptor.TokenScope] {
			continue
		}
		seen[descriptor.TokenScope] = true
		scopes = append(scopes, descriptor.TokenScope)
	}
	slices.Sort(scopes)
	return scopes
}

// SessionScopes is what a session-authenticated person may exercise: every declared scope except
// the control plane's. A session is the person themselves - but the admin surface is entered by
// a deliberately minted credential, never by whoever happens to be signed in (H-06, 0.6.0
// decision 6), so the one scope class sessions never carry is `admin:*`.
func SessionScopes() []string {
	scopes := make([]string, 0, len(Scopes()))
	for _, scope := range Scopes() {
		if strings.HasPrefix(scope, "admin:") {
			continue
		}
		scopes = append(scopes, scope)
	}
	return scopes
}
