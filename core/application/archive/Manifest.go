// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package archive is the Hubtask archive format of backup-restore.md §3: what a backup writes and
// what a restore reads, as a reader and a writer over the backup target port.
//
// It lives in the application layer rather than in the domain because an archive is a wire format.
// A domain object does not serialise itself (project-structure.md §3), and the thing this package
// is about is exactly the serialisation - which field is called what, in which order the members
// land, and what a reader of version 2 is allowed to assume about a file written by version 1.
//
// The format is a commitment rather than an implementation detail, which is why it is its own task
// and comes before the job that writes one on a timer. An archive written this month has to open in
// a version nobody has written yet; a change here that a reader cannot see coming is a restore that
// fails on the day it is needed.
package archive

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// FormatVersion is the version of the archive format this build writes.
//
// It is not the schema version and not the product version, and keeping the three apart is the
// point: the schema version says which migrations the data has taken, the product version says
// which build produced the file, and this says how the file is put together. A patch release
// changes the second, a migration changes the first, and this one changes when a reader that did
// not know about the change would misread the file.
const FormatVersion = 1

// MinimumReadableFormatVersion is the oldest format this build still opens. It is 1 and will stay
// 1 for as long as there is any archive of version 1 anywhere - raising it is a decision about
// somebody else's data, taken in a release note rather than in a constant.
const MinimumReadableFormatVersion = 1

// The names of the members. Fixed strings rather than a computed layout: a restore in a later
// version looks for these exact keys, and a reader that had to derive them would be a reader that
// could derive them differently.
const (
	// ManifestName is what an archive says about itself. It is written second to last, and it is
	// what `GET /backup-targets/{id}/backups` reads at the target when the database is gone
	// (backup-restore.md §8.1).
	ManifestName = "manifest.json"
	// ChecksumsName is the commit point of an archive. See Checksums.go for why it is this file
	// and not the manifest.
	ChecksumsName = "checksums.txt"
	// DataPrefix holds one JSON Lines file per aggregate.
	DataPrefix = "data/"
	// MediaPrefix holds the media, addressed by the SHA-256 of their content.
	MediaPrefix = "media/"
)

// Mode is what an archive holds: everything, or what changed since its parent.
type Mode string

const (
	// ModeFull is a self-contained archive. It needs no parent and restores on its own.
	ModeFull Mode = "FULL"
	// ModeIncremental holds what changed since the parent it names, including the deletions.
	// Without its chain back to a full archive it restores nothing.
	ModeIncremental Mode = "INCREMENTAL"
)

func (m Mode) Valid() bool { return m == ModeFull || m == ModeIncremental }

// suffix is what the mode contributes to a directory name, lower case, as §3 spells it.
func (m Mode) suffix() string { return strings.ToLower(string(m)) }

// ScopeKind is what an archive covers.
type ScopeKind string

const (
	// ScopeTenant is a whole tenant - the only scope a schedule can be configured with today
	// (backup-restore.md §5).
	ScopeTenant ScopeKind = "TENANT"
	// ScopeContainer is one hub or collection and what hangs below it. Not written by anything
	// yet; named here because the manifest field is read by a restore that must not have to guess
	// what an unfamiliar value means.
	ScopeContainer ScopeKind = "CONTAINER"
)

func (k ScopeKind) Valid() bool { return k == ScopeTenant || k == ScopeContainer }

// Scope is what the archive covers, in the shape §5 writes it in a schedule.
type Scope struct {
	Kind ScopeKind `json:"kind"`
	ID   string    `json:"id"`
}

// Period is the stretch of time an archive accounts for.
//
// From is exclusive and To is inclusive, which is what makes a chain of incrementals join up
// without a gap and without an overlap: the next run's From is this run's To. On a full archive
// From is the zero time, because a full archive accounts for everything that ever happened.
type Period struct {
	From time.Time `json:"from,omitempty"`
	To   time.Time `json:"to"`
}

// Encryption describes how the members were protected, and - deliberately - nothing that would
// help anybody open them.
//
// The salt and the cost parameters are here rather than derived from a constant for the reason
// core/port/crypto gives: the parameters this system derives with will be raised as machines get
// faster, and an archive written under the old ones has to keep opening. A salt and three numbers
// are not secret. The passphrase is not here and is not anywhere.
type Encryption struct {
	// Mode is AES256_GCM or NONE, matching backup.EncryptionMode on the target that wrote it.
	Mode string `json:"mode"`
	// KeyID names the key. It is what makes a rotation cheap and BK-2's second half true: a new
	// archive is written under the new key, an old one keeps naming the old one, and neither is
	// rewritten. Empty on an unencrypted archive.
	KeyID string `json:"key_id,omitempty"`
	// KDF names the derivation, when the key came from a passphrase. Empty when the key was
	// supplied directly, and then the four numbers below are empty too.
	KDF         string `json:"kdf,omitempty"`
	Salt        string `json:"salt,omitempty"`
	Passes      uint32 `json:"passes,omitempty"`
	MemoryKiB   uint32 `json:"memory_kib,omitempty"`
	Parallelism uint8  `json:"parallelism,omitempty"`
	KeyLength   uint32 `json:"key_length,omitempty"`
}

// The encryption modes as the manifest spells them. They are the values of
// backup.EncryptionMode; repeated as strings here because the manifest is a wire format and a
// wire format that changed when a domain constant was renamed would be a wire format nobody
// controls.
const (
	EncryptionAES256GCM = "AES256_GCM"
	EncryptionNone      = "NONE"
)

// IsEncrypted reports whether the members have to be opened before they can be read.
func (e Encryption) IsEncrypted() bool { return e.Mode == EncryptionAES256GCM }

// File is one member of the archive, as stored.
//
// SHA256 is over the bytes at the target, which on an encrypted archive is the ciphertext. That
// is the one checksum `POST /backups/{id}:verify` can check without the key, and checking an
// archive at the target without restoring it is exactly what that endpoint promises
// (backup-restore.md §3).
type File struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	// Records is how many JSON Lines the file holds before encryption. Zero for a member that is
	// not a data file.
	Records int64 `json:"records,omitempty"`
}

// Manifest is what an archive says about itself.
//
// It is the one member that is never encrypted, and that is a decision rather than an oversight:
// §8.1 requires a restore to list the archives at a target "without requiring any state in the
// database", showing the timestamp, the scope, the size, full or incremental, the chain to the
// parent and the encryption key ID. An operator who has lost the database and is holding the
// target credentials has not necessarily got the archive key yet, and a manifest they cannot read
// makes the listing impossible - which turns the worst day into a guessing game about which of
// forty directories is the one to try.
//
// What follows from that is a rule with teeth: no user content in the manifest. Not a container
// name, not an item title, not an account's address. Rule 10 applies to the exporter exactly as it
// applies to a log line, and here it applies for a second reason on top - this file is readable by
// whoever holds the storage.
type Manifest struct {
	FormatVersion int `json:"format_version"`
	// ArchiveID identifies this archive across targets and is what a later incremental names as
	// its parent. It is supplied by the caller from the ID port, never derived from the clock:
	// two runs in the same second at two targets must not collide.
	ArchiveID string `json:"archive_id"`
	// SchemaVersion is the migration the source database stood at. A restore compares it against
	// its own and runs the upward migrations in between - which is the whole reason the data is
	// JSON Lines rather than a pg_dump (§3).
	SchemaVersion string `json:"schema_version"`
	// ProductVersion is the build that wrote the archive. Diagnostic rather than load-bearing:
	// the restore path reads SchemaVersion, and this is what an operator quotes in a bug report.
	ProductVersion string `json:"product_version"`
	Mode           Mode   `json:"mode"`
	Scope          Scope  `json:"scope"`
	Period         Period `json:"period"`
	// SnapshotAt is the instant the export's REPEATABLE READ snapshot was taken, and therefore
	// the instant the archive represents (§5). It equals Period.To and is repeated under its own
	// name because that is what `backup_run.snapshot_at` records and what an operator reads.
	SnapshotAt time.Time `json:"snapshot_at"`
	// ParentID is the archive this one continues, empty on a full archive.
	ParentID string `json:"parent_id,omitempty"`
	// ParentPrefix is where that parent lies at the same target, so that a restore can walk the
	// chain by reading manifests rather than by listing and matching identifiers.
	ParentPrefix string     `json:"parent_prefix,omitempty"`
	Encryption   Encryption `json:"encryption"`
	// Counts is how many records each entity contributed, by entity name. It is what a dry run
	// reports before anything is written (§8.3) and what BK-3 compares.
	Counts map[string]int64 `json:"counts"`
	// MediaCount and MediaBytes describe the media without listing them. A content-addressed file
	// is named after the SHA-256 of its content, so a list of media checksums would be a list of
	// the file names - and on a holding with a hundred thousand attachments it would also be the
	// largest thing in the archive.
	MediaCount int64 `json:"media_count"`
	MediaBytes int64 `json:"media_bytes"`
	// Whole names the entities this archive carried complete rather than as a delta, even when it
	// is an incremental one - the join tables and the configuration rows whose schema cannot say
	// when they changed (Entity.Whole).
	//
	// It is in the manifest rather than only in the registry because it is a restore instruction:
	// for these entities the newest archive of a chain holds the whole truth, and the copies in
	// older archives are superseded rather than merged. A reader that had to consult its own
	// build's registry would be a reader deciding from its own version what an older archive
	// meant.
	Whole []string `json:"whole,omitempty"`
	// Files are the data members and their checksums. Media are deliberately absent, for the
	// reason above.
	Files []File `json:"files"`
}

// The refusals of the format, as codes rather than as prose.
const (
	// CodeFormatUnsupported is an archive written in a format version this build does not know.
	// It is refused before anything is read, which is the acceptance criterion: a partial import
	// of a file whose shape is guessed is worse than no import at all.
	CodeFormatUnsupported = "backup.archive_format_unsupported"
	// CodeManifestUnreadable is a manifest that is not JSON, or larger than any manifest has
	// business being.
	CodeManifestUnreadable = "backup.archive_manifest_unreadable"
	// CodeManifestInvalid is a manifest this build can parse and cannot use - a missing scope, an
	// unknown mode, an incremental with no parent.
	CodeManifestInvalid = "backup.archive_manifest_invalid"
)

// manifestMaxBytes bounds what ReadManifest will take from a target.
//
// A manifest holds a dozen data-file entries and a count per entity; a megabyte is three orders of
// magnitude more than that needs. The bound is here because the file arrives from somebody else's
// storage: without one, a target that answers a manifest request with an endless stream is a way
// to exhaust the process from outside (T-17).
const manifestMaxBytes = 1 << 20

// ReadManifest parses a manifest, and refuses a format version it does not know before it looks at
// anything else.
//
// The two-stage decode is the acceptance criterion in code. A single Unmarshal into Manifest would
// happily fill in the fields it recognised from a version 2 file and leave the rest at their zero
// values, and the caller would then restore an archive it had understood half of. So the version is
// read first, from a struct with one field, and a file this build cannot read is a typed error
// rather than a partial import.
func ReadManifest(r io.Reader) (Manifest, error) {
	raw, err := io.ReadAll(io.LimitReader(r, manifestMaxBytes+1))
	if err != nil {
		return Manifest{}, shared.ErrUnavailable.WithDetail(CodeManifestUnreadable).WithCause(err)
	}
	if len(raw) > manifestMaxBytes {
		return Manifest{}, shared.ErrValidation.WithDetail(CodeManifestUnreadable).
			WithCause(fmt.Errorf("manifest larger than %d bytes", manifestMaxBytes))
	}

	var version struct {
		FormatVersion *int `json:"format_version"`
	}
	if err := json.Unmarshal(raw, &version); err != nil {
		return Manifest{}, shared.ErrValidation.WithDetail(CodeManifestUnreadable).WithCause(err)
	}
	if version.FormatVersion == nil {
		return Manifest{}, shared.ErrValidation.WithDetail(CodeManifestUnreadable).
			WithCause(errors.New("no format_version"))
	}
	if *version.FormatVersion < MinimumReadableFormatVersion || *version.FormatVersion > FormatVersion {
		return Manifest{}, shared.ErrValidation.WithDetail(CodeFormatUnsupported).
			WithParams(map[string]string{
				"format_version": fmt.Sprint(*version.FormatVersion),
				"supported":      fmt.Sprint(FormatVersion),
			})
	}

	// Unknown fields are refused rather than ignored. Within one format version they cannot occur
	// except from a file that is not ours or one that has been edited, and both are worth stopping
	// for; across versions the check above has already refused the file.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, shared.ErrValidation.WithDetail(CodeManifestUnreadable).WithCause(err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Encode writes the manifest as the archive stores it: indented, so that an operator staring at a
// target with a text editor can read it, and with a trailing newline, so that the file is a
// well-behaved text file.
func (m Manifest) Encode(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(m)
}

// Validate reports what makes a manifest unusable rather than merely unexpected.
//
// It is deliberately about structure and not about content: whether the counts are true is a
// question for the checksums and for the reader, and a manifest that lied about them would still
// have to survive being parsed in order to be caught.
func (m Manifest) Validate() error {
	invalid := func(reason string) error {
		return shared.ErrValidation.WithDetail(CodeManifestInvalid).
			WithParams(map[string]string{"reason": reason}).WithCause(errors.New(reason))
	}
	switch {
	case m.ArchiveID == "":
		return invalid("archive_id")
	case !m.Mode.Valid():
		return invalid("mode")
	case !m.Scope.Kind.Valid() || m.Scope.ID == "":
		return invalid("scope")
	case m.SchemaVersion == "":
		return invalid("schema_version")
	case m.SnapshotAt.IsZero():
		return invalid("snapshot_at")
	case m.Encryption.Mode != EncryptionAES256GCM && m.Encryption.Mode != EncryptionNone:
		return invalid("encryption.mode")
	case m.Encryption.Mode == EncryptionAES256GCM && m.Encryption.KeyID == "":
		return invalid("encryption.key_id")
	// An incremental with no parent is the defect BK-3 exists to catch, and it is cheaper to
	// catch here: such an archive looks complete, restores silently, and leaves out everything
	// that happened before it.
	case m.Mode == ModeIncremental && m.ParentID == "":
		return invalid("parent_id")
	case m.Mode == ModeFull && m.ParentID != "":
		return invalid("parent_id_on_full")
	}
	return nil
}

// Name is the directory an archive occupies at a target, as §3 spells it:
//
//	hubtask-backup-<tenant>-<utc-timestamp>-<full|incremental>
//
// The timestamp is UTC and basic ISO 8601, because a target's key namespace is flat and sorting it
// as text has to sort it by time. A colon is not spelled out for the same reason a slash is not:
// several of the protocols behind the port cannot carry one in a name.
func Name(scopeID shared.ID, at time.Time, mode Mode) string {
	return fmt.Sprintf("hubtask-backup-%s-%s-%s",
		scopeID.String(), at.UTC().Format("20060102T150405Z"), mode.suffix())
}

// Prefix is the beginning every archive name of one scope shares, and nothing else at a target
// does.
//
// It is a filter over names rather than a key to ask a target for. The storage port's prefix is a
// place - the local adapter walks it as a directory - and an archive's name is a directory under
// the target's root rather than a directory containing them. So a caller lists the target and keeps
// what starts with this, which is the reading that works on every adapter and leaves everything
// else at the target unlisted.
func Prefix(scopeID shared.ID) string { return "hubtask-backup-" + scopeID.String() + "-" }

// DataName is the member one entity's records are written to.
func DataName(entity string) string { return DataPrefix + entity + ".jsonl" }

// MediaName is where a medium lives, addressed by the SHA-256 of its content.
//
// The two-character prefix directory is not decoration. Several of the targets behind the port are
// ordinary file systems, and a hundred thousand entries in one directory is where `ext4` starts
// answering a listing slowly and where a WebDAV `PROPFIND` starts timing out.
func MediaName(digest string) string {
	if len(digest) < 2 {
		return MediaPrefix + digest
	}
	return MediaPrefix + digest[:2] + "/" + digest
}
