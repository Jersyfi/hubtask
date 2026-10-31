// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The sandbox (configuration.md §3): one copy of the workspace's configuration without content,
// where an agent changes freely - its token's plan scope is enough, and no policy holds it back,
// because nothing runs. A write is checked as its promotion would be, so the sandbox never holds
// what the workspace would refuse; promoting it is an ordinary plan and apply of what it changed
// since its copy (`from_sandbox`), held to every rule an apply is.
const (
	ReadConfigSandboxName  = "ReadConfigSandbox"
	WriteConfigSandboxName = "WriteConfigSandbox"

	SandboxAction audit.Action = "config.sandbox_written"
)

// ReadConfigSandbox answers the sandbox and what promoting it would do.
type ReadConfigSandbox struct{ Desk Desk }

func (h ReadConfigSandbox) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReadConfigSandboxName,
		Permission: service.PermissionStructure,
		Summary: "The configuration sandbox: a copy of the workspace's configuration without content " +
			"where changes run nothing, the configuration it was copied from, and the plan of " +
			"promoting it - only what it changed since, against the configuration in force.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: target, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, ReadAction, scopeRead, true); err != nil {
				return nil, err
			}
			var (
				sandbox domain.Sandbox
				held    bool
			)
			err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				sandbox, held, err = h.Desk.Store.Sandbox(ctx)
				return err
			})
			if err != nil {
				return nil, err
			}
			if !held {
				return usecase.Output{"exists": false}, nil
			}
			p, _, _, _, err := h.Desk.planned(ctx, actor, usecase.Input{"from_sandbox": true})
			if err != nil {
				return nil, err
			}
			return SandboxOutput(sandbox, PlanOutput(p)), nil
		}),
	}
}

// SandboxOutput is the sandbox as every door answers it.
func SandboxOutput(s domain.Sandbox, promotion usecase.Output) usecase.Output {
	out := usecase.Output{
		"exists": true, "document": documentMap(s.Document), "hash": domain.Hash(s.Document),
		"base_hash": s.BaseHash, "base_version": s.BaseVersion, "writes": s.Writes, "updated_at": s.UpdatedAt,
	}
	if promotion != nil {
		out["promotion"] = promotion
	}
	return out
}

// WriteConfigSandbox changes the sandbox, copies it afresh, or discards it.
type WriteConfigSandbox struct{ Desk Desk }

func (h WriteConfigSandbox) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       WriteConfigSandboxName,
		Permission: service.PermissionStructure,
		Summary: "Changes the configuration sandbox by a document, as a plan would read it against " +
			"the sandbox - nothing runs, so an agent may do it freely; or, with reset, copies the " +
			"configuration in force afresh; or, with discard, throws the sandbox away. A document " +
			"the workspace would refuse is refused here too, with the same refusals.",
		SideEffects: "Stores the sandbox and writes an audit entry with the count of changes. Changes " +
			"no configuration in force.",
		TokenScope: scopePlan,
		Input: []usecase.Field{
			{Name: "document", Kind: usecase.KindObject, Description: "Sections of blocks by stable key to change in the sandbox; a null block removes it."},
			{Name: "mode", Kind: usecase.KindString, Enum: modes, Description: "MERGE (default) changes only what the document names; REPLACE states every section it names whole."},
			{Name: "reset", Kind: usecase.KindBool, Description: "True copies the configuration in force afresh, before the document is read."},
			{Name: "discard", Kind: usecase.KindBool, Description: "True throws the sandbox away; nothing else may be given."},
		},
		Audit:    usecase.AuditDeclaration{Action: SandboxAction, TargetType: target, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "the sandbox is not in force and not an item; the trail holds the write."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Desk.authorize(ctx, actor, SandboxAction, scopePlan, false); err != nil {
				return nil, err
			}
			return h.Desk.writeSandbox(ctx, actor, in)
		}),
	}
}

func (d Desk) writeSandbox(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	var out usecase.Output
	err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		at := d.Clock.Now()
		if in.Bool("discard") {
			if in.Present("document") || in.Bool("reset") {
				return domain.ErrRefused([]shared.FieldError{{Path: "/discard", Code: "config.sandbox_discard_alone"}})
			}
			if _, err := d.Store.DropSandbox(ctx); err != nil {
				return err
			}
			out = usecase.Output{"exists": false}
			return d.trail(ctx, actor, SandboxAction, at, audit.Change{Field: "discarded", Classification: audit.Open, To: "true"})
		}
		now, err := d.export(ctx, actor, "")
		if err != nil {
			return err
		}
		sandbox, held, err := d.Store.Sandbox(ctx)
		if err != nil {
			return err
		}
		if !held || in.Bool("reset") {
			latest, _, err := d.Store.Latest(ctx)
			if err != nil {
				return err
			}
			sandbox = domain.Sandbox{
				TenantID: actor.TenantID, Document: now.Document, Base: now.Document, BaseHash: now.hash(),
				BaseVersion: latest.Version,
			}
		}
		changed := 0
		if in.Present("document") {
			desired, refusals := documentOf(in["document"])
			if len(refusals) > 0 {
				return domain.ErrRefused(refusals)
			}
			mode := domain.ModeMerge
			if in.String("mode") == string(domain.ModeReplace) {
				mode = domain.ModeReplace
			}
			next := domain.Resolve(sandbox.Document, desired, mode)
			changed = len(domain.Diff(sandbox.Document, next))
			// Held to what promoting it would be held to, against the configuration in force.
			candidate := sandbox
			candidate.Document = next
			if p := d.planOf(now, candidate.Promotion(), domain.ModeMerge); len(p.Refusals) > 0 {
				return domain.ErrRefused(p.Refusals)
			}
			sandbox.Document = next
		}
		sandbox.Writes += changed
		sandbox.UpdatedAt = at
		if err := d.Store.PutSandbox(ctx, sandbox); err != nil {
			return err
		}
		out = SandboxOutput(sandbox, nil)
		return d.trail(ctx, actor, SandboxAction, at,
			audit.Change{Field: "changed", Classification: audit.Open, To: strconv.Itoa(changed)},
			audit.Change{Field: "reset", Classification: audit.Open, To: strconv.FormatBool(!held || in.Bool("reset"))})
	})
	return out, err
}

// sandboxPromotion is what a plan or apply with `from_sandbox` asks: the sandbox's changes since
// its copy, read as MERGE against the configuration in force.
func (d Desk) sandboxPromotion(ctx context.Context) (domain.Document, error) {
	sandbox, held, err := d.Store.Sandbox(ctx)
	if err != nil {
		return nil, err
	}
	if !held {
		return nil, shared.ErrNotFound.WithDetail("config.sandbox_none")
	}
	return sandbox.Promotion(), nil
}
