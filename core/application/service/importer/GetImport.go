// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// GetImport answers an import and its report, to whoever may read the hub it landed in.
type GetImport struct {
	Runs       repository.Runs
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// Execute reads the run. The permission is asked after the row is found and against the hub the
// row names, because the row is what says which hub - and a run the caller may not see answers
// the same code as one that does not exist.
func (h GetImport) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Run, error) {
	var run domain.Run
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Runs.Find(ctx, id)
		run = found
		return err
	})
	if err != nil {
		return domain.Run{}, err
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: domainservice.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(run.HubID)},
		TokenScope: containersRead,
		TargetType: importTarget,
		TargetID:   run.ID,
	}); err != nil {
		if errors.Is(err, shared.ErrForbidden) {
			return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeNotFound).
				WithParams(map[string]string{"import_id": id.String()})
		}
		return domain.Run{}, err
	}
	return run, nil
}

// Descriptor is the catalogue entry.
func (h GetImport) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetImportName,
		Summary: "An import and its report: what landed, in the restore's shape, and the rows the " +
			"converter refused by number. Visible to whoever may read the hub it landed in.",
		SideEffects: "None.",
		TokenScope:  containersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "import_id", Kind: usecase.KindID, Required: true, Description: "The import."},
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetImport) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	id, err := in.ID("import_id")
	if err != nil {
		return nil, err
	}
	run, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return RunOutput(run), nil
}

// RunOutput is the shape every channel answers: the contract's ImportRun.
func RunOutput(run domain.Run) usecase.Output {
	out := usecase.Output{
		"id": run.ID.String(), "kind": string(run.Kind), "hub_id": run.HubID.String(),
		"media_id": run.MediaID.String(), "status": string(run.Status),
		"created_at": run.CreatedAt, "finished_at": nil, "error_code": nil,
	}
	if run.FinishedAt != nil {
		out["finished_at"] = *run.FinishedAt
	}
	if run.ErrorCode != "" {
		out["error_code"] = run.ErrorCode
	}
	if run.Status == domain.StatusSucceeded || run.Status == domain.StatusFailed {
		report := run.Report
		out["report"] = usecase.Output{
			"new": report.New, "overwritten": report.Overwritten, "skipped": report.Skipped,
			"duplicated": report.Duplicated, "conflicts": report.Conflicts, "media": report.Media,
			"withheld": report.Withheld, "entities": report.Entities,
		}
	}
	refused := make([]usecase.Output, 0, len(run.Refused))
	for _, r := range run.Refused {
		refused = append(refused, usecase.Output{"row": r.Row, "code": r.Code})
	}
	out["refused"] = refused
	return out
}
