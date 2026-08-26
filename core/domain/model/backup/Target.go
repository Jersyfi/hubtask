// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backup is where a backup goes and what has to be true before it may go there
// (backup-restore.md §2, ADR-0019).
//
// A target is not an ordinary piece of configuration. It is a data egress channel by definition,
// which is why `backup-restore.md` §2 makes it a *named, narrow* exception to the SSRF rule
// rather than a hole, and why two of the things it can be - unencrypted storage, and a protocol
// that carries bytes in the clear - are refused here rather than only by the check constraints
// `0001_init` has carried since phase 0. A constraint produces a database error; a person
// configuring a backup deserves a field error with a message code saying what to acknowledge.
package backup

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TargetKind is the protocol a target speaks. The set is the contract's enum, whole: the column
// has carried all eleven since `0001_init`, and a kind this build has no adapter for is refused
// by the adapter registry rather than by pretending the name does not exist - "Hubtask cannot
// talk to SMB yet" and "SMB is not a thing" are different answers.
type TargetKind string

const (
	KindLocal     TargetKind = "LOCAL"
	KindS3        TargetKind = "S3"
	KindSFTP      TargetKind = "SFTP"
	KindFTPS      TargetKind = "FTPS"
	KindFTP       TargetKind = "FTP"
	KindWebDAV    TargetKind = "WEBDAV"
	KindSMB       TargetKind = "SMB"
	KindAzureBlob TargetKind = "AZURE_BLOB"
	KindGCS       TargetKind = "GCS"
	KindRclone    TargetKind = "RCLONE"
	KindHTTPPut   TargetKind = "HTTP_PUT"
)

var kinds = [...]TargetKind{
	KindLocal, KindS3, KindSFTP, KindFTPS, KindFTP,
	KindWebDAV, KindSMB, KindAzureBlob, KindGCS, KindRclone, KindHTTPPut,
}

func (k TargetKind) String() string { return string(k) }

func (k TargetKind) Valid() bool { return slices.Contains(kinds[:], k) }

// Kinds is the whole enum, for a caller that has to render it.
func Kinds() []TargetKind { return slices.Clone(kinds[:]) }

// CarriesBytesInTheClear reports a protocol with no transport security of its own.
//
// Only FTP, and that is not an oversight: FTPS is FTP over TLS, WebDAV is HTTP and is required
// below to be HTTPS, S3 the same, and SFTP is SSH. `HTTP_PUT` is judged by the scheme in its
// configuration rather than by its name, which is why it is not a member here - a target named
// HTTP_PUT pointed at an https:// endpoint is encrypted in transit and one pointed at http:// is
// not, and the kind alone cannot tell them apart.
func (k TargetKind) CarriesBytesInTheClear() bool { return k == KindFTP }

// EncryptionMode is whether the archive itself is encrypted before it leaves this process
// (backup-restore.md §4). Not to be confused with the transport: a target can be encrypted at
// rest and reached over a plaintext protocol, or the reverse, and both are worth a warning.
type EncryptionMode string

const (
	EncryptionAES256GCM EncryptionMode = "AES256_GCM"
	EncryptionNone      EncryptionMode = "NONE"
)

func (m EncryptionMode) String() string { return string(m) }

func (m EncryptionMode) Valid() bool {
	return m == EncryptionAES256GCM || m == EncryptionNone
}

// TargetConfig is where a target points: endpoint, bucket, path, host, region.
//
// Flat, and strings. The contract spells it `additionalProperties: true`, which a boundary
// narrows to scalars: a nested document in a target's configuration is a shape nothing reads and
// a place for something to hide, and every value any of these protocols needs is a name, a
// number or a flag. What is never in here is a credential - see Target.
type TargetConfig map[string]string

// Get answers one setting, trimmed, empty when it is not there.
func (c TargetConfig) Get(name string) string { return strings.TrimSpace(c[name]) }

// Clone is what leaves the aggregate. A map is a reference, and a caller that mutated the one
// inside a Target would be editing a stored object from the outside.
func (c TargetConfig) Clone() TargetConfig { return maps(c) }

func maps(from TargetConfig) TargetConfig {
	to := make(TargetConfig, len(from))
	for name, value := range from {
		to[name] = value
	}
	return to
}

// requiredConfig is what a kind cannot be created without. Deliberately the minimum that makes a
// target addressable at all, and no more: the adapter validates the rest when it opens the
// target, and a domain that knew every adapter's options would have to be edited for every new
// one.
var requiredConfig = map[TargetKind][]string{
	KindLocal:   {"path"},
	KindS3:      {"bucket"},
	KindSFTP:    {"host", "path"},
	KindWebDAV:  {"url"},
	KindFTPS:    {"host"},
	KindFTP:     {"host"},
	KindSMB:     {"host", "share"},
	KindHTTPPut: {"url"},
	// AZURE_BLOB, GCS and RCLONE have no adapter yet and therefore nothing to require. A kind
	// with no entry requires nothing here and is refused by the registry a moment later, which
	// is the more useful refusal.
}

// The warnings a target carries about itself. They are the resource's vocabulary and are
// deliberately not the installation's: `/meta/health` speaks `config.backup_*` about the
// installation as a whole ("no target configured", "only one target"), and a target speaks
// `backup.target_*` about itself. The two were named inconsistently across the specification and
// backup-restore.md §10; E-03 settles it this way round because a warning is named after what it
// describes, and `config.` names a thing an operator sets in the environment.
const (
	WarningUnencrypted       = "backup.target_unencrypted"
	WarningPlaintextProtocol = "backup.target_plaintext_protocol"
)

// Target is one configured place backups go.
type Target struct {
	ID shared.ID
	// TenantID is whose target this is, and the zero identifier for an instance-wide one. A
	// tenant-owned target exists only where the operator has switched them on
	// (HUBTASK_BACKUP_TENANT_TARGETS); the switch is read in the application layer, because
	// "may this exist here" is an authorisation question (ADR-0005).
	TenantID shared.ID
	Name     string
	Kind     TargetKind
	Config   TargetConfig
	// CredentialKeyID is the master key the credentials were sealed under, empty for a target
	// that needs none. The credentials themselves are never part of this aggregate: they are
	// sealed on the way in and read only by the adapter that connects (backup-restore.md §2).
	CredentialKeyID string
	EncryptionMode  EncryptionMode
	// EncryptionKeyID names the backup key an archive at this target is encrypted under, so that
	// a rotation leaves older archives readable (backup-restore.md §4).
	EncryptionKeyID string
	// RegionNote is what the operator wrote down about where the bytes physically are. Data
	// residency is a legal question with no technical answer, so this is prose an auditor reads
	// rather than something the system enforces (GDPR chapter V).
	RegionNote string
	// InsecureAckBy and InsecureAckAt are who accepted an unencrypted or plaintext target, and
	// when. Kept rather than reduced to a flag: "somebody ticked a box" is not an answer to
	// "who decided this", and it is the entry an auditor looks for.
	InsecureAckBy shared.ID
	InsecureAckAt time.Time
	Enabled       bool
	LastTestAt    time.Time
	// LastTestOK is nil for a target nobody has tested. Not false: never tested and tested and
	// failed are different things, and the second is the one that should worry somebody.
	LastTestOK *bool
	// LastTestError is the message code of the last failed probe, never a driver message - a
	// driver message from an FTP or SSH library carries the host, the user and sometimes the
	// password (rule 10, T-18).
	LastTestError string
	CreatedAt     time.Time
	CreatedBy     shared.ID
	Version       int
}

// NewTargetInput is what creating one needs.
type NewTargetInput struct {
	ID                   shared.ID
	TenantID             shared.ID
	Name                 string
	Kind                 TargetKind
	Config               TargetConfig
	EncryptionMode       EncryptionMode
	RegionNote           string
	InsecureAcknowledged bool
	CreatedBy            shared.ID
	Now                  time.Time
}

// maxNameLength is what a name may be. A target's name is what an operator picks it out by in a
// list, not a document.
const maxNameLength = 200

// NewTarget builds a target, or says what is wrong with it in the words a client can act on.
func NewTarget(in NewTargetInput) (Target, error) {
	var fields []shared.FieldError

	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		fields = append(fields, field("/name", "backup.name_required"))
	case len([]rune(name)) > maxNameLength:
		fields = append(fields, field("/name", "backup.name_too_long"))
	}

	if !in.Kind.Valid() {
		fields = append(fields, field("/kind", "backup.kind_invalid"))
	}

	mode := in.EncryptionMode
	if mode == "" {
		// The contract's default, stated here as well: a target created without an opinion is
		// encrypted, and unencrypted is something somebody has to ask for.
		mode = EncryptionAES256GCM
	}
	if !mode.Valid() {
		fields = append(fields, field("/encryption_mode", "backup.encryption_mode_invalid"))
	}

	config := in.Config.Clone()
	if config == nil {
		config = TargetConfig{}
	}
	for _, setting := range requiredConfig[in.Kind] {
		if config.Get(setting) == "" {
			fields = append(fields, shared.FieldError{
				Path:   "/config/" + setting,
				Code:   "backup.config_required",
				Params: map[string]string{"setting": setting},
			})
		}
	}

	target := Target{
		ID: in.ID, TenantID: in.TenantID, Name: name, Kind: in.Kind, Config: config,
		EncryptionMode: mode, RegionNote: strings.TrimSpace(in.RegionNote),
		Enabled: true, CreatedAt: in.Now, CreatedBy: in.CreatedBy, Version: 1,
	}

	// Last, so that a caller fixing an insecure target is not also told about a missing bucket
	// one field at a time. The acknowledgement is the check constraint's, made a field error:
	// the database refuses this too, and a database error is not something a client can act on.
	if target.NeedsAcknowledgement() && !in.InsecureAcknowledged {
		fields = append(fields, shared.FieldError{
			Path:   "/insecure_acknowledged",
			Code:   "backup.insecure_acknowledgement_required",
			Params: map[string]string{"reasons": strings.Join(target.Warnings(), " ")},
		})
	}

	if len(fields) > 0 {
		return Target{}, shared.ErrValidation.
			WithDetail("backup.target_invalid").
			WithFields(fields...)
	}

	if target.NeedsAcknowledgement() {
		target.InsecureAckBy, target.InsecureAckAt = in.CreatedBy, in.Now
	}
	return target, nil
}

// NeedsAcknowledgement reports whether somebody has to say out loud that they accept this.
//
// Two independent reasons, and either is enough: the archive is not encrypted before it leaves,
// or the protocol carries it in the clear. They are independent because they fail differently -
// the first exposes the data to whoever holds the storage, the second to whoever is on the wire.
func (t Target) NeedsAcknowledgement() bool {
	return t.EncryptionMode == EncryptionNone || t.Kind.CarriesBytesInTheClear()
}

// Warnings is what this target says about itself, whether or not it was acknowledged. An
// acknowledgement is a decision, not a silencer: the operator who accepted an unencrypted target
// last year is not the person reading the list today.
func (t Target) Warnings() []string {
	var warnings []string
	if t.EncryptionMode == EncryptionNone {
		warnings = append(warnings, WarningUnencrypted)
	}
	if t.Kind.CarriesBytesInTheClear() {
		warnings = append(warnings, WarningPlaintextProtocol)
	}
	return warnings
}

// IsInstanceWide reports a target that belongs to no tenant.
func (t Target) IsInstanceWide() bool { return t.TenantID.IsZero() }

// Acknowledged reports whether the insecure configuration was accepted by somebody.
func (t Target) Acknowledged() bool { return !t.InsecureAckAt.IsZero() }

func field(path, code string) shared.FieldError {
	return shared.FieldError{Path: path, Code: code}
}
