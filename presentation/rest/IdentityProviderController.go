// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The relying party's configuration use cases.
const (
	countAccountsWithoutProviderUseCase = "CountAccountsWithoutProvider"
)

// identityProvidersResponse maps the collection.
func identityProvidersResponse(out usecase.Output) []openapi.IdentityProvider {
	rows, _ := out["data"].([]usecase.Output)
	answers := make([]openapi.IdentityProvider, 0, len(rows))
	for _, row := range rows {
		answers = append(answers, identityProviderResponse(row))
	}
	return answers
}

// identityProviderResponse maps the use case's answer. The client secret is not among the
// fields, because it is not among the use case's either - there is no call that answers it.
func identityProviderResponse(out usecase.Output) openapi.IdentityProvider {
	answer := openapi.IdentityProvider{
		Id:                  uuidValue(out.String("id")),
		Scope:               openapi.IdentityProviderScope(out.String("scope")),
		Issuer:              out.String("issuer"),
		ClientId:            out.String("client_id"),
		DisplayName:         out.String("display_name"),
		Kind:                openapi.IdentityProviderKind(out.String("kind")),
		Provisioning:        openapi.IdentityProviderProvisioning(out.String("provisioning")),
		AllowedEmailDomains: []string{},
		AllowedDirectories:  []string{},
	}
	if position, held := out["position"].(int); held {
		answer.Position = position
	}
	// A nil list stays the empty one: the contract's arrays are never null.
	if domains, held := out["allowed_email_domains"].([]string); held && domains != nil {
		answer.AllowedEmailDomains = domains
	}
	if directories, held := out["allowed_directories"].([]string); held && directories != nil {
		answer.AllowedDirectories = directories
	}
	if enabled, held := out["enabled"].(bool); held {
		answer.Enabled = enabled
	}
	if role, held := out["newcomer_role"].(string); held && role != "" {
		newcomer := openapi.MembershipRole(role)
		answer.NewcomerRole = &newcomer
	}
	if group, held := out["newcomer_group_id"].(string); held && group != "" {
		newcomer := uuidValue(group)
		answer.NewcomerGroupId = &newcomer
	}
	if offered, held := out["offered_here"].(bool); held {
		answer.OfferedHere = &offered
	}
	if lock, held := out["installation_lock"].(string); held {
		value := openapi.IdentityProviderInstallationLock(lock)
		answer.InstallationLock = &value
	}
	if created, held := out["created_at"].(time.Time); held {
		answer.CreatedAt = created
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	if version, held := out["version"].(int); held {
		answer.Version = version
	}
	if withdrawAt, held := out["withdraw_at"].(time.Time); held && !withdrawAt.IsZero() {
		answer.WithdrawAt = &withdrawAt
	}
	// The operator's projection carries it and a workspace's does not; absent stays absent.
	if offered, held := out["offered_workspaces"].(int); held {
		answer.OfferedWorkspaces = &offered
	}
	return answer
}

// identityProviderPresetResponse maps one preset. The instructions stay a message code: the
// backend holds no display text (rule 8), and which language it is rendered in is the client's.
func identityProviderPresetResponse(out usecase.Output) openapi.IdentityProviderPreset {
	answer := openapi.IdentityProviderPreset{
		Kind:         openapi.IdentityProviderKind(out.String("kind")),
		Scopes:       []string{},
		RedirectUri:  out.String("redirect_uri"),
		Instructions: out.String("instructions"),
		Provisioning: []openapi.IdentityProviderProvisioning{},
		// Required by the contract, so never absent: the installation's list is what its own
		// screen offers (ADR-0071's addendum).
		InstallationProvisioning: []openapi.IdentityProviderProvisioning{},
	}
	answer.Scopes = append(answer.Scopes, outputStrings(out["scopes"])...)
	for _, mode := range outputStrings(out["provisioning"]) {
		answer.Provisioning = append(answer.Provisioning,
			openapi.IdentityProviderProvisioning(mode))
	}
	for _, mode := range outputStrings(out["installation_provisioning"]) {
		answer.InstallationProvisioning = append(answer.InstallationProvisioning,
			openapi.IdentityProviderProvisioning(mode))
	}
	if verified, held := out["addresses_verified"].(bool); held {
		answer.AddressesVerified = verified
	}
	if public, held := out["public"].(bool); held {
		answer.Public = public
	}
	if particular := out.String("particular"); particular != "" {
		answer.Particular = &particular
	}
	if issuer := out.String("issuer"); issuer != "" {
		answer.Issuer = &issuer
	}
	if claim := out.String("directory_claim"); claim != "" {
		answer.DirectoryClaim = &claim
	}
	if templated, held := out["supports_templated_issuer"].(bool); held {
		answer.SupportsTemplatedIssuer = templated
	}
	// Required in the contract: an empty list, never null, where no issuer is shared.
	answer.SharedIssuerPaths = []string{}
	if paths, held := out["shared_issuer_paths"].([]any); held {
		for _, path := range paths {
			if text, ok := path.(string); ok {
				answer.SharedIssuerPaths = append(answer.SharedIssuerPaths, text)
			}
		}
	}
	answer.PersonalIssuerPaths = []string{}
	if paths, held := out["personal_issuer_paths"].([]any); held {
		for _, path := range paths {
			if text, ok := path.(string); ok {
				answer.PersonalIssuerPaths = append(answer.PersonalIssuerPaths, text)
			}
		}
	}
	answer.PersonalDirectories = []string{}
	if directories, held := out["personal_directories"].([]any); held {
		for _, directory := range directories {
			if text, ok := directory.(string); ok {
				answer.PersonalDirectories = append(answer.PersonalDirectories, text)
			}
		}
	}
	return answer
}

// The relying-party flow's use cases.
const (
	startOidcSignInUseCase = "StartOidcSignIn"
	locateOidcFlowUseCase  = "LocateOidcFlow"
)

// outputStrings reads a list a use case built for every channel: `[]any` of strings, which is what
// the registry's projection is made of.
func outputStrings(value any) []string {
	entries, held := value.([]any)
	if !held {
		return nil
	}
	strings := make([]string, 0, len(entries))
	for _, entry := range entries {
		if text, held := entry.(string); held {
			strings = append(strings, text)
		}
	}
	return strings
}
