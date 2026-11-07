// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
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

// Usage is what the object was staged for. The schema also reserves EXPORT, which nothing stages.
type Usage string

const (
	UsageCover      Usage = "COVER"
	UsageAttachment Usage = "ATTACHMENT"
	// UsageImport is a file for POST /imports: it attaches to nothing, covers nothing, and is
	// deleted when the import's job ends.
	UsageImport Usage = "IMPORT"
	// UsageAvatar is a person's picture of themselves (PUT /accounts/me/avatar): read by every
	// member of the workspace, so it is bounded tighter than an attachment and must be an image a
	// browser renders inline.
	UsageAvatar Usage = "AVATAR"
	// UsageDeskLogo is a desk's own logo (PUT /containers/{id}/desk-logo): drawn on the portal for
	// every requester of the service, so it is bounded and judged as a picture of oneself is.
	UsageDeskLogo Usage = "DESK_LOGO"
)

// MaxAvatarBytes is the most a picture of oneself may be, whatever the installation allows for an
// attachment: it is drawn small beside a name on every screen the person appears on.
const MaxAvatarBytes int64 = 5_000_000

func knownUsage(usage Usage) bool {
	return usage == UsageCover || usage == UsageAttachment || usage == UsageImport ||
		usage == UsageAvatar || usage == UsageDeskLogo
}

// ParseUsage reads a submitted usage.
func ParseUsage(value string) (Usage, error) {
	usage := Usage(value)
	if !knownUsage(usage) {
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
	// StatusPendingScan is sealed but held: judged and measured, and the installation's scanner
	// has not answered for it yet. It may join an entry; its bytes are never served.
	StatusPendingScan Status = "PENDING_SCAN"
	// StatusInfected is what a scan of a held object found malicious: the bytes are gone and the
	// record stays for what pointed at it.
	StatusInfected Status = "INFECTED"
	// StatusUnscannable is a held object the scanner refused for itself - too large for its stream
	// limit - rather than for being down: no later pass would answer, so it is settled, never served.
	StatusUnscannable Status = "UNSCANNABLE"
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

	// Text brings the file name to normal form C before it is bounded and stored (i18n-l10n.md §5).
	// A file name is the text most likely to arrive decomposed at all: one file system writes names
	// that way, and a browser hands them on as they are.
	Text text.Normalizer
}

// NewPendingObject validates and stages an object.
// StorageKeyFor is where a medium's bytes live.
//
// Tenant-prefixed, so one bucket holds every tenant without a key collision ever being possible -
// and a listing of one prefix is one tenant's objects, nobody else's.
//
// A function rather than a line inside NewPendingObject because a restore has to mint the same key
// again for a row it writes back (backup-restore.md §8.3): an archive carries the key the medium
// had where it was taken, and a restore into another tenant that kept it would put two tenants'
// rows on one object - where deleting either one takes the other's bytes with it.
func StorageKeyFor(tenantID, id shared.ID) string {
	return "media/" + tenantID.String() + "/" + id.String()
}

func NewPendingObject(input NewObjectInput) (Object, error) {
	if input.DeclaredSize < 1 {
		return Object{}, shared.ErrValidation.
			WithDetail("media.size_required").
			WithFields(shared.FieldError{Path: "/size", Code: "media.size_required"})
	}
	if input.SizeLimit > 0 && input.DeclaredSize > input.SizeLimit {
		return Object{}, TooLarge(input.SizeLimit)
	}
	if (input.Usage == UsageAvatar || input.Usage == UsageDeskLogo) && input.DeclaredSize > MaxAvatarBytes {
		return Object{}, TooLarge(MaxAvatarBytes)
	}
	fileName, err := validFileName(input.FileName, input.Text)
	if err != nil {
		return Object{}, err
	}
	if !knownUsage(input.Usage) {
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
	if o.Status != StatusPending {
		return Object{}, shared.ErrConflict.WithDetail("media.already_confirmed")
	}
	if o.DeletedAt != nil {
		return Object{}, notFound()
	}

	o.Status = StatusReady
	o.ContentType = judgedType
	o.ByteSize = measuredSize
	return o, nil
}

// HeldForScan returns the object sealed and held for a scan: the judgement recorded, the bytes not served
// until a scan clears them (security.md §4, T-11).
func (o Object) HeldForScan(judgedType string, measuredSize int64) (Object, error) {
	sealed, err := o.Sealed(judgedType, measuredSize)
	if err != nil {
		return Object{}, err
	}
	sealed.Status = StatusPendingScan
	return sealed, nil
}

// Held reports whether the object waits for a scan.
func (o Object) Held() bool { return o.Status == StatusPendingScan }

// Scanned settles a held object by the scan's verdict: READY when clean, INFECTED when not.
func (o Object) Scanned(malicious bool) (Object, error) {
	if o.Status != StatusPendingScan {
		return Object{}, shared.ErrConflict.WithDetail("media.not_held")
	}
	o.Status = StatusReady
	if malicious {
		o.Status = StatusInfected
	}
	return o, nil
}

// Unscannable settles a held object the scanner refused for itself; its bytes stay, unserved.
func (o Object) Unscannable() (Object, error) {
	if o.Status != StatusPendingScan {
		return Object{}, shared.ErrConflict.WithDetail("media.not_held")
	}
	o.Status = StatusUnscannable
	return o, nil
}

// Attachable answers whether the object may join an item right now - as a cover or an
// attachment, which is what its usage says it was staged for.
func (o Object) Attachable(usage Usage) error {
	if o.DeletedAt != nil {
		return notFound()
	}
	if o.Status == StatusInfected {
		return Infected()
	}
	// A held object joins: what it is attached to serves nothing of it until a scan clears it, and
	// refusing it would turn a scanner that is down into uploads that are down.
	if o.Status != StatusReady && o.Status != StatusPendingScan {
		return notReady()
	}
	if o.Usage != usage {
		return shared.ErrValidation.
			WithDetail("media.usage_mismatch").
			WithParams(map[string]string{"usage": string(o.Usage), "wanted": string(usage)}).
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.usage_mismatch"})
	}
	return nil
}

// Portrayable answers whether the object may be shown as a person's picture: confirmed, staged as
// an avatar, and judged an image a browser renders without executing anything (T-11). Anything
// else would be served as a download, and a download is not a picture.
func (o Object) Portrayable() error {
	return o.drawable(UsageAvatar, "media.avatar_not_an_image")
}

// Emblem answers whether the object may be shown as a desk's logo, by Portrayable's rules: every
// requester's browser draws it.
func (o Object) Emblem() error {
	return o.drawable(UsageDeskLogo, "media.logo_not_an_image")
}

func (o Object) drawable(usage Usage, notAnImage string) error {
	if err := o.Attachable(usage); err != nil {
		return err
	}
	if o.Status != StatusReady {
		// Drawn for others: a picture is drawn, so a held one is not one yet.
		return notReady()
	}
	if DeliveryFor(o.ContentType) != DispositionInline {
		return shared.ErrValidation.
			WithDetail(notAnImage).
			WithFields(shared.FieldError{Path: "/media_id", Code: notAnImage})
	}
	return nil
}

// Infected is the refusal of a file a scan found malicious.
func Infected() error {
	return shared.ErrValidation.
		WithDetail("media.infected").
		WithFields(shared.FieldError{Path: "/media_id", Code: "media.infected"})
}

// TooLargeToScan is the scanner's refusal of one file for its size: the file, not the scanner, is
// the cause, so it is neither held for a later pass nor passed unscanned.
func TooLargeToScan() error {
	return shared.ErrValidation.
		WithDetail("media.too_large_to_scan").
		WithFields(shared.FieldError{Path: "/media_id", Code: "media.too_large_to_scan"})
}

// IsTooLargeToScan reports whether err is TooLargeToScan.
func IsTooLargeToScan(err error) bool {
	return err != nil && shared.AsError(err).DetailCode == "media.too_large_to_scan"
}

func notReady() error {
	return shared.ErrValidation.
		WithDetail("media.not_ready").
		WithFields(shared.FieldError{Path: "/media_id", Code: "media.not_ready"})
}

// notFound is the one answer for an object on its way out, and it carries a code because the
// alternative is what a bare category produces: "This entry does not exist.", the sentence every
// not-found in the system falls back to. A person told that about a file they have just uploaded
// learns nothing, and the log of the installation that told them learns no more (rule 8).
func notFound() error {
	return shared.ErrNotFound.WithDetail("media.not_found")
}

// validFileName keeps the name a name: bounded, on one line, and never path material. The
// separators are refused rather than stripped - a caller that sent "a/b" meant something this
// field does not store.
func validFileName(name string, form text.Normalizer) (string, error) {
	trimmed, err := shared.NFC(strings.TrimSpace(name), form)
	if err != nil {
		return "", err
	}
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
