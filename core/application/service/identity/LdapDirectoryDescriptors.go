// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ldapStepUp is the descriptors' sentence for the proof a change to the directory demands.
const ldapStepUp = "changing or removing the directory, and confirming a run past its switch-off guard"

var ldapExempt = usecase.ActivityDeclaration{
	Exempt: "A workspace's directory configuration is not an entry, and the item history is keyed on an entry.",
}

// Descriptor is the catalogue entry.
func (h ShowLdapDirectory) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ShowLdapDirectoryName,
		Permission: service.PermissionManageMembers,
		Summary: "The LDAP directory the workspace's people and groups are synchronised from: where it is, " +
			"what is read, the service account the run acts as, when it runs, and the last run's outcome and " +
			"counts. The bind password is never among the fields.",
		SideEffects: "None. Reads only; a run more than a day overdue is queued again.",
		TokenScope:  identityProviderManage,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: LdapReadAction, TargetType: ldapTarget, Severity: audit.SeverityInfo,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ConfigureLdapDirectory) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ConfigureLdapDirectoryName,
		Permission:  service.PermissionManageMembers,
		Entitlement: entitlement.OwnSignInProviders,
		Summary: "Sets the LDAP directory the workspace synchronises its people and groups from, whole. It is " +
			"bound to once before anything is stored, over ldaps or StartTLS only; the password is sealed and " +
			"omitting it keeps the sealed one. The run acts as the named service account, which needs to manage " +
			"the workspace's members.",
		SideEffects: "Stores the configuration, seals the password, queues the first run and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Input: []usecase.Field{
			{Name: "url", Kind: usecase.KindString, Required: true, Description: "ldaps://host[:port], or ldap://host[:port] upgraded by StartTLS."},
			{Name: "bind_dn", Kind: usecase.KindString, Required: true, Description: "The account the directory is read as."},
			{Name: "bind_password", Kind: usecase.KindString, Description: "Sealed and never answered. Required the first time; absent keeps the sealed one."},
			{Name: "user_base", Kind: usecase.KindString, Required: true, Description: "Where the people are searched, subtree."},
			{Name: "user_filter", Kind: usecase.KindString, Description: "Which entries are people; (objectClass=person) when absent."},
			{Name: "group_base", Kind: usecase.KindString, Description: "Where the groups are searched; absent synchronises no groups."},
			{Name: "group_filter", Kind: usecase.KindString, Description: "Which entries are groups; (objectClass=groupOfNames) when absent."},
			{Name: "id_attribute", Kind: usecase.KindString, Description: "The attribute that never changes; entryUUID when absent, objectGUID for Active Directory."},
			{Name: "mail_attribute", Kind: usecase.KindString, Description: "The address; mail when absent."},
			{Name: "name_attribute", Kind: usecase.KindString, Description: "The name shown; displayName when absent."},
			{Name: "group_name_attribute", Kind: usecase.KindString, Description: "A group's name; cn when absent."},
			{Name: "member_attribute", Kind: usecase.KindString, Description: "A group's members by their entries' names; member when absent."},
			{Name: "run_as", Kind: usecase.KindID, Required: true, Description: "The service account the run acts as."},
			{Name: "every_minutes", Kind: usecase.KindInt, Description: "How often it runs, 15 to 1440; 60 when absent."},
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether it runs on its own; true when absent."},
			ProviderStepUpField,
		},
		StepUp: ldapStepUp,
		Audit: usecase.AuditDeclaration{
			Action: LdapConfiguredAction, TargetType: ldapTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: ldapExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h RemoveLdapDirectory) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RemoveLdapDirectoryName,
		Permission: service.PermissionManageMembers,
		Summary: "Stops synchronising from the LDAP directory: the configuration and its sealed password go. " +
			"The people and groups it provisioned stay as they are.",
		SideEffects: "Deletes the configuration and writes an audit entry.",
		TokenScope:  identityProviderManage,
		Input:       []usecase.Field{ProviderStepUpField},
		StepUp:      ldapStepUp,
		Audit: usecase.AuditDeclaration{
			Action: LdapRemovedAction, TargetType: ldapTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: ldapExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h SyncLdapDirectory) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        SyncLdapDirectoryName,
		Permission:  service.PermissionManageMembers,
		Entitlement: entitlement.OwnSignInProviders,
		Summary: "Synchronises from the LDAP directory now, as its service account: invites who is new, renames, " +
			"switches off who left - never erases, never an owner - and on who returned, and keeps the groups' " +
			"members. Refused, changing nothing, when the directory answers nobody or would switch off more " +
			"than ten people and a fifth of those it holds, unless confirm_switch_off gives that count back.",
		SideEffects: "Invites, renames and switches people off and on, changes groups, records the run and writes an audit entry; nothing with dry_run.",
		TokenScope:  membersWrite,
		Input: []usecase.Field{
			{Name: "dry_run", Kind: usecase.KindBool, Description: "Read and plan only."},
			{Name: "confirm_switch_off", Kind: usecase.KindInt, Description: "The count a refused run would switch off, given back to let it through; needs a fresh step-up."},
			ProviderStepUpField,
		},
		StepUp: ldapStepUp,
		Audit: usecase.AuditDeclaration{
			Action: LdapSyncedAction, TargetType: ldapTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: ldapExempt,
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}
