// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// SignInPictures admits a picture the sign-in card draws, for a browser without a session
// (UC-INS-22 check 8). Not a catalogue entry: it answers bytes, which neither MCP nor a rule could use.
//
// The workspace is the one the address names, resolved as the rules resolve it but never from a
// header - an image request sends none, and the installation's own address draws the product's
// mark. The picture must be one that card draws now, and still a raster image of the workspace.
// Every refusal is the same 404, so the route says nothing the rules at that address do not (T-34).
type SignInPictures struct {
	Rules GetSignInRules
	// Objects is the workspace's media, judged again before a byte is served.
	Objects mediarepo.Objects
}

func signInPictureGone() error {
	return shared.ErrNotFound.WithDetail("media.not_found")
}

// Picture answers the workspace the picture is read in.
func (p SignInPictures) Picture(ctx context.Context, host string, mediaID shared.ID) (shared.ID, error) {
	rules := p.Rules
	if !rules.addressedByHost(host) || mediaID.IsZero() {
		return "", signInPictureGone()
	}
	tenantID := rules.resolveTenant(ctx, host, "")
	if tenantID.IsZero() {
		return "", signInPictureGone()
	}
	resolved, err := rules.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if !rules.signInCard(host, tenantID, resolved).Draws(mediaID) {
		return "", signInPictureGone()
	}
	err = rules.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		object, err := p.Objects.Find(ctx, mediaID)
		if err != nil {
			return err
		}
		if object.DeletedAt != nil {
			return signInPictureGone()
		}
		if object.WorkspaceLogo() != nil {
			// Set as a raster picture and judged so; anything else is never served from here.
			return signInPictureGone()
		}
		return nil
	})
	if errors.Is(err, shared.ErrNotFound) {
		return "", signInPictureGone()
	}
	if err != nil {
		return "", err
	}
	return tenantID, nil
}
