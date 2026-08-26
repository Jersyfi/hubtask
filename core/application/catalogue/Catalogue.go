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
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	jobservice "github.com/Jersyfi/hubtask/core/application/service/job"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
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
		identity.UpdateAccountPreferences{}.Descriptor(),
		identity.GrantMembership{}.Descriptor(),
		identity.RevokeMembership{}.Descriptor(),
		identity.CreateGroup{}.Descriptor(),
		identity.UpdateGroup{}.Descriptor(),
		identity.DeleteGroup{}.Descriptor(),
	}
}
