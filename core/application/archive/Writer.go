// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Source is where an archive's records come from.
//
// It is a callback rather than a slice, and that is the shape the whole export rests on: the
// database hands over one row at a time inside its snapshot, and a method returning []Record would
// be a method that reads a tenant into memory before writing a byte of it.
type Source interface {
	// Records hands every record of one entity to yield, oldest first.
	//
	// since is exclusive and is the zero time on a full archive. An incremental run passes its
	// parent's snapshot instant, and the source answers what changed after it - including the
	// tombstones, which are what stop a chain resurrecting deleted objects.
	Records(ctx context.Context, entity Entity, since time.Time, yield func(Record) error) error
}

// Media opens a medium by its content address. Separate from Source because the two answer to
// different stores: the records come from the database, and the bytes of an attachment come from
// the object store (C-05).
type Media interface {
	// Open answers the medium's content, or ErrNotFound. The caller closes the stream.
	Open(ctx context.Context, digest string) (io.ReadCloser, error)
}

// Request is one run: what to write, where, and under which key.
type Request struct {
	// ArchiveID identifies the archive. It comes from the ID port rather than from the clock, so
	// that two runs in the same second at two targets cannot collide.
	ArchiveID shared.ID
	// Prefix is the directory at the target, from Name. Everything this run writes lands beneath
	// it and nothing outside it.
	Prefix string
	Scope  Scope
	Mode   Mode
	// SnapshotAt is when the export's snapshot was taken. It is the archive's instant, and on an
	// incremental it is the next run's Since.
	SnapshotAt time.Time
	// Since is where an incremental starts, exclusive. Zero on a full archive.
	Since time.Time
	// ParentID and ParentPrefix name the archive an incremental continues.
	ParentID     string
	ParentPrefix string
	// Ancestors are the prefixes of the whole chain back to the full archive, newest first. They
	// are what makes "do not re-transfer a file that did not change" true: a medium already
	// stored anywhere in the chain is referenced rather than written again, and a restore
	// resolves a digest by searching the chain from newest to oldest.
	Ancestors []string

	SchemaVersion  string
	ProductVersion string

	// Encryption describes what protects the members, and is copied into the manifest verbatim.
	Encryption Encryption
	// Key is the archive key. Empty exactly when Encryption.Mode is NONE - a mismatch either way
	// is refused, because an archive that says it is encrypted and is not is worse than one that
	// says it is not.
	Key secret.Bytes

	// IncludeMedia and IncludeAudit are the two schedule settings that change what an archive
	// holds (backup-restore.md §5, §7).
	IncludeMedia bool
	IncludeAudit bool
}

// The refusals of a run.
const (
	// CodeArchiveRequestInvalid is a run that would produce an archive nothing could restore.
	CodeArchiveRequestInvalid = "backup.archive_request_invalid"
	// CodeMediaMissing is a record referencing a medium the object store no longer has. It is a
	// refusal rather than a warning: an archive silently missing an attachment is one whose
	// restore is a surprise.
	CodeMediaMissing = "backup.archive_media_missing"
)

// Writer assembles an archive at a target.
//
// **The archive is streamed to the target as it is produced, not staged and then transferred**,
// and that is this task's own decision. Both were open. Staging makes a checksum over the whole
// archive trivial and a resumption after process death cheap, because the finished part is on
// local disk; streaming keeps memory and disk flat whatever the holding weighs.
//
// Streaming wins on the case that decides it. The installation this system is built for runs in a
// container whose writable layer is small and whose data volume is the database - staging a
// hundred gigabytes there fails at the disk rather than at the target, which is the worse of the
// two failures because it happens on the machine that is still working. What staging bought is
// bought differently instead: the archive is a directory of members rather than one stream, so a
// checksum per member replaces a checksum over the whole, and a resumption finds the members
// already at the target with one List rather than on a scratch disk that a restarted container no
// longer has.
//
// What that costs is honest to state: a member is written once and cannot be rewound, so a
// producer that fails half way leaves a member at the target that no manifest names. Nothing
// reads such a member - the manifest is written after them and checksums.txt after that - and
// generational retention removes the directory when it removes the run (E-05).
type Writer struct {
	store  backupstorage.Store
	cipher crypto.StreamCipher
	media  Media
}

// NewWriter builds a writer for one target.
func NewWriter(store backupstorage.Store, cipher crypto.StreamCipher, media Media) *Writer {
	return &Writer{store: store, cipher: cipher, media: media}
}

// Write produces the archive and answers the manifest it wrote.
//
// The order is the format: the data files, then the media they reference, then the manifest that
// names them, then checksums.txt. Nothing later depends on nothing earlier, and the last file is
// the one that says the run finished.
func (w *Writer) Write(ctx context.Context, request Request, source Source) (Manifest, error) {
	if err := request.validate(); err != nil {
		return Manifest{}, err
	}

	manifest := Manifest{
		FormatVersion:  FormatVersion,
		ArchiveID:      request.ArchiveID.String(),
		SchemaVersion:  request.SchemaVersion,
		ProductVersion: request.ProductVersion,
		Mode:           request.Mode,
		Scope:          request.Scope,
		Period:         Period{From: request.Since, To: request.SnapshotAt},
		SnapshotAt:     request.SnapshotAt,
		ParentID:       request.ParentID,
		ParentPrefix:   request.ParentPrefix,
		Encryption:     request.Encryption,
		Counts:         map[string]int64{},
	}

	// The digests every record referenced, in the order they were first seen. A set rather than a
	// list per record, because the same attachment on ten items is one file - that is what
	// content addressing is for. It is the one thing a run holds in memory that grows with the
	// holding: a digest is 64 characters, so a hundred thousand attachments cost a few
	// mebibytes, which is the price of not asking the object store the same question ten times.
	referenced := map[string]struct{}{}
	var order []string

	for _, entity := range Entities() {
		if entity.Optional && !request.IncludeAudit {
			continue
		}
		file, err := w.writeData(ctx, request, source, entity, func(blob Blob) {
			if _, seen := referenced[blob.Digest]; !seen {
				referenced[blob.Digest] = struct{}{}
				order = append(order, blob.Digest)
			}
		})
		if err != nil {
			return Manifest{}, err
		}
		manifest.Files = append(manifest.Files, file)
		manifest.Counts[entity.Name] = file.Records
	}

	if request.IncludeMedia {
		count, bytes, err := w.writeMedia(ctx, request, order)
		if err != nil {
			return Manifest{}, err
		}
		manifest.MediaCount, manifest.MediaBytes = count, bytes
	}

	return w.seal(ctx, request, manifest)
}

// writeData streams one entity's records to the target and answers the member it wrote.
func (w *Writer) writeData(
	ctx context.Context, request Request, source Source, entity Entity, saw func(Blob),
) (File, error) {
	path := DataName(entity.Name)
	var records int64

	file, err := w.putMember(ctx, request, path, func(to io.Writer) error {
		written, err := WriteRecords(to, func(yield func(Record) error) error {
			return source.Records(ctx, entity, request.Since, func(record Record) error {
				for _, blob := range record.Blobs {
					saw(blob)
				}
				return yield(record)
			})
		})
		records = written
		return err
	})
	if err != nil {
		return File{}, err
	}
	file.Records = records
	return file, nil
}

// writeMedia transfers the media the records referenced, skipping whatever the chain already
// holds.
//
// They go after the data files rather than beside them, which is §5's rule and not an
// optimisation: the snapshot fixed which checksums the records name, and fetching the bytes
// afterwards means fetching exactly those. A medium replaced during the run is not the one the
// archive refers to, and asking for it by digest is what makes that impossible rather than
// unlikely.
func (w *Writer) writeMedia(ctx context.Context, request Request, order []string) (int64, int64, error) {
	if w.media == nil {
		return 0, 0, shared.Internalf("archive: media requested with no media source")
	}

	alreadyStored, err := w.chainMedia(ctx, request)
	if err != nil {
		return 0, 0, err
	}

	var count, bytes int64
	for _, digest := range order {
		if alreadyStored[digest] {
			continue
		}
		content, err := w.media.Open(ctx, digest)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return 0, 0, shared.ErrValidation.WithDetail(CodeMediaMissing).
					WithParams(map[string]string{"sha256": digest}).WithCause(err)
			}
			return 0, 0, err
		}

		file, putErr := w.putMember(ctx, request, MediaName(digest), func(to io.Writer) error {
			_, copyErr := io.Copy(to, content)
			return copyErr
		})
		_ = content.Close()
		if putErr != nil {
			return 0, 0, putErr
		}
		count++
		bytes += file.Bytes
	}
	return count, bytes, nil
}

// chainMedia answers what the ancestors already hold, with one listing per ancestor rather than
// one question per medium. A target is somebody else's machine, and the difference between a
// listing and a hundred thousand Stat calls is the difference between a backup and an outage.
func (w *Writer) chainMedia(ctx context.Context, request Request) (map[string]bool, error) {
	stored := map[string]bool{}
	for _, ancestor := range request.Ancestors {
		entries, err := w.store.List(ctx, ancestor+"/"+MediaPrefix)
		if err != nil {
			// An ancestor that cannot be listed is not a reason to fail the run: the worst it
			// costs is transferring a file the chain already has, and refusing to back up
			// because an old archive is unreachable is the wrong trade on the day it matters.
			if errors.Is(err, shared.ErrNotFound) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if digest := entry.Key[strings.LastIndex(entry.Key, "/")+1:]; looksLikeDigest(digest) {
				stored[digest] = true
			}
		}
	}
	return stored, nil
}

// seal writes the manifest and then checksums.txt, in that order, which is what makes the second
// of them the moment the archive became an archive.
func (w *Writer) seal(ctx context.Context, request Request, manifest Manifest) (Manifest, error) {
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}

	checksums := NewChecksums()
	for _, file := range manifest.Files {
		if err := checksums.Add(file.Path, file.SHA256); err != nil {
			return Manifest{}, err
		}
	}

	// The manifest goes to the target in the clear - see the type's own comment for why - so it
	// is written directly rather than through putMember, which would encrypt it.
	counter := NewCounter()
	reader, produced := pipe(ctx, "backup.archive.manifest", func(to io.Writer) error {
		return manifest.Encode(io.MultiWriter(counter, to))
	})
	if _, err := w.store.Put(ctx, request.Prefix+"/"+ManifestName, reader); err != nil {
		reader.CloseWithError(err)
		<-produced
		return Manifest{}, err
	}
	if err := <-produced; err != nil {
		return Manifest{}, err
	}
	if err := checksums.Add(ManifestName, counter.Digest()); err != nil {
		return Manifest{}, err
	}

	checksumReader, checksumProduced := pipe(ctx, "backup.archive.checksums", checksums.Encode)
	if _, err := w.store.Put(ctx, request.Prefix+"/"+ChecksumsName, checksumReader); err != nil {
		checksumReader.CloseWithError(err)
		<-checksumProduced
		return Manifest{}, err
	}
	if err := <-checksumProduced; err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// putMember writes one member: produced by produce, encrypted if the run is, counted on the way
// past, and streamed to the target without ever being held.
func (w *Writer) putMember(
	ctx context.Context, request Request, path string, produce func(io.Writer) error,
) (File, error) {
	counter := NewCounter()

	reader, produced := pipe(ctx, "backup.archive.member", func(to io.Writer) error {
		// The counter sits between the cipher and the target, so what it measures is what the
		// target stores - which is the checksum `:verify` can check without the archive key.
		stored := io.MultiWriter(counter, to)
		if !request.Encryption.IsEncrypted() {
			return produce(stored)
		}
		sealed, err := w.cipher.Seal(stored, request.Key, request.purposeOf(path))
		if err != nil {
			return err
		}
		if err := produce(sealed); err != nil {
			// Closed anyway, so that the cipher's own buffers go with the failure rather than
			// with the process.
			_ = sealed.Close()
			return err
		}
		return sealed.Close()
	})

	written, err := w.store.Put(ctx, request.Prefix+"/"+path, reader)
	if err != nil {
		// Unblock the producer, which is otherwise waiting to write into a pipe nobody reads.
		reader.CloseWithError(err)
		<-produced
		return File{}, err
	}
	if err := <-produced; err != nil {
		return File{}, err
	}
	if written != counter.Bytes() {
		return File{}, shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetFailed).
			WithCause(errors.New("the target accepted a different number of bytes than were sent"))
	}
	return File{Path: path, Bytes: counter.Bytes(), SHA256: counter.Digest()}, nil
}

// pipe turns a producer that writes into a reader that a target pulls from.
//
// The goroutine is the bridge between the two halves of the port: records are pushed by the
// database and members are pulled by the store, and something has to stand between them. It goes
// through concurrency.Go, so a panic in a producer is reported rather than taking the process with
// it (ADR-0016), and the channel is what makes the producer's error the caller's error rather than
// a truncated file.
func pipe(ctx context.Context, component string, produce func(io.Writer) error) (*io.PipeReader, <-chan error) {
	reader, writer := io.Pipe()
	produced := make(chan error, 1)

	concurrency.Go(ctx, component, func(context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := shared.Internalf("archive: producing a member panicked: %v", recovered)
				_ = writer.CloseWithError(err)
				produced <- err
				// Re-raised, so that the panic observer sees it as the defect it is.
				panic(recovered)
			}
		}()
		err := produce(writer)
		_ = writer.CloseWithError(err)
		produced <- err
	})
	return reader, produced
}

func (r Request) purposeOf(path string) crypto.Purpose {
	return MemberPurpose(r.ArchiveID.String(), path)
}

func (r Request) validate() error {
	invalid := func(reason string) error {
		return shared.ErrValidation.WithDetail(CodeArchiveRequestInvalid).
			WithParams(map[string]string{"reason": reason}).WithCause(errors.New(reason))
	}
	switch {
	case r.ArchiveID.IsZero():
		return invalid("archive_id")
	case r.Prefix == "" || strings.Contains(r.Prefix, ".."):
		return invalid("prefix")
	case !r.Mode.Valid():
		return invalid("mode")
	case r.SnapshotAt.IsZero():
		return invalid("snapshot_at")
	case r.Mode == ModeIncremental && r.Since.IsZero():
		return invalid("since")
	case r.Mode == ModeIncremental && !r.Since.Before(r.SnapshotAt):
		return invalid("since_after_snapshot")
	case !slices.Contains([]string{EncryptionAES256GCM, EncryptionNone}, r.Encryption.Mode):
		return invalid("encryption_mode")
	// An archive that says it is encrypted and is not is worse than one that says it is not: the
	// first is trusted.
	case r.Encryption.IsEncrypted() && r.Key.Len() == 0:
		return invalid("key_missing")
	case !r.Encryption.IsEncrypted() && r.Key.Len() > 0:
		return invalid("key_without_encryption")
	}
	return nil
}
