// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package shared holds the vocabulary every bounded context needs: identifiers, locales, and
// the error model.
//
// Errors are typed values, never strings (arc42 §8.11). A caller decides on the type, not by
// matching on a message - a message is display text, and display text does not exist in the
// backend (ADR-0011). What travels instead is a stable code plus parameters; the client turns
// that into a sentence in its own language.
package shared

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Category is the coarse classification the adapters map to a protocol. The application layer
// assigns it; an adapter never invents one.
//
// arc42 §8.11 names the first six. UNAUTHENTICATED, GONE and UNAVAILABLE follow from the status
// mapping in api-guidelines.md §6, which requires 401, 410 and 503 - none of which any of the six
// can produce.
type Category string

const (
	// CategoryValidation is input the domain rejects: a malformed field, a broken invariant,
	// a capability the item type does not have.
	CategoryValidation Category = "VALIDATION"
	// CategoryNotFound is a resource that does not exist, or that the actor may not know exists.
	CategoryNotFound Category = "NOT_FOUND"
	// CategoryConflict is a clash with the current state: a duplicate, a stale version, a cycle
	// in the hierarchy.
	CategoryConflict Category = "CONFLICT"
	// CategoryForbidden is an authenticated actor without the permission for this operation.
	CategoryForbidden Category = "FORBIDDEN"
	// CategoryUnauthenticated is a missing, expired, or unreadable credential.
	CategoryUnauthenticated Category = "UNAUTHENTICATED"
	// CategoryGone is a resource that existed and was permanently deleted. Distinct from
	// NOT_FOUND, because the difference is the whole point for a synchronising client.
	CategoryGone Category = "GONE"
	// CategoryRateLimited is a limit reached: per IP, per token, per tenant.
	CategoryRateLimited Category = "RATE_LIMITED"
	// CategoryUnavailable is a dependency that is unreachable or deliberately degraded. It says
	// "later", not "wrong" - the client may retry (ADR-0016).
	CategoryUnavailable Category = "UNAVAILABLE"
	// CategoryInternal is a defect. Everything unclassified lands here, and nothing about it
	// reaches the client beyond the code and the request ID (security.md §9).
	CategoryInternal Category = "INTERNAL"
)

// categories is the closed set, in the order of the constants above. Everything that needs to
// know "which categories exist" reads it here - a second list somewhere else is a second list to
// forget, and the one that gets forgotten is always the newest category.
var categories = [...]Category{
	CategoryValidation, CategoryNotFound, CategoryConflict, CategoryForbidden,
	CategoryUnauthenticated, CategoryGone, CategoryRateLimited, CategoryUnavailable,
	CategoryInternal,
}

// Categories returns every defined category. An adapter uses it to prove it handles all of them -
// the observability layer derives its `result` label from this set, so a new category shows up in
// the metrics by itself instead of being silently folded into INTERNAL
// (observability-reliability.md §4.1).
func Categories() []Category { return slices.Clone(categories[:]) }

// Valid reports whether the category is one of the defined ones. An adapter uses this to decide
// on a status; an unknown category must be treated as INTERNAL rather than silently passed on.
func (c Category) Valid() bool { return slices.Contains(categories[:], c) }

// Error is the error type of the domain and the application layer.
//
// Code is part of the API contract: stable, machine-readable, and removing one is a breaking
// change (versioning-release.md §2). DetailCode plus Params are what the client localises -
// see locales/en.json.
type Error struct {
	Category Category
	// Code is the coarse, stable contract code: validation_failed, not_found, conflict.
	Code string
	// DetailCode names the concrete case: items.cover_not_supported_for_type. Optional; without
	// it the client falls back to Code.
	DetailCode string
	// Params carry the values of the message, never a finished sentence (ADR-0011).
	Params map[string]string
	// Fields lists per-field findings for a validation error.
	Fields []FieldError
	// cause keeps the technical origin for the log. It never leaves the process: an adapter
	// reads Code, never Unwrap.
	cause error
}

// FieldError points at one field of the request. Path is a JSON Pointer (RFC 6901), so that a
// client can highlight the input that is wrong.
type FieldError struct {
	Path   string
	Code   string
	Params map[string]string
}

// Error implements the error interface. The text is for the log, not for a user - which is why
// it consists of codes rather than prose.
func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Category))
	b.WriteString(": ")
	b.WriteString(e.Code)
	if e.DetailCode != "" {
		b.WriteString(" (")
		b.WriteString(e.DetailCode)
		b.WriteString(")")
	}
	// Parameters are ordered, because an error message that varies between runs is unusable in
	// a test and unpleasant in a log.
	if len(e.Params) > 0 {
		b.WriteString(" [")
		for i, k := range slices.Sorted(maps.Keys(e.Params)) {
			if i > 0 {
				b.WriteString(" ")
			}
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(e.Params[k])
		}
		b.WriteString("]")
	}
	if e.cause != nil {
		b.WriteString(": ")
		b.WriteString(e.cause.Error())
	}
	return b.String()
}

// Unwrap exposes the technical cause to errors.Is and errors.As. It is deliberately not part of
// what an adapter renders.
func (e *Error) Unwrap() error { return e.cause }

// Is makes two errors of this type comparable by their code, so that a caller can write
// errors.Is(err, shared.ErrNotFound) without knowing the concrete instance.
func (e *Error) Is(target error) bool {
	var other *Error
	if !errors.As(target, &other) {
		return false
	}
	if other.Code != "" && other.Code != e.Code {
		return false
	}
	return other.Category == "" || other.Category == e.Category
}

// WithParams returns a copy carrying the given parameters. The receiver stays untouched, so that
// package-level sentinels cannot be modified by a caller.
func (e *Error) WithParams(params map[string]string) *Error {
	out := *e
	out.Params = maps.Clone(params)
	return &out
}

// WithDetail returns a copy with the concrete case named.
func (e *Error) WithDetail(detailCode string) *Error {
	out := *e
	out.DetailCode = detailCode
	return &out
}

// WithCause returns a copy carrying the technical origin for the log.
func (e *Error) WithCause(cause error) *Error {
	out := *e
	out.cause = cause
	return &out
}

// WithFields returns a copy carrying per-field findings.
func (e *Error) WithFields(fields ...FieldError) *Error {
	out := *e
	out.Fields = slices.Clone(fields)
	return &out
}

// New builds an error of the given category. Prefer one of the constructors below - they use the
// codes from the contract in api-guidelines.md §6.
func New(category Category, code string) *Error {
	return &Error{Category: category, Code: code}
}

// The contract codes from api-guidelines.md §6. They are sentinels and constructors at once:
// comparable with errors.Is, and a starting point for WithParams/WithDetail.
var (
	ErrValidation       = New(CategoryValidation, "validation_failed")
	ErrNotFound         = New(CategoryNotFound, "not_found")
	ErrConflict         = New(CategoryConflict, "conflict")
	ErrVersionConflict  = New(CategoryConflict, "version_conflict")
	ErrForbidden        = New(CategoryForbidden, "forbidden")
	ErrUnauthenticated  = New(CategoryUnauthenticated, "unauthenticated")
	ErrGone             = New(CategoryGone, "gone")
	ErrRateLimited      = New(CategoryRateLimited, "rate_limited")
	ErrUnavailable      = New(CategoryUnavailable, "dependency_unavailable")
	ErrMalformedRequest = New(CategoryValidation, "malformed_request")
	ErrInternal         = New(CategoryInternal, "internal")
	// ErrCapabilityNotSupported is a field set on an item type whose profile does not carry the
	// capability (ADR-0006, domain-model.md §2). Its own code rather than a validation_failed,
	// because the answer to it is different: the request is well formed and the field is real,
	// and what a client has to change is the type it is writing to - which is worth telling it in
	// the code rather than only in a field error.
	ErrCapabilityNotSupported = New(CategoryValidation, "capability_not_supported")
)

// AsError classifies any error for an adapter. An error that is not of this type is a defect and
// becomes INTERNAL - deliberately without its text, because the text of an unknown error may
// contain anything, including a connection string (security.md §9).
func AsError(err error) *Error {
	if err == nil {
		return nil
	}
	var domainErr *Error
	if errors.As(err, &domainErr) {
		if !domainErr.Category.Valid() {
			return ErrInternal.WithCause(err)
		}
		return domainErr
	}
	return ErrInternal.WithCause(err)
}

// Validation builds a validation error for one field.
func Validation(path, code string, params map[string]string) *Error {
	return ErrValidation.WithFields(FieldError{Path: path, Code: code, Params: maps.Clone(params)})
}

// Internalf builds an internal error from a technical cause. The format string is for the log;
// nothing of it reaches the client.
func Internalf(format string, args ...any) *Error {
	return ErrInternal.WithCause(fmt.Errorf(format, args...))
}
