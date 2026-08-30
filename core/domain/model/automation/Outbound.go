// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ActionHTTPRequest is the outbound call (G-09, automation.md §1.3) - the riskiest surface in the
// milestone, and the reason its parameters are read here in full rather than left to the run: a
// URL nobody can dial and a method nobody serves are answers their author needs at the write, and
// unlike a use case's parameters, none of these can be supplied by the run later.
const ActionHTTPRequest = "HTTP_REQUEST"

// HTTPMethods is the closed set an outbound call may use. No CONNECT, no TRACE, no arbitrary
// verbs: the five that mean something to the APIs people wire rules to.
var HTTPMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}

// SecretMask is what a stored secret reads as everywhere after creation - in the rule as every
// channel answers it, and in what a client sends back unchanged on an edit to mean "keep it".
const SecretMask = "***"

// SealedSecret is a header secret as the rule stores it: ciphertext under E-02's sealing, never
// the value. The purpose binds it to the rule it belongs to, so a ciphertext lifted out of one
// rule and dropped into another no longer opens.
type SealedSecret struct {
	// Ciphertext is base64, because it lives inside the rule's JSON document.
	Ciphertext string
	KeyID      string
	Purpose    string
}

// HTTPRequest is what an HTTP_REQUEST action says, read out of its parameters.
type HTTPRequest struct {
	Method string
	URL    string
	// Headers are the plain ones - a content type, an API version. A credential does not belong
	// here: it belongs in the secret header, which is stored sealed and masked everywhere.
	Headers map[string]string
	// SecretHeaderName is the header the secret travels in, when it travels as one.
	SecretHeaderName string
	// SecretValue is the plaintext, present only on the way in - a fresh write, or a direct call.
	// Stored it never is: the writer seals it and Sealed replaces it.
	SecretValue string
	// SecretMasked reports that the caller sent the mask, asking for the stored secret to be kept.
	SecretMasked bool
	Sealed       *SealedSecret
	// SignatureHeader, when set, carries an HMAC-SHA256 over the body, computed with the secret as
	// the key - the same t=<ts>,v1=<hex> shape a webhook signature has, so a receiver that
	// verifies one can verify the other.
	SignatureHeader string
	// BodyTemplate is a CEL expression producing text, evaluated against the run's event when the
	// call is made - which is what lets a retry two days later send what the first attempt would
	// have. A static body is a string literal.
	BodyTemplate string
}

// HasSecret reports whether the request carries a secret in any form.
func (r HTTPRequest) HasSecret() bool {
	return r.SecretValue != "" || r.SecretMasked || r.Sealed != nil
}

// ReadHTTPRequest reads an HTTP_REQUEST's parameters, or says which field is wrong.
//
// The path is the action's own, so a refusal inside a branch points at the line an editor has to
// put the cursor on.
func ReadHTTPRequest(params map[string]any, path string) (HTTPRequest, error) {
	request := HTTPRequest{}
	var findings []shared.FieldError

	method, _ := params["method"].(string)
	request.Method = strings.ToUpper(strings.TrimSpace(method))
	switch {
	case request.Method == "":
		findings = append(findings, shared.FieldError{
			Path: path + "/params/method", Code: "automation.http_method_required",
		})
	case !contains(HTTPMethods, request.Method):
		findings = append(findings, shared.FieldError{
			Path: path + "/params/method", Code: "automation.http_method_unknown",
		})
	}

	rawURL, _ := params["url"].(string)
	request.URL = strings.TrimSpace(rawURL)
	switch {
	case request.URL == "":
		findings = append(findings, shared.FieldError{
			Path: path + "/params/url", Code: "automation.http_url_required",
		})
	case !strings.HasPrefix(request.URL, "https://") && !strings.HasPrefix(request.URL, "http://"):
		// The shape question only. Whether the address resolves somewhere this installation is
		// willing to dial is the guard's question, answered at the call - an allowlist decision
		// does not belong in a domain model.
		findings = append(findings, shared.FieldError{
			Path: path + "/params/url", Code: "automation.http_url_invalid",
		})
	}

	if raw, present := params["headers"]; present && raw != nil {
		document, ok := raw.(map[string]any)
		if !ok {
			findings = append(findings, shared.FieldError{
				Path: path + "/params/headers", Code: "automation.http_headers_invalid",
			})
		} else {
			request.Headers = make(map[string]string, len(document))
			for name, value := range document {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(name) == "" {
					findings = append(findings, shared.FieldError{
						Path: path + "/params/headers/" + name, Code: "automation.http_headers_invalid",
					})
					continue
				}
				request.Headers[name] = text
			}
		}
	}

	request.SecretHeaderName, _ = params["secret_header_name"].(string)
	request.SecretHeaderName = strings.TrimSpace(request.SecretHeaderName)
	request.SignatureHeader, _ = params["signature_header"].(string)
	request.SignatureHeader = strings.TrimSpace(request.SignatureHeader)
	request.BodyTemplate, _ = params["body_template"].(string)

	value, _ := params["secret_header_value"].(string)
	if value == SecretMask {
		request.SecretMasked = true
	} else if value != "" {
		request.SecretValue = value
	}
	if sealed, present := params["secret_header_sealed"]; present && sealed != nil {
		read, err := readSealed(sealed, path)
		if err != nil {
			return HTTPRequest{}, err
		}
		request.Sealed = &read
	}

	// The secret's consistency. Each rule catches a shape whose author believes something that is
	// not true: a secret that is never used, a signature with no key to compute it, or a value
	// beside a ciphertext where nobody can say which one is meant.
	switch {
	case request.SecretValue != "" && request.Sealed != nil:
		findings = append(findings, shared.FieldError{
			Path: path + "/params/secret_header_value", Code: "automation.http_secret_conflict",
		})
	case request.HasSecret() && request.SecretHeaderName == "" && request.SignatureHeader == "":
		findings = append(findings, shared.FieldError{
			Path: path + "/params/secret_header_name", Code: "automation.http_secret_unused",
		})
	case !request.HasSecret() && (request.SecretHeaderName != "" || request.SignatureHeader != ""):
		findings = append(findings, shared.FieldError{
			Path: path + "/params/secret_header_value", Code: "automation.http_secret_required",
		})
	}

	if len(findings) > 0 {
		return HTTPRequest{}, shared.ErrValidation.
			WithDetail("automation.http_request_invalid").
			WithFields(findings...)
	}
	return request, nil
}

// readSealed reads the stored form back. Defensive, because the document outlives the release
// that wrote it.
func readSealed(raw any, path string) (SealedSecret, error) {
	document, ok := raw.(map[string]any)
	if !ok {
		return SealedSecret{}, fieldError(
			path+"/params/secret_header_sealed", "automation.http_sealed_invalid")
	}
	sealed := SealedSecret{}
	sealed.Ciphertext, _ = document["ciphertext"].(string)
	sealed.KeyID, _ = document["key_id"].(string)
	sealed.Purpose, _ = document["purpose"].(string)
	if sealed.Ciphertext == "" || sealed.KeyID == "" || sealed.Purpose == "" {
		return SealedSecret{}, fieldError(
			path+"/params/secret_header_sealed", "automation.http_sealed_invalid")
	}
	return sealed, nil
}

// Document is the sealed secret as the rule's JSON stores it.
func (s SealedSecret) Document() map[string]any {
	return map[string]any{"ciphertext": s.Ciphertext, "key_id": s.KeyID, "purpose": s.Purpose}
}

func contains(list []string, value string) bool {
	for _, entry := range list {
		if entry == value {
			return true
		}
	}
	return false
}
