// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backupstorage is the port a backup target sits behind (backup-restore.md §2, ADR-0019).
//
// It is deliberately not core/port/storage. That port is media-shaped in two ways an archive
// cannot satisfy: `Upload.Size` is a declared exact length, documented as "the size is declared
// rather than discovered", and an archive's length is not known before it has been written; and
// `TransferIssuer` is typed on `media.Object`, a domain type a backup has no business carrying.
// What does carry over is the pattern rather than the interface - the hand-signed SigV4, the
// resilience wrapper, the health registration under a feature name, and the outbound exception
// for a self-hosted endpoint on a private network.
//
// Everything here is a stream. An archive is as large as the data it holds, and reading one into
// memory to write it somewhere else is the architecture defect observability-reliability.md §6
// names rather than an operations problem (T-17).
package backupstorage

import (
	"context"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Store is one configured target: a flat namespace of keys, and five things done to them.
//
// The error contract is the shared one. A missing key is ErrNotFound; an unreachable target is
// ErrUnavailable with a `backup.` detail code and never a raw driver message, because a driver
// message from an FTP or SSH library carries the host, the user and sometimes the password
// (T-18). No call here runs inside a database transaction: a target is somebody else's machine,
// and a transaction waiting on one holds a connection for as long as they feel like taking
// (observability-reliability.md §8).
type Store interface {
	// Put writes one object and answers how many bytes it took.
	//
	// The length is discovered rather than declared, which is the whole reason this port exists
	// separately: an archive is compressed and encrypted as it is written, so nobody knows its
	// size until the last byte. An adapter whose protocol needs a length ahead of time - S3's
	// Content-Length - buys it with a spill to a temporary file rather than with a buffer in
	// memory.
	Put(ctx context.Context, key string, content io.Reader) (int64, error)

	// Get opens one object for reading. The caller owns the stream and closes it; an adapter
	// must not buffer the object to serve it.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// List answers what is under a prefix, and is the reason a restore works after a total loss:
	// `listBackupsAtTarget` reads the manifests at the target rather than rows in a database
	// that may no longer exist (backup-restore.md §2).
	//
	// Recursive, and the keys are answered whole rather than relative to the prefix, so that a
	// caller can hand one straight back to Get.
	List(ctx context.Context, prefix string) ([]Entry, error)

	// Stat answers one object's size and age without reading it.
	Stat(ctx context.Context, key string) (Entry, error)

	// Delete removes one object. Removing what is not there succeeds: deletion is the state the
	// caller asked for, and the generational retention that calls this retries.
	Delete(ctx context.Context, key string) error
}

// Entry is one object at the target.
type Entry struct {
	// Key is the whole key, not a name relative to the prefix it was found under.
	Key        string
	Size       int64
	ModifiedAt time.Time
}

// SpaceReporter is the optional half of a target: how much room is left.
//
// Optional because most of these protocols cannot answer it. A directory can, an SFTP server
// usually can, and a bucket cannot at all - and an interface that forced all of them to would
// have three implementations returning a number they invented. The connection probe reports
// `free_bytes: null` for a target that does not implement this, which is what the contract's
// nullable field is for.
type SpaceReporter interface {
	// FreeBytes is the room left at the target.
	FreeBytes(ctx context.Context) (int64, error)
}

// Spec is a target as an adapter needs it: what kind it is, where it points, and what it
// authenticates with.
//
// The credentials are separate from the configuration rather than a field inside it, and that
// separation is the one this whole task turns on: the configuration is read back by anybody who
// may list targets, and the credentials are sealed on the way in and never returned by any read
// (backup-restore.md §2, the `BackupTargetCreate` schema).
type Spec struct {
	Kind   backup.TargetKind
	Config backup.TargetConfig
	// Credentials are the secret half, already opened from storage. A map because each protocol
	// wants different names for them - `access_key` and `secret_key`, `password`, `private_key` -
	// and every value is wrapped, so a Spec printed whole says nothing (T-18).
	Credentials map[string]secret.Secret
}

// Credential answers one credential, empty when the target carries none of that name.
func (s Spec) Credential(name string) secret.Secret { return s.Credentials[name] }

// Opener turns a stored target into something that can be written to.
//
// A port rather than a switch in the application layer, because "which adapters does this build
// have" is a composition decision: an image without `rclone` in it has no rclone adapter, and the
// use case that refuses such a target must find that out by asking rather than by knowing.
type Opener interface {
	// Open answers the store for the target, or ErrValidation with `backup.kind_unsupported`
	// when this build has no adapter for it.
	Open(ctx context.Context, spec Spec) (Store, error)

	// Kinds lists what this build can talk to. It is what `/meta/capabilities` reports and what
	// a client offers in a dropdown - never the full enum of the contract, which names seven
	// adapters that do not exist yet.
	Kinds() []backup.TargetKind
}

// The refusals, as codes rather than as prose, so that four adapters cannot describe the same
// failure four ways.
const (
	// CodeKindUnsupported is a target whose kind this build has no adapter for.
	CodeKindUnsupported = "backup.kind_unsupported"
	// CodeTargetUnreachable is the target not answering at all - no route, no listener, a
	// timeout. Never the driver's message.
	CodeTargetUnreachable = "backup.target_unreachable"
	// CodeTargetRefused is the target answering and saying no: bad credentials, no permission on
	// the path. One code for both, because telling a caller which of the two it was is telling
	// them which half of their guess was right.
	CodeTargetRefused = "backup.target_refused"
	// CodeObjectNotFound is a key that is not there.
	CodeObjectNotFound = "backup.object_not_found"
	// CodeKeyInvalid is a key that tries to leave the target's namespace, or that the protocol
	// cannot express. Refused by every adapter rather than trusted from the caller, because
	// defence in depth is cheaper than certainty about every future caller (the reasoning
	// core/port/storage already applies to a media key).
	CodeKeyInvalid = "backup.key_invalid"
	// CodeTargetFailed is the target answering something this build cannot make sense of.
	CodeTargetFailed = "backup.target_failed"
)
