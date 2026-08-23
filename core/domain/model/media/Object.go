// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Object is one uploaded file: the record beside the bytes (domain-model.md §3.5, arc42 §5.2).
//
// The bytes live in object storage under StorageKey and are shared, never copied: a cover and
// three attachments of the same file are four references to one object, which is what RefCount
// counts and what the reconciliation job decides deletion by (data-protection.md §5). The record
// is what the API serves; the bytes only ever leave through a download target.
type Object struct {
	ID       shared.ID
	TenantID shared.ID
	// StorageKey is where the bytes live: minted, tenant-prefixed, never user text.
	StorageKey string
	// FileName is the name the file arrived under, kept for the download and nothing else - it
	// is user content and never becomes a path (T-11, rule 10 keeps it out of logs).
	FileName string
	// ContentType is the claim while PENDING and the judged type once READY - sniffed from the
	// bytes, never the client's word (T-11). Delivery decides from it.
	ContentType string
	// ByteSize is declared at staging and measured at confirmation.
	ByteSize int64
	// Checksum is reserved: nothing computes it yet, and a value nothing verifies would be a
	// promise nothing keeps. The column is nullable for the same reason.
	Checksum  string
	Usage     Usage
	Status    Status
	RefCount  int
	CreatedBy shared.ID
	CreatedAt time.Time
	// DeletedAt is the reconciliation job's mark: an orphan waiting out its grace period. A
	// marked object serves nothing and joins nothing.
	DeletedAt *time.Time
}

// Usage is what the object was staged for. The schema also reserves IMPORT and EXPORT; they
// arrive with the milestones that own them.
type Usage string

const (
	UsageCover      Usage = "COVER"
	UsageAttachment Usage = "ATTACHMENT"
)

// ParseUsage reads a submitted usage.
func ParseUsage(value string) (Usage, error) {
	usage := Usage(value)
	if usage != UsageCover && usage != UsageAttachment {
		return "", shared.ErrValidation.
			WithDetail("media.usage_unknown").
			WithParams(map[string]string{"value": value}).
			WithFields(shared.FieldError{Path: "/usage", Code: "media.usage_unknown"})
	}
	return usage, nil
}

// Status is where the object stands in its upload life.
type Status string

const (
	// StatusPending is staged: a record exists, the bytes may or may not.
	StatusPending Status = "PENDING"
	// StatusReady is sealed: the bytes were read back, judged and measured.
	StatusReady Status = "READY"
)

// MaxFileNameLength counts code points, for the reason every length here does (I-W7).
const MaxFileNameLength = 255

// NewObjectInput is what a staging needs decided.
type NewObjectInput struct {
	ID       shared.ID
	TenantID shared.ID
	FileName string
	// ClaimedType is what the sender says the bytes are. Kept on the PENDING record so the
	// confirmation can hold the claim against the sniff; an empty claim is allowed.
	ClaimedType string
	// DeclaredSize is the exact size the upload will have, in bytes.
	DeclaredSize int64
	// SizeLimit is the installation's HUBTASK_MAX_UPLOAD_BYTES.
	SizeLimit int64
	Usage     Usage
	CreatedBy shared.ID
	Now       time.Time
}

// NewPendingObject validates and stages an object.
func NewPendingObject(input NewObjectInput) (Object, error) {
	if input.DeclaredSize < 1 {
		return Object{}, shared.ErrValidation.
			WithDetail("media.size_required").
			WithFields(shared.FieldError{Path: "/size", Code: "media.size_required"})
	}
	if input.SizeLimit > 0 && input.DeclaredSize > input.SizeLimit {
		return Object{}, TooLarge(input.SizeLimit)
	}
	fileName, err := validFileName(input.FileName)
	if err != nil {
		return Object{}, err
	}
	if input.Usage != UsageCover && input.Usage != UsageAttachment {
		return Object{}, shared.ErrValidation.
			WithDetail("media.usage_unknown").
			WithParams(map[string]string{"value": string(input.Usage)}).
			WithFields(shared.FieldError{Path: "/usage", Code: "media.usage_unknown"})
	}

	return Object{
		ID:       input.ID,
		TenantID: input.TenantID,
		// Tenant-prefixed, so one bucket holds every tenant without a key collision ever being
		// possible - and a listing of one prefix is one tenant's objects, nobody else's.
		StorageKey:  "media/" + input.TenantID.String() + "/" + input.ID.String(),
		FileName:    fileName,
		ContentType: input.ClaimedType,
		ByteSize:    input.DeclaredSize,
		Usage:       input.Usage,
		Status:      StatusPending,
		CreatedBy:   input.CreatedBy,
		CreatedAt:   input.Now,
	}, nil
}

// Sealed returns the object READY, carrying the judged type and the measured size. The
// judgement happened outside: the confirmation read the bytes back and ran them through the
// upload guard, and this only records its answer.
//
// Idempotent by refusal: sealing a READY object is the caller's signal to answer with what is
// there rather than to judge again, and the ErrConflict tells it apart from success.
func (o Object) Sealed(judgedType string, measuredSize int64) (Object, error) {
	if o.Status == StatusReady {
		return Object{}, shared.ErrConflict.WithDetail("media.already_confirmed")
	}
	if o.DeletedAt != nil {
		return Object{}, shared.ErrNotFound
	}

	o.Status = StatusReady
	o.ContentType = judgedType
	o.ByteSize = measuredSize
	return o, nil
}

// Attachable answers whether the object may join an item right now - as a cover or an
// attachment, which is what its usage says it was staged for.
func (o Object) Attachable(usage Usage) error {
	if o.DeletedAt != nil {
		return shared.ErrNotFound
	}
	if o.Status != StatusReady {
		return shared.ErrValidation.
			WithDetail("media.not_ready").
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.not_ready"})
	}
	if o.Usage != usage {
		return shared.ErrValidation.
			WithDetail("media.usage_mismatch").
			WithParams(map[string]string{"usage": string(o.Usage), "wanted": string(usage)}).
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.usage_mismatch"})
	}
	return nil
}

// validFileName keeps the name a name: bounded, on one line, and never path material. The
// separators are refused rather than stripped - a caller that sent "a/b" meant something this
// field does not store.
func validFileName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", nil
	}
	if utf8.RuneCountInString(trimmed) > MaxFileNameLength {
		return "", shared.ErrValidation.
			WithDetail("media.file_name_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxFileNameLength)}).
			WithFields(shared.FieldError{Path: "/file_name", Code: "media.file_name_too_long"})
	}
	if strings.ContainsAny(trimmed, "/\\") || strings.ContainsFunc(trimmed, unicode.IsControl) {
		return "", shared.ErrValidation.
			WithDetail("media.file_name_invalid").
			WithFields(shared.FieldError{Path: "/file_name", Code: "media.file_name_invalid"})
	}
	return trimmed, nil
}
