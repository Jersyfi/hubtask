// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import "net/http"

// ShownOnceRoutes are the operations whose every successful answer carries a credential shown once
// - a token, a secret, recovery codes, a start password, a session's tokens. The idempotency record
// of such an answer keeps its status and no body, as ShownOnceHeader marks it for a single answer:
// a replay record is a copy of the credential in clear, and the database and its backups would carry
// it (T-18; security.md §8).
//
// A table rather than a header in each handler, because the class belongs to the operation and a
// table can be held to the contract: the contract test derives the credential-bearing answers from
// api/openapi.yaml and fails for one missing here. An answer that carries a credential only
// sometimes - an invitation shows its link only where no mail goes - stays the handler's to mark.
var ShownOnceRoutes = map[string]bool{
	// Sessions: the access and refresh tokens, or the second factor's pending token.
	http.MethodPost + " " + APIBasePath + "/auth/sessions":              true,
	http.MethodPost + " " + APIBasePath + "/auth/sessions:refresh":      true,
	http.MethodPost + " " + APIBasePath + "/auth/sessions:verify":       true,
	http.MethodPost + " " + APIBasePath + "/auth/sessions:link":         true,
	http.MethodPost + " " + APIBasePath + "/auth/sessions:agree-terms":  true,
	http.MethodPost + " " + APIBasePath + "/auth/sessions:set-password": true,
	http.MethodPost + " " + APIBasePath + "/auth/invitations:redeem":    true,
	http.MethodPost + " " + APIBasePath + "/auth/installation:set-up":   true,
	http.MethodPost + " " + APIBasePath + "/auth/password:reset":        true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/totp:confirm":      true,
	http.MethodPost + " " + APIBasePath + "/auth/oidc:callback":         true,
	http.MethodPost + " " + APIBasePath + "/oauth/token":                true,
	http.MethodPost + " " + APIBasePath + "/auth/step-up":               true,
	http.MethodPost + " " + APIBasePath + "/oauth/authorize":            true,
	// Tokens of machines and links.
	http.MethodPost + " " + APIBasePath + "/auth/tokens":                                    true,
	http.MethodPost + " " + APIBasePath + "/automation/rules/{ruleId}:rotate-inbound-token": true,
	http.MethodPost + " " + APIBasePath + "/jumble/intake:rotate-token":                     true,
	http.MethodPost + " " + APIBasePath + "/integrations/calendar-feeds":                    true,
	// Secrets and second-factor material.
	http.MethodPost + " " + APIBasePath + "/integrations/webhooks":                           true,
	http.MethodPost + " " + APIBasePath + "/integrations/webhooks/{webhookId}:rotate-secret": true,
	http.MethodPost + " " + APIBasePath + "/oauth/clients":                                   true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/totp:enroll":                            true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/totp:replace":                           true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/totp/replacement:confirm":               true,
	http.MethodPost + " " + APIBasePath + "/auth/mfa/recovery:regenerate":                    true,
	// Start passwords.
	http.MethodPost + " " + APIBasePath + "/accounts:create-managed":                   true,
	http.MethodPost + " " + APIBasePath + "/accounts/{accountId}:issue-start-password": true,
}

// ShownOnceByHandler are the operations whose answer carries a credential only sometimes; their
// handler marks the answer that does (ShownOnceHeader). Named here so the contract test can tell
// them from a route somebody forgot.
var ShownOnceByHandler = map[string]string{
	http.MethodPost + " " + APIBasePath + "/accounts:invite":                       "the link only where no mail is sent",
	http.MethodPost + " " + APIBasePath + "/accounts/{accountId}:renew-invitation": "the link only where no mail is sent",
}

// ShownOnceKept are the operations whose answer carries a credential and whose replay record still
// keeps it, each with the reason; the contract test accepts them by name. Provisioning is one: a
// platform repeats the call because it lost the answer, and the workspace's identifier in the replay
// is how it learns which workspace it made (UC-INS-06 check 3). The owner's redemption token in the
// copy works once, and the copy goes with the record (the IDEMPOTENCY_KEY sweep).
var ShownOnceKept = map[string]string{
	http.MethodPost + " " + APIBasePath + "/admin/tenants": "a repeat answers the same workspace (UC-INS-06 check 3)",
}
