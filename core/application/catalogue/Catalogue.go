// SPDX-License-Identifier: Apache-2.0
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
	agentservice "github.com/Jersyfi/hubtask/core/application/service/agent"
	"slices"
	"strings"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	auditservice "github.com/Jersyfi/hubtask/core/application/service/audit"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	calendarpushservice "github.com/Jersyfi/hubtask/core/application/service/calendarpush"
	chatbotservice "github.com/Jersyfi/hubtask/core/application/service/chatbot"
	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
	configservice "github.com/Jersyfi/hubtask/core/application/service/config"
	filelinkservice "github.com/Jersyfi/hubtask/core/application/service/filelink"
	filestoreservice "github.com/Jersyfi/hubtask/core/application/service/filestore"
	forgeservice "github.com/Jersyfi/hubtask/core/application/service/forge"
	goalservice "github.com/Jersyfi/hubtask/core/application/service/goal"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	importservice "github.com/Jersyfi/hubtask/core/application/service/importer"
	"github.com/Jersyfi/hubtask/core/application/service/integration"
	jobservice "github.com/Jersyfi/hubtask/core/application/service/job"
	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	knowledgeservice "github.com/Jersyfi/hubtask/core/application/service/knowledge"
	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	mailservice "github.com/Jersyfi/hubtask/core/application/service/mail"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	metaservice "github.com/Jersyfi/hubtask/core/application/service/meta"
	planningservice "github.com/Jersyfi/hubtask/core/application/service/planning"
	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	sealingservice "github.com/Jersyfi/hubtask/core/application/service/sealing"
	servicedeskservice "github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	suggestionservice "github.com/Jersyfi/hubtask/core/application/service/suggestion"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	timeservice "github.com/Jersyfi/hubtask/core/application/service/timekeeping"
	voiceservice "github.com/Jersyfi/hubtask/core/application/service/voice"
	whiteboardservice "github.com/Jersyfi/hubtask/core/application/service/whiteboard"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	workflowservice "github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
)

// Descriptors is every use case, in the order the composition root registers them.
func Descriptors() []usecase.Descriptor {
	return []usecase.Descriptor{
		settingservice.ListModules{}.Descriptor(),
		settingservice.SetModule{}.Descriptor(),
		settingservice.PreviewModuleReach{}.Descriptor(),
		settingservice.ReadSettingsCatalogue{}.Descriptor(),
		configservice.ReadConfigSchema{}.Descriptor(),
		configservice.ExportConfig{}.Descriptor(),
		configservice.ExplainConfig{}.Descriptor(),
		configservice.CheckConfig{}.Descriptor(),
		configservice.ValidateConfig{}.Descriptor(),
		configservice.PlanConfig{}.Descriptor(),
		configservice.ProposeConfig{}.Descriptor(),
		configservice.ApplyConfig{}.Descriptor(),
		configservice.ListConfigVersions{}.Descriptor(),
		configservice.ReadConfigVersion{}.Descriptor(),
		configservice.ListConfigProposals{}.Descriptor(),
		configservice.DecideConfigProposal{}.Descriptor(),
		configservice.SetConfigPolicy{}.Descriptor(),
		configservice.ListConfigTemplates{}.Descriptor(),
		configservice.PublishConfigTemplate{}.Descriptor(),
		configservice.PlanConfigTemplate{}.Descriptor(),
		configservice.ReadConfigSandbox{}.Descriptor(),
		configservice.WriteConfigSandbox{}.Descriptor(),
		workflowservice.ListEntryTypes{}.Descriptor(),
		workflowservice.CreateEntryType{}.Descriptor(),
		workflowservice.ChangeEntryType{}.Descriptor(),
		workflowservice.DeleteEntryType{}.Descriptor(),
		workflowservice.SetEntryTypeForm{}.Descriptor(),
		work.EvaluateEntryForm{}.Descriptor(),
		knowledgeservice.CreatePage{}.Descriptor(),
		knowledgeservice.ReadPage{}.Descriptor(),
		knowledgeservice.ListPages{}.Descriptor(),
		knowledgeservice.UpdatePage{}.Descriptor(),
		knowledgeservice.MovePage{}.Descriptor(),
		knowledgeservice.TrashPage{}.Descriptor(),
		knowledgeservice.RestorePage{}.Descriptor(),
		knowledgeservice.ListTrashedPages{}.Descriptor(),
		knowledgeservice.VerifyPage{}.Descriptor(),
		knowledgeservice.ListPageVersions{}.Descriptor(),
		knowledgeservice.ReadPageVersion{}.Descriptor(),
		knowledgeservice.NamePageVersion{}.Descriptor(),
		knowledgeservice.RestorePageVersion{}.Descriptor(),
		knowledgeservice.AddPageComment{}.Descriptor(),
		knowledgeservice.ListPageComments{}.Descriptor(),
		knowledgeservice.ChangePageComment{}.Descriptor(),
		knowledgeservice.DeletePageComment{}.Descriptor(),
		knowledgeservice.SearchPages{}.Descriptor(),
		knowledgeservice.ListPageBacklinks{}.Descriptor(),
		knowledgeservice.ExportPage{}.Descriptor(),
		knowledgeservice.RestrictPage{}.Descriptor(),
		knowledgeservice.ReadPageAccess{}.Descriptor(),
		knowledgeservice.SharePage{}.Descriptor(),
		knowledgeservice.UnsharePage{}.Descriptor(),
		knowledgeservice.CreatePagePublicLink{}.Descriptor(),
		knowledgeservice.DeletePagePublicLink{}.Descriptor(),
		knowledgeservice.ReadPublicLinksPolicy{}.Descriptor(),
		knowledgeservice.SetPublicLinksPolicy{}.Descriptor(),
		knowledgeservice.PublishPageToPortal{}.Descriptor(),
		knowledgeservice.UnpublishPageFromPortal{}.Descriptor(),
		knowledgeservice.SuggestArticles{}.Descriptor(),
		knowledgeservice.ReadArticle{}.Descriptor(),
		knowledgeservice.RateArticle{}.Descriptor(),
		knowledgeservice.ListSharedPages{}.Descriptor(),
		knowledgeservice.ReadPersonalSpace{}.Descriptor(),
		knowledgeservice.OpenDailyNote{}.Descriptor(),
		knowledgeservice.AttachPageMedia{}.Descriptor(),
		whiteboardservice.CreateWhiteboard{}.Descriptor(),
		whiteboardservice.ReadWhiteboard{}.Descriptor(),
		whiteboardservice.ListWhiteboards{}.Descriptor(),
		whiteboardservice.UpdateWhiteboard{}.Descriptor(),
		whiteboardservice.TrashWhiteboard{}.Descriptor(),
		whiteboardservice.RestoreWhiteboard{}.Descriptor(),
		whiteboardservice.ExportWhiteboard{}.Descriptor(),
		whiteboardservice.ListWhiteboardVersions{}.Descriptor(),
		whiteboardservice.ReadWhiteboardVersion{}.Descriptor(),
		whiteboardservice.NameWhiteboardVersion{}.Descriptor(),
		whiteboardservice.AddWhiteboardComment{}.Descriptor(),
		whiteboardservice.ListWhiteboardComments{}.Descriptor(),
		whiteboardservice.ChangeWhiteboardComment{}.Descriptor(),
		whiteboardservice.DeleteWhiteboardComment{}.Descriptor(),
		whiteboardservice.ListWhiteboardBacklinks{}.Descriptor(),
		whiteboardservice.AttachWhiteboardMedia{}.Descriptor(),
		servicedeskservice.ListBusinessCalendars{}.Descriptor(),
		servicedeskservice.DefineBusinessCalendar{}.Descriptor(),
		servicedeskservice.DeleteBusinessCalendar{}.Descriptor(),
		servicedeskservice.ListHubCalendars{}.Descriptor(),
		servicedeskservice.DefineHubCalendar{}.Descriptor(),
		servicedeskservice.DeleteHubCalendar{}.Descriptor(),
		servicedeskservice.ListSlaPolicies{}.Descriptor(),
		servicedeskservice.DefineSlaPolicy{}.Descriptor(),
		servicedeskservice.DeleteSlaPolicy{}.Descriptor(),
		servicedeskservice.ReadEntrySla{}.Descriptor(),
		servicedeskservice.ListReplyTemplates{}.Descriptor(),
		servicedeskservice.DefineReplyTemplate{}.Descriptor(),
		servicedeskservice.DeleteReplyTemplate{}.Descriptor(),
		servicedeskservice.RenderReplyTemplate{}.Descriptor(),
		servicedeskservice.ReportSla{}.Descriptor(),
		servicedeskservice.ListRequestCatalogue{}.Descriptor(),
		servicedeskservice.RaiseRequest{}.Descriptor(),
		servicedeskservice.ListMyRequests{}.Descriptor(),
		servicedeskservice.ListRequesterOrganisations{}.Descriptor(),
		servicedeskservice.DefineRequesterOrganisation{}.Descriptor(),
		servicedeskservice.DeleteRequesterOrganisation{}.Descriptor(),
		servicedeskservice.ReadMyRequest{}.Descriptor(),
		servicedeskservice.CommentOnMyRequest{}.Descriptor(),
		servicedeskservice.RateMyRequest{}.Descriptor(),
		servicedeskservice.DecideApproval{}.Descriptor(),
		servicedeskservice.ListMyApprovals{}.Descriptor(),
		servicedeskservice.ReadEntryApprovals{}.Descriptor(),
		servicedeskservice.SetApprovalDelegation{}.Descriptor(),
		servicedeskservice.ReadApprovalDelegation{}.Descriptor(),
		servicedeskservice.EndApprovalDelegation{}.Descriptor(),
		planningservice.ReadPlanSchedule{}.Descriptor(),
		planningservice.ApplyPlanSchedule{}.Descriptor(),
		planningservice.SetDependencyTiming{}.Descriptor(),
		planningservice.SetEntryScheduling{}.Descriptor(),
		planningservice.TakeBaseline{}.Descriptor(),
		planningservice.ListBaselines{}.Descriptor(),
		planningservice.CompareBaseline{}.Descriptor(),
		planningservice.DeleteBaseline{}.Descriptor(),
		planningservice.ReadPlanTolerance{}.Descriptor(),
		planningservice.SetPlanTolerance{}.Descriptor(),
		planningservice.ReadWorkspaceTolerance{}.Descriptor(),
		planningservice.SetWorkspaceTolerance{}.Descriptor(),
		planningservice.ImportPlan{}.Descriptor(),
		planningservice.CreateRelease{}.Descriptor(),
		planningservice.UpdateRelease{}.Descriptor(),
		planningservice.ListReleases{}.Descriptor(),
		planningservice.ReadRelease{}.Descriptor(),
		planningservice.DeleteRelease{}.Descriptor(),
		planningservice.AddReleaseEntries{}.Descriptor(),
		planningservice.RemoveReleaseEntries{}.Descriptor(),
		planningservice.PublishRelease{}.Descriptor(),
		planningservice.CreateSprint{}.Descriptor(),
		planningservice.UpdateSprint{}.Descriptor(),
		planningservice.ListSprints{}.Descriptor(),
		planningservice.ReadSprint{}.Descriptor(),
		planningservice.DeleteSprint{}.Descriptor(),
		planningservice.AddSprintEntries{}.Descriptor(),
		planningservice.RemoveSprintEntries{}.Descriptor(),
		planningservice.StartSprint{}.Descriptor(),
		planningservice.CloseSprint{}.Descriptor(),
		planningservice.ReadSprintBacklog{}.Descriptor(),
		planningservice.ReadVelocity{}.Descriptor(),
		planningservice.SetEntryEstimate{}.Descriptor(),
		planningservice.ClearEntryEstimate{}.Descriptor(),
		timeservice.LogTime{}.Descriptor(),
		timeservice.UpdateTimeRecord{}.Descriptor(),
		timeservice.DeleteTimeRecord{}.Descriptor(),
		timeservice.CorrectTimeRecord{}.Descriptor(),
		timeservice.ListTimeRecords{}.Descriptor(),
		timeservice.StartTimer{}.Descriptor(),
		timeservice.StopTimer{}.Descriptor(),
		timeservice.DiscardTimer{}.Descriptor(),
		timeservice.ReadTimer{}.Descriptor(),
		timeservice.ReadTimesheet{}.Descriptor(),
		timeservice.CopyTimesheetWeek{}.Descriptor(),
		timeservice.SubmitTimesheet{}.Descriptor(),
		timeservice.WithdrawTimesheet{}.Descriptor(),
		timeservice.ApproveTimesheet{}.Descriptor(),
		timeservice.RejectTimesheet{}.Descriptor(),
		timeservice.ListTimesheets{}.Descriptor(),
		timeservice.ListWorkTypes{}.Descriptor(),
		timeservice.DefineWorkType{}.Descriptor(),
		timeservice.ReadTimePolicy{}.Descriptor(),
		timeservice.SetTimePolicy{}.Descriptor(),
		timeservice.ReadTimeReport{}.Descriptor(),
		timeservice.ReadEstimatesVsActuals{}.Descriptor(),
		timeservice.ExportTimeRecords{}.Descriptor(),
		timeservice.ListCapacities{}.Descriptor(),
		timeservice.SetCapacity{}.Descriptor(),
		timeservice.RemoveCapacity{}.Descriptor(),
		timeservice.ListAbsences{}.Descriptor(),
		timeservice.RecordAbsence{}.Descriptor(),
		timeservice.UpdateAbsence{}.Descriptor(),
		timeservice.RemoveAbsence{}.Descriptor(),
		timeservice.ReadWorkload{}.Descriptor(),
		timeservice.ListCostRates{}.Descriptor(),
		timeservice.SetCostRate{}.Descriptor(),
		timeservice.RemoveCostRate{}.Descriptor(),
		timeservice.ListBillingRates{}.Descriptor(),
		timeservice.SetBillingRate{}.Descriptor(),
		timeservice.RemoveBillingRate{}.Descriptor(),
		timeservice.ReadBudget{}.Descriptor(),
		timeservice.SetBudget{}.Descriptor(),
		timeservice.RemoveBudget{}.Descriptor(),
		goalservice.ListGoalPeriods{}.Descriptor(),
		goalservice.DefineGoalPeriod{}.Descriptor(),
		goalservice.CloseGoalPeriod{}.Descriptor(),
		goalservice.ListGoals{}.Descriptor(),
		goalservice.ReadGoal{}.Descriptor(),
		goalservice.CreateGoal{}.Descriptor(),
		goalservice.UpdateGoal{}.Descriptor(),
		goalservice.DeleteGoal{}.Descriptor(),
		goalservice.LinkGoalEntries{}.Descriptor(),
		goalservice.UnlinkGoalEntry{}.Descriptor(),
		goalservice.CheckInGoal{}.Descriptor(),
		goalservice.ListGoalCheckIns{}.Descriptor(),
		goalservice.ListEntryGoals{}.Descriptor(),
		goalservice.ListPortfolios{}.Descriptor(),
		goalservice.ReadPortfolio{}.Descriptor(),
		goalservice.ReadPortfolioRoadmap{}.Descriptor(),
		goalservice.CreatePortfolio{}.Descriptor(),
		goalservice.UpdatePortfolio{}.Descriptor(),
		goalservice.DeletePortfolio{}.Descriptor(),
		goalservice.ReportPortfolioStatus{}.Descriptor(),
		goalservice.ListPortfolioUpdates{}.Descriptor(),
		goalservice.ListDashboards{}.Descriptor(),
		goalservice.ReadDashboard{}.Descriptor(),
		goalservice.CreateDashboard{}.Descriptor(),
		goalservice.UpdateDashboard{}.Descriptor(),
		goalservice.DeleteDashboard{}.Descriptor(),
		goalservice.ReadDashboardFigures{}.Descriptor(),
		goalservice.ReadDashboardDigest{}.Descriptor(),
		agentservice.RegisterAgentModel{}.Descriptor(),
		agentservice.ListAgentModels{}.Descriptor(),
		agentservice.ChangeAgentModel{}.Descriptor(),
		agentservice.RegisterAgent{}.Descriptor(),
		agentservice.ListAgents{}.Descriptor(),
		agentservice.ReadAgent{}.Descriptor(),
		agentservice.ChangeAgent{}.Descriptor(),
		agentservice.ApproveAgent{}.Descriptor(),
		agentservice.ActivateAgent{}.Descriptor(),
		agentservice.SuspendAgent{}.Descriptor(),
		agentservice.RetireAgent{}.Descriptor(),
		agentservice.SetAgentHalt{}.Descriptor(),
		agentservice.ReadAgentPolicy{}.Descriptor(),
		agentservice.SetAgentPolicy{}.Descriptor(),
		agentservice.ReadAgentHalt{}.Descriptor(),
		agentservice.ListAgentRequests{}.Descriptor(),
		agentservice.DecideAgentRequest{}.Descriptor(),
		agentservice.ListAgentActions{}.Descriptor(),
		agentservice.ReadAgentMetering{}.Descriptor(),
		agentservice.AddAgentCase{}.Descriptor(),
		agentservice.ListAgentCases{}.Descriptor(),
		agentservice.RemoveAgentCase{}.Descriptor(),
		agentservice.RecordAgentEvaluation{}.Descriptor(),
		agentservice.RunAgentEvaluation{}.Descriptor(),
		agentservice.ListAgentEvaluations{}.Descriptor(),
		agentservice.StartAgentJob{}.Descriptor(),
		agentservice.ListAgentJobs{}.Descriptor(),
		agentservice.ListValueProvenance{}.Descriptor(),
		goalservice.SetDashboardDigest{}.Descriptor(),
		goalservice.DeleteDashboardDigest{}.Descriptor(),
		cmdbservice.ListCiClasses{}.Descriptor(),
		cmdbservice.DefineCiClass{}.Descriptor(),
		cmdbservice.ListCiRelationTypes{}.Descriptor(),
		cmdbservice.DefineCiRelationType{}.Descriptor(),
		cmdbservice.ListCiSources{}.Descriptor(),
		cmdbservice.DefineCiSource{}.Descriptor(),
		cmdbservice.ListCis{}.Descriptor(),
		cmdbservice.ReadCi{}.Descriptor(),
		cmdbservice.CreateCi{}.Descriptor(),
		cmdbservice.UpdateCi{}.Descriptor(),
		cmdbservice.SetCiCompliance{}.Descriptor(),
		cmdbservice.RelateCis{}.Descriptor(),
		cmdbservice.UnrelateCis{}.Descriptor(),
		cmdbservice.ReconcileCis{}.Descriptor(),
		cmdbservice.ImportCiFile{}.Descriptor(),
		cmdbservice.ListCiRuns{}.Descriptor(),
		cmdbservice.ReadCiRun{}.Descriptor(),
		cmdbservice.ListCiHolds{}.Descriptor(),
		cmdbservice.DismissCiHold{}.Descriptor(),
		cmdbservice.ListCiDuplicates{}.Descriptor(),
		cmdbservice.ResolveCiDuplicate{}.Descriptor(),
		cmdbservice.ReadCmdbHealth{}.Descriptor(),
		cmdbservice.ReadCiImpact{}.Descriptor(),
		cmdbservice.ListEntryCis{}.Descriptor(),
		cmdbservice.AffectEntryCi{}.Descriptor(),
		cmdbservice.UnaffectEntryCi{}.Descriptor(),
		cmdbservice.ListCiCampaigns{}.Descriptor(),
		cmdbservice.StartCiCampaign{}.Descriptor(),
		cmdbservice.ReadCiCampaign{}.Descriptor(),
		cmdbservice.AttestCi{}.Descriptor(),
		cmdbservice.CloseCiCampaign{}.Descriptor(),
		cmdbservice.ListCiGroups{}.Descriptor(),
		cmdbservice.DefineCiGroup{}.Descriptor(),
		cmdbservice.ListCiGroupMembers{}.Descriptor(),
		cmdbservice.ReadServiceMap{}.Descriptor(),
		cmdbservice.ListCiAssets{}.Descriptor(),
		cmdbservice.ReadCiAsset{}.Descriptor(),
		cmdbservice.RegisterCiAsset{}.Descriptor(),
		cmdbservice.MoveCiAsset{}.Descriptor(),
		cmdbservice.ListSoftwareAliases{}.Descriptor(),
		cmdbservice.DefineSoftwareAlias{}.Descriptor(),
		cmdbservice.RemoveSoftwareAlias{}.Descriptor(),
		cmdbservice.ListSoftwareProducts{}.Descriptor(),
		forgeservice.ListCodeHooks{}.Descriptor(),
		forgeservice.CreateCodeHook{}.Descriptor(),
		forgeservice.UpdateCodeHook{}.Descriptor(),
		forgeservice.RotateCodeHookSecret{}.Descriptor(),
		forgeservice.ListEntryCodeLinks{}.Descriptor(),
		filelinkservice.ListEntryFileLinks{}.Descriptor(),
		filelinkservice.LinkEntryFile{}.Descriptor(),
		filelinkservice.UnlinkEntryFile{}.Descriptor(),
		calendarpushservice.ListCalendarPushes{}.Descriptor(),
		calendarpushservice.AuthorizeCalendarPush{}.Descriptor(),
		calendarpushservice.CompleteCalendarPushAuthorization{}.Descriptor(),
		calendarpushservice.DisconnectCalendarPush{}.Descriptor(),
		filestoreservice.ListFileStoreConnections{}.Descriptor(),
		chatbotservice.ListChatBots{}.Descriptor(),
		chatbotservice.CreateChatBot{}.Descriptor(),
		chatbotservice.UpdateChatBot{}.Descriptor(),
		chatbotservice.DeleteChatBot{}.Descriptor(),
		filestoreservice.ConnectFileStore{}.Descriptor(),
		filestoreservice.CompleteFileStoreConnection{}.Descriptor(),
		filestoreservice.DisconnectFileStore{}.Descriptor(),
		filestoreservice.BrowseFileStore{}.Descriptor(),
		filestoreservice.ShareFileStoreItem{}.Descriptor(),
		cmdbservice.ListCiConnectors{}.Descriptor(),
		cmdbservice.DefineCiConnector{}.Descriptor(),
		cmdbservice.SetCiConnectorCredential{}.Descriptor(),
		cmdbservice.RunCiConnector{}.Descriptor(),
		cmdbservice.ListCiAlerts{}.Descriptor(),
		cmdbservice.ListCiBacklinks{}.Descriptor(),
		work.ListLinkTypes{}.Descriptor(),
		work.CreateLinkType{}.Descriptor(),
		work.ChangeLinkType{}.Descriptor(),
		work.DeleteLinkType{}.Descriptor(),
		work.LinkEntries{}.Descriptor(),
		work.UnlinkEntries{}.Descriptor(),
		work.ListEntryLinks{}.Descriptor(),
		work.ListLinksAmong{}.Descriptor(),
		work.ResolveReferences{}.Descriptor(),
		workflowservice.ListWorkflows{}.Descriptor(),
		workflowservice.CreateWorkflow{}.Descriptor(),
		workflowservice.SaveWorkflowDraft{}.Descriptor(),
		workflowservice.PreviewWorkflowPublish{}.Descriptor(),
		workflowservice.PublishWorkflow{}.Descriptor(),
		workflowservice.DeleteWorkflow{}.Descriptor(),
		workflowservice.PreviewWorkflowScheme{}.Descriptor(),
		workflowservice.SetWorkflowScheme{}.Descriptor(),
		workflowservice.TryWorkflowStep{}.Descriptor(),
		work.TakeStep{}.Descriptor(),
		work.ListEntrySteps{}.Descriptor(),
		work.SetItemEntryType{}.Descriptor(),
		work.CreateContainer{}.Descriptor(),
		work.CreateWorkItem{}.Descriptor(),
		work.UpdateWorkItem{}.Descriptor(),
		work.RenameContainer{}.Descriptor(),
		work.UpdateContainerPolicies{}.Descriptor(),
		work.SetContainerStructure{}.Descriptor(),
		work.PreviewContainerStructure{}.Descriptor(),
		work.SetWorkspaceStructure{}.Descriptor(),
		work.PreviewWorkspaceStructure{}.Descriptor(),
		work.SavePreset{}.Descriptor(),
		work.ChangePreset{}.Descriptor(),
		work.WithdrawPreset{}.Descriptor(),
		work.CloseStarter{}.Descriptor(),
		work.SetHubPrivacy{}.Descriptor(),
		work.ListPrivateHubs{}.Descriptor(),
		work.OpenPrivateHub{}.Descriptor(),
		work.ListPrivateHubAccesses{}.Descriptor(),
		work.ListEmergencyNotices{}.Descriptor(),
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
		work.RestoreLabel{}.Descriptor(),
		work.AddLabel{}.Descriptor(),
		work.RemoveLabel{}.Descriptor(),
		work.AssignWorkItem{}.Descriptor(),
		work.UnassignWorkItem{}.Descriptor(),
		work.AutoAssignWorkItem{}.Descriptor(),
		work.AddMember{}.Descriptor(),
		work.RemoveMember{}.Descriptor(),
		work.AddComment{}.Descriptor(),
		work.ListComments{}.Descriptor(),
		work.ReadKudos{}.Descriptor(),
		work.GiveKudos{}.Descriptor(),
		work.TakeBackKudos{}.Descriptor(),
		work.EditComment{}.Descriptor(),
		work.DeleteComment{}.Descriptor(),
		work.GetContainer{}.Descriptor(),
		work.ListContainers{}.Descriptor(),
		work.ListPresets{}.Descriptor(),
		work.ListJumbleDestinations{}.Descriptor(),
		work.GetWorkItem{}.Descriptor(),
		work.ListWorkItems{}.Descriptor(),
		work.QueryItems{}.Descriptor(),
		work.AggregateItems{}.Descriptor(),
		work.AggregateView{}.Descriptor(),
		work.SearchItems{}.Descriptor(),
		work.ReindexSearch{}.Descriptor(),
		work.ListActivity{}.Descriptor(),
		work.CompleteWorkItem{}.Descriptor(),
		work.ReopenWorkItem{}.Descriptor(),
		work.MoveWorkItem{}.Descriptor(),
		work.DuplicateWorkItem{}.Descriptor(),
		work.MergeWorkItems{}.Descriptor(),
		work.SplitWorkItem{}.Descriptor(),
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
		work.PreviewCustomFieldReach{}.Descriptor(),
		work.ExportView{}.Descriptor(),
		work.ExportItems{}.Descriptor(),
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
		backupservice.DeleteBackupTarget{}.Descriptor(),
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
		lifecycle.UpdateRetentionPolicy{}.Descriptor(),
		lifecycle.DeleteRetentionPolicy{}.Descriptor(),
		lifecycle.PreviewRetentionPolicy{}.Descriptor(),
		lifecycle.RetainItem{}.Descriptor(),
		lifecycle.PlaceLegalHold{}.Descriptor(),
		lifecycle.ReleaseLegalHold{}.Descriptor(),
		lifecycle.ListLegalHolds{}.Descriptor(),
		lifecycle.ReplaceLegalHolds{}.Descriptor(),
		privacyservice.ReapplyErasures{}.Descriptor(),
		auditservice.ListAuditEntries{}.Descriptor(),
		auditservice.VerifyAuditChain{}.Descriptor(),
		auditservice.ConfigureAuditAnchoring{}.Descriptor(),
		auditservice.ExportAuditTrail{}.Descriptor(),
		importservice.ImportEntries{}.Descriptor(),
		importservice.GetImport{}.Descriptor(),
		privacyservice.CreateDataSubjectRequest{}.Descriptor(),
		privacyservice.ListDataSubjectRequests{}.Descriptor(),
		privacyservice.UpdateDataSubjectRequest{}.Descriptor(),
		privacyservice.ExtendDataSubjectRequest{}.Descriptor(),
		privacyservice.PreviewErasure{}.Descriptor(),
		privacyservice.RestrictProcessing{}.Descriptor(),
		privacyservice.WithdrawConsent{}.Descriptor(),
		privacyservice.GiveConsent{}.Descriptor(),
		privacyservice.ListConsents{}.Descriptor(),
		integration.ReadOwnAiPosition{}.Descriptor(),
		integration.AcknowledgeAiPosition{}.Descriptor(),
		backupservice.CreateBackupSchedule{}.Descriptor(),
		backupservice.ListBackupSchedules{}.Descriptor(),
		backupservice.UpdateBackupSchedule{}.Descriptor(),
		backupservice.DeleteBackupSchedule{}.Descriptor(),
		jobservice.GetJob{}.Descriptor(),
		jobservice.CancelJob{}.Descriptor(),
		identity.InviteAccount{}.Descriptor(),
		identity.RenewInvitation{}.Descriptor(),
		identity.CreateManagedAccount{}.Descriptor(),
		identity.IssueStartPassword{}.Descriptor(),
		identity.RequestAddress{}.Descriptor(),
		identity.GetOwnAccount{}.Descriptor(),
		identity.ReadOwnSettings{}.Descriptor(),
		identity.GetAccount{}.Descriptor(),
		identity.UpdateAccountPreferences{}.Descriptor(),
		identity.UpdateOwnAccount{}.Descriptor(),
		identity.SetOwnAvatar{}.Descriptor(),
		identity.RemoveOwnAvatar{}.Descriptor(),
		identity.ListNotificationPreferences{}.Descriptor(),
		identity.SetNotificationPreference{}.Descriptor(),
		identity.ListMemberships{}.Descriptor(),
		identity.GrantMembership{}.Descriptor(),
		identity.RevokeMembership{}.Descriptor(),
		identity.ListRoles{}.Descriptor(),
		identity.CreateRole{}.Descriptor(),
		identity.ChangeRole{}.Descriptor(),
		identity.DeleteRole{}.Descriptor(),
		identity.GrantRole{}.Descriptor(),
		identity.RevokeRoleGrant{}.Descriptor(),
		identity.ExplainAccess{}.Descriptor(),
		identity.ListGroups{}.Descriptor(),
		identity.GetGroup{}.Descriptor(),
		identity.CreateGroup{}.Descriptor(),
		identity.UpdateGroup{}.Descriptor(),
		identity.DeleteGroup{}.Descriptor(),
		identity.GetSignInRules{}.Descriptor(),
		identity.ChangePassword{}.Descriptor(),
		identity.CheckPassword{}.Descriptor(),
		identity.ForgetPassword{}.Descriptor(),
		identity.ResetPassword{}.Descriptor(),
		identity.ConfirmAddress{}.Descriptor(),
		identity.SetPasswordAndSignIn{}.Descriptor(),
		identity.RegenerateRecoveryCodes{}.Descriptor(),
		identity.ElevateSession{}.Descriptor(),
		identity.StartViewAs{}.Descriptor(),
		identity.GetViewAs{}.Descriptor(),
		identity.EndViewAs{}.Descriptor(),
		identity.SignIn{}.Descriptor(),
		identity.RefreshSession{}.Descriptor(),
		identity.ListSessions{}.Descriptor(),
		identity.RevokeSession{}.Descriptor(),
		identity.RevokeAllSessions{}.Descriptor(),
		identity.RevokeOtherSessions{}.Descriptor(),
		syncservice.ListSyncDevices{}.Descriptor(),
		syncservice.ForgetSyncDevice{}.Descriptor(),
		identity.RedeemInvitation{}.Descriptor(),
		identity.SetUpInstallation{}.Descriptor(),
		identity.CompleteSignIn{}.Descriptor(),
		identity.EnrollTotp{}.Descriptor(),
		identity.ConfirmTotp{}.Descriptor(),
		identity.DisableTotp{}.Descriptor(),
		identity.StartAuthenticatorReplacement{}.Descriptor(),
		identity.ConfirmAuthenticatorReplacement{}.Descriptor(),
		identity.StepUp{}.Descriptor(),
		identity.StartProviderStepUp{}.Descriptor(),
		identity.StartPasskeyStepUp{}.Descriptor(),
		identity.RegisterOauthClient{}.Descriptor(),
		identity.ListOauthClients{}.Descriptor(),
		identity.ReadOauthClient{}.Descriptor(),
		identity.DeleteOauthClient{}.Descriptor(),
		identity.AuthorizeOauthClient{}.Descriptor(),
		identity.ExchangeOauthCode{}.Descriptor(),
		identity.ListOauthGrants{}.Descriptor(),
		identity.RevokeOauthGrant{}.Descriptor(),
		identity.ReadWorkspace{}.Descriptor(),
		identity.UpdateWorkspace{}.Descriptor(),
		identity.ConfigureIdentityProvider{}.Descriptor(),
		identity.ShowLdapDirectory{}.Descriptor(),
		identity.ConfigureLdapDirectory{}.Descriptor(),
		identity.RemoveLdapDirectory{}.Descriptor(),
		identity.SyncLdapDirectory{}.Descriptor(),
		identity.ListIdentityProviders{}.Descriptor(),
		identity.CountAccountsWithoutProvider{}.Descriptor(),
		identity.ReadIdentityProvider{}.Descriptor(),
		identity.ConfigureFirstIdentityProvider{}.Descriptor(),
		identity.OfferIdentityProvider{}.Descriptor(),
		identity.RemoveIdentityProvider{}.Descriptor(),
		identity.ListIdentityProviderPresets{}.Descriptor(),
		adminservice.ListInstanceIdentityProviders{}.Descriptor(),
		adminservice.ConfigureInstanceIdentityProvider{}.Descriptor(),
		adminservice.RemoveInstanceIdentityProvider{}.Descriptor(),
		adminservice.WithdrawInstanceIdentityProvider{}.Descriptor(),
		adminservice.CancelInstanceIdentityProviderWithdrawal{}.Descriptor(),
		adminservice.ListInstanceAiProviders{}.Descriptor(),
		adminservice.ConfigureInstanceAiProvider{}.Descriptor(),
		adminservice.RemoveInstanceAiProvider{}.Descriptor(),
		adminservice.ListInstanceBackupTargets{}.Descriptor(),
		adminservice.CreateInstanceBackupTarget{}.Descriptor(),
		adminservice.TestInstanceBackupTarget{}.Descriptor(),
		adminservice.DeleteInstanceBackupTarget{}.Descriptor(),
		integration.ConfigureAiProvider{}.Descriptor(),
		integration.ReadAiProvider{}.Descriptor(),
		integration.RemoveAiProvider{}.Descriptor(),
		integration.ReadAiSource{}.Descriptor(),
		integration.ChooseAiSource{}.Descriptor(),
		integration.ReadAiBudgetState{}.Descriptor(),
		integration.SetAiPersonOptOut{}.Descriptor(),
		integration.SetAiArrivalProposals{}.Descriptor(),
		suggestionservice.ListSuggestions{}.Descriptor(),
		suggestionservice.AcceptSuggestion{}.Descriptor(),
		suggestionservice.DismissSuggestion{}.Descriptor(),
		suggestionservice.SuggestDecomposition{}.Descriptor(),
		suggestionservice.AiSuggestFields{}.Descriptor(),
		suggestionservice.AiSummarize{}.Descriptor(),
		suggestionservice.AiClassify{}.Descriptor(),
		suggestionservice.SuggestDuplicates{}.Descriptor(),
		suggestionservice.FindDuplicatesByTitle{}.Descriptor(),
		suggestionservice.AiSummarizeThread{}.Descriptor(),
		suggestionservice.AiSummarizeContainer{}.Descriptor(),
		suggestionservice.AiGenerateTemplate{}.Descriptor(),
		suggestionservice.DescribeRule{}.Descriptor(),
		suggestionservice.MakeEntries{}.Descriptor(),
		suggestionservice.AskWorkspace{}.Descriptor(),
		work.AiTranslate{}.Descriptor(),
		identity.StartOidcSignIn{}.Descriptor(),
		identity.CompleteOidcSignIn{}.Descriptor(),
		identity.LocateOidcFlow{}.Descriptor(),
		identity.CompleteLink{}.Descriptor(),
		identity.StartProviderConnect{}.Descriptor(),
		identity.CompleteProviderConnect{}.Descriptor(),
		identity.ListOwnIdentities{}.Descriptor(),
		identity.ListOwnPasskeys{}.Descriptor(),
		identity.StartPasskeyRegistration{}.Descriptor(),
		identity.CompletePasskeyRegistration{}.Descriptor(),
		identity.StartPasskeySignIn{}.Descriptor(),
		identity.CompletePasskeySignIn{}.Descriptor(),
		identity.StartPasskeyEnrollment{}.Descriptor(),
		identity.CompletePasskeyEnrollment{}.Descriptor(),
		identity.RenameOwnPasskey{}.Descriptor(),
		identity.RemoveOwnPasskey{}.Descriptor(),
		identity.AgreeToTerms{}.Descriptor(),
		identity.InspectInvitation{}.Descriptor(),
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
		automationservice.CheckRules{}.Descriptor(),
		jumbleservice.SubmitJumbleEntry{}.Descriptor(),
		jumbleservice.ListJumbleEntries{}.Descriptor(),
		jumbleservice.GetJumbleEntry{}.Descriptor(),
		jumbleservice.ConvertJumbleEntry{}.Descriptor(),
		jumbleservice.DismissJumbleEntry{}.Descriptor(),
		jumbleservice.SuggestFromJumbleEntry{}.Descriptor(),
		jumbleservice.RotateJumbleIntake{}.Descriptor(),
		jumbleservice.CommentJumbleEntry{}.Descriptor(),
		voiceservice.ShowVoiceSettings{}.Descriptor(),
		voiceservice.SaveVoiceSettings{}.Descriptor(),
		voiceservice.TryVoiceSentence{}.Descriptor(),
		voiceservice.ShowAlexaModel{}.Descriptor(),
		mailservice.ShowMailSettings{}.Descriptor(),
		mailservice.SaveMailSettings{}.Descriptor(),
		mailservice.ListMailReceipts{}.Descriptor(),
		mailservice.SetCollectionHandle{}.Descriptor(),
		mailservice.ListMailboxes{}.Descriptor(),
		mailservice.ConnectMailbox{}.Descriptor(),
		mailservice.CheckMailbox{}.Descriptor(),
		mailservice.RemoveMailbox{}.Descriptor(),
		mailservice.AuthorizeMailbox{}.Descriptor(),
		mailservice.CompleteMailboxAuthorization{}.Descriptor(),
		adminservice.ReadInstanceSettings{}.Descriptor(),
		adminservice.PlanInstanceSettings{}.Descriptor(),
		adminservice.WriteInstanceSettings{}.Descriptor(),
		adminservice.ListOperators{}.Descriptor(),
		adminservice.ReadInstanceOverview{}.Descriptor(),
		adminservice.ListInstanceJournal{}.Descriptor(),
		adminservice.ListPlatformEvents{}.Descriptor(),
		adminservice.ListPlatformSubscriptions{}.Descriptor(),
		adminservice.CreatePlatformSubscription{}.Descriptor(),
		adminservice.ReadPlatformSubscription{}.Descriptor(),
		adminservice.UpdatePlatformSubscription{}.Descriptor(),
		adminservice.DeletePlatformSubscription{}.Descriptor(),
		adminservice.RotatePlatformSubscriptionSecret{}.Descriptor(),
		adminservice.ListPlatformDeliveries{}.Descriptor(),
		adminservice.RedeliverPlatformSubscription{}.Descriptor(),
		adminservice.ReadUsage{}.Descriptor(),
		adminservice.AddOperator{}.Descriptor(),
		adminservice.RemoveOperator{}.Descriptor(),
		adminservice.ListPlans{}.Descriptor(),
		adminservice.ReadPlan{}.Descriptor(),
		adminservice.CreatePlan{}.Descriptor(),
		adminservice.ChangePlan{}.Descriptor(),
		adminservice.RemovePlan{}.Descriptor(),
		adminservice.MoveTenantPlan{}.Descriptor(),
		adminservice.ListInstallationPresets{}.Descriptor(),
		adminservice.CreateInstallationPreset{}.Descriptor(),
		adminservice.ChangeInstallationPreset{}.Descriptor(),
		adminservice.WithdrawInstallationPreset{}.Descriptor(),
		adminservice.UpdateTenant{}.Descriptor(),
		adminservice.ReadTenant{}.Descriptor(),
		adminservice.QueryTenants{}.Descriptor(),
		adminservice.ProvisionTenant{}.Descriptor(),
		adminservice.ListTenants{}.Descriptor(),
		adminservice.SuspendTenant{}.Descriptor(),
		adminservice.ResumeTenant{}.Descriptor(),
		adminservice.RequestTenantDeletion{}.Descriptor(),
		adminservice.CancelTenantDeletion{}.Descriptor(),
		adminservice.OpenTenantPassword{}.Descriptor(),
		adminservice.CloseTenantPassword{}.Descriptor(),
		adminservice.ExportTenant{}.Descriptor(),
		adminservice.UpdateTenantQuotas{}.Descriptor(),
		quotaservice.ReadQuotas{}.Descriptor(),
		quotaservice.SetOwnAiBudget{}.Descriptor(),
		metaservice.GetHealthReport{}.Descriptor(),
		sealingservice.ReadEncryptionStatus{}.Descriptor(),
		sealingservice.ResealSecrets{}.Descriptor(),
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
		for _, scope := range []string{descriptor.TokenScope, descriptor.AlsoScope} {
			if scope == "" || seen[scope] {
				continue
			}
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	// And the one scope that is not an operation's. `agent:destructive` is a *capability*:
	// it says an agent token may reach the destructive use cases at all, and no descriptor declares
	// it because no single operation is what it permits. It is named here rather than derived so
	// that CreateAccessToken accepts it - a scope a token cannot be minted with is a guardrail
	// nobody can switch off, which sounds safe and means the feature does not exist.
	scopes = append(scopes, usecase.AgentDestructiveScope)
	// And the per-section apply scopes of the configuration document, for the same reason: no one
	// operation is what they permit, they bound which blocks an apply may write.
	scopes = append(scopes, configservice.ApplyScopes()...)

	slices.Sort(scopes)
	return scopes
}

// AutomationActions is every action kind a rule may name that is a use case, sorted: what the
// manifest answers and a rule editor offers. Derived from the descriptors for the
// reason Scopes is, and handed to the manifest by the composition root for the same reason - the
// engine's own flow kinds (WAIT, BRANCH, STOP) are not in it because they are in no catalogue;
// the contract says so, and a client names those three itself.
func AutomationActions() []string {
	actions := make([]string, 0, len(Descriptors()))
	for _, descriptor := range Descriptors() {
		actions = append(actions, descriptor.AutomationAction())
	}
	slices.Sort(actions)
	return actions
}

// ActionEntitlements is, for every action kind whose use case uses a feature a plan may leave out,
// that feature (usecase.Descriptor.Entitlement): what the manifest leaves out of the actions it
// offers where the plan leaves the feature out.
func ActionEntitlements() map[string]entitlement.Feature {
	uses := map[string]entitlement.Feature{}
	for _, descriptor := range Descriptors() {
		if descriptor.Entitlement != "" {
			uses[descriptor.AutomationAction()] = descriptor.Entitlement
		}
	}
	return uses
}

// AutomationActionFields is, for every action kind AutomationActions names, the fields the use
// case behind it declares - what the manifest answers so that a rule editor builds an action's
// form from the declaration rather than from a schema compiled into it. The same
// []usecase.Field the MCP tool schema is derived from, read off the descriptor here and never
// declared a second time; a kind with no fields maps to an empty slice rather than being absent,
// because "no parameters" and "no such kind" are two different answers.
func AutomationActionFields() map[string][]usecase.Field {
	fields := make(map[string][]usecase.Field, len(Descriptors()))
	for _, descriptor := range Descriptors() {
		declared := make([]usecase.Field, len(descriptor.Input))
		copy(declared, descriptor.Input)
		fields[descriptor.AutomationAction()] = declared
	}
	return fields
}

// AutomationActionSummaries is, for every action kind AutomationActions names, the one sentence
// its use case declares as its Summary - the sentence the MCP tool for it carries, read off the
// descriptor here so that a rule editor's catalogue says what a kind does without a second copy
// of ninety sentences. Protocol documentation in English, as the Field
// descriptions are, and for the same reason not a breach of ADR-0011.
func AutomationActionSummaries() map[string]string {
	summaries := make(map[string]string, len(Descriptors()))
	for _, descriptor := range Descriptors() {
		summaries[descriptor.AutomationAction()] = descriptor.Summary
	}
	return summaries
}

// SessionScopes is what a session-authenticated person may exercise: every declared scope except
// the control plane's. A session is the person themselves - but the admin surface is entered by
// a deliberately minted credential, never by whoever happens to be signed in (identity.md
// §15.4), so the one scope class sessions never carry is `admin:*`.
//
// **One exception, ADR-0070 §4, written here rather than left to be discovered.** A
// session *raised* by a registered operator who has just passed a step-up carries `admin:tenants`
// for an hour - which is not this function's doing: it still answers every scope but the control
// plane's, and `AuthenticateToken.elevatedScopes` adds the one back for exactly that hour, for
// exactly that session, and only while the register still names its holder. The rule is therefore
// weakened to *only for a registered operator, only after a fresh proof, only for an hour, only on
// the session that proved it, and written down*, and what it buys is that nobody has to mint a
// long-lived all-powerful token and paste it into a browser to change a switch.
func SessionScopes() []string {
	scopes := make([]string, 0, len(Scopes()))
	for _, scope := range Scopes() {
		if strings.HasPrefix(scope, "admin:") {
			continue
		}
		// Nor the agent capability. A session is a person at a keyboard, never an actor this
		// system calls an agent, so the scope would be one a session could carry and nothing
		// would ever read - and a bound that is never read is one its holder believes in.
		if scope == usecase.AgentDestructiveScope {
			continue
		}
		scopes = append(scopes, scope)
	}
	return scopes
}
