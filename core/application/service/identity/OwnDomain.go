// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// OwnDomainOutput is the contract's OwnDomain: the workspace's own domain with its two records,
// or the block its owner still sees. Nil where there is neither.
func OwnDomainOutput(view *domain.OwnDomain, block *domain.DomainBlock, installationHost string) usecase.Output {
	if block != nil {
		return usecase.Output{
			"domain": block.Host, "state": string(repository.HostStateBlocked), "since": block.BlockedAt,
			"is_main": block.WasMain, "certificate_asked": false, "blocked_reason": block.Reason,
			"records": []usecase.Output{},
		}
	}
	if view == nil {
		return nil
	}
	out := usecase.Output{
		"domain": view.Host, "state": string(view.State), "since": view.Since, "is_main": view.Main,
		"certificate_asked": view.CertificateAsked,
		"records": []usecase.Output{
			{"type": "CNAME", "name": view.Host, "value": domain.OwnDomainTarget(installationHost), "found": view.TargetFound},
			{"type": "TXT", "name": domain.OwnDomainProofName(view.Host), "value": view.Verification, "found": view.ProofFound},
		},
	}
	if !view.CheckedAt.IsZero() {
		out["checked_at"] = view.CheckedAt
	}
	if !view.FellBackAt.IsZero() {
		out["fell_back_at"] = view.FellBackAt
	}
	return out
}

// ownDomainView reads the workspace's own domain, or the block it still sees, in the caller's
// transaction.
func ownDomainView(ctx context.Context, domains repository.OwnDomains, installationHost string) (usecase.Output, error) {
	view, err := domains.Find(ctx)
	if err == nil {
		return OwnDomainOutput(&view, nil, installationHost), nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}
	block, err := domains.BlockInView(ctx)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return OwnDomainOutput(nil, &block, installationHost), nil
}

// CertificateQuestion answers the ingress's question on the operations port: may this host have a
// certificate (ADR-0095 §5)? Yes only for a workspace's serving host - an own domain once verified
// and never once blocked. The port is not public, so this is no lookup for strangers; nothing about
// the workspace is answered beyond the yes. Single mode answers no: its one workspace is the
// installation's own host, whose certificate is the operator's.
type CertificateQuestion struct {
	Doors      repository.OwnDomainDoors
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Tenancy    env.TenancyMode
}

// Allowed answers the question for one host.
func (q CertificateQuestion) Allowed(ctx context.Context, rawHost string) (bool, error) {
	host := domain.RequestHost(rawHost)
	if q.Tenancy != env.TenancyMulti || q.Doors == nil || host == "" {
		return false, nil
	}
	allowed := false
	err := q.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		answer, err := q.Doors.Certificate(ctx, host, q.Clock.Now())
		allowed = answer
		return err
	})
	return allowed, err
}
