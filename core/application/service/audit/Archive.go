// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The audit export's archive (E-09, audit.md §5): what `POST /audit:export` writes at a backup
// target, and the one place its shape is decided.
//
// It is not the backup archive of `core/application/archive`, and the difference is what it is
// for. A backup is read back by this system, into a tenant, and is therefore encrypted, versioned
// and restorable. An export is read by somebody outside it - an auditor with a spreadsheet, a
// second system, a regulator - so it is written in the clear, in a format they already have a tool
// for, and it is never restored. What the two share is the closing discipline: a checksum per
// member, and the checksums written last, so that an unfinished export is recognisable as one.

const (
	// ExportFormatVersion is the version of *this* format - not the backup archive's, not the
	// schema's, not the product's. It changes when a reader that did not know about the change
	// would misread the file.
	ExportFormatVersion = 1

	// The members. Fixed strings rather than a computed layout, for the reason the backup
	// archive's are fixed: a reader in a later version looks for these exact names.
	exportManifestName  = "manifest.json"
	exportSignatureName = "signature.json"
	exportChecksumsName = "checksums.txt"
	exportDataStem      = "entries"

	// exportComponent names the producing goroutine for the panic observer and its metric. An
	// underscore rather than the dot every other label uses: `audit.` is the prefix of a message
	// code in this project, and a component label that looked like one would be read as a missing
	// catalogue entry by the gate that checks them.
	exportComponent = "audit_export"

	// signaturePurpose binds a signature to the export it closes, so that one lifted from another
	// archive fails to open rather than appearing to belong here.
	signaturePurpose = "audit_export.signature:"
)

// ArchiveRequest is one export as the job describes it.
type ArchiveRequest struct {
	ExportID shared.ID
	TenantID shared.ID
	TargetID shared.ID
	Period   repository.Period
	Format   Format
}

// TargetStore opens a configured backup target.
//
// An interface here rather than the three dependencies behind it, because an export has no
// business with a target's credentials: it needs somewhere to write. What is on the other side
// reads the target, unseals its credential and opens the adapter (core/application/service/backup).
type TargetStore interface {
	OpenTarget(ctx context.Context, tenantID, targetID shared.ID) (backupstorage.Store, error)
}

// Archivist writes one export, end to end.
//
// The application layer's half of the `audit.export` job: the worker owns the queue, the retries
// and the lease, and everything about what an export *is* lives here.
type Archivist struct {
	Trail   repository.Trail
	Targets TargetStore
	// Pseudonyms applies the same substitution the read applies (audit.md §6): an archive that
	// carried an erased actor's name would be the one copy of the trail where the erasure had not
	// happened, and it is the copy that leaves the installation.
	Pseudonyms Pseudonyms
	Encryptor  crypto.Encryptor
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// ProductVersion goes into the manifest: what an archive has to record is the build that
	// wrote it, and a build knows that about itself.
	ProductVersion string
}

// ExportManifest is what the archive says about itself.
type ExportManifest struct {
	FormatVersion int       `json:"format_version"`
	ExportID      string    `json:"export_id"`
	TenantID      string    `json:"tenant_id"`
	GeneratedAt   time.Time `json:"generated_at"`
	Product       string    `json:"product_version"`

	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Format string    `json:"format"`
	Member string    `json:"member"`

	// Entries is how many are in the file, and the chain fields say which stretch of the trail
	// they are. An auditor comparing this with `POST /audit:verify` over the same period is
	// comparing the archive with the live trail, which is the one check the archive cannot do for
	// itself.
	Entries   int    `json:"entries"`
	FirstSeq  int64  `json:"first_seq"`
	LastSeq   int64  `json:"last_seq"`
	FirstHash string `json:"first_hash"`
	LastHash  string `json:"last_hash"`

	// Encryption is `none`, and it is stated rather than left out. An export is written to be
	// read outside this installation; encrypting it under a key only this installation holds
	// would make it unreadable exactly where it is meant to be read (audit.md §5, which asks for
	// a signature rather than a cipher).
	Encryption string `json:"encryption"`
	// Signed says whether signature.json is there to be found.
	Signed bool `json:"signed"`
}

// ExportSignature is signature.json.
//
// Symmetric rather than a public key signature, and the difference matters to whoever reads it: it
// proves the archive was produced by this installation and has not been altered since, to anybody
// who can ask this installation. It does not prove it to a third party who cannot. Asymmetric
// signing waits for a key management decision that is not this task's (open point S-2).
type ExportSignature struct {
	Algorithm string `json:"algorithm"`
	// KeyID names the master key the value was sealed under. Not a secret: it says which key,
	// never anything about it.
	KeyID string `json:"key_id"`
	// Over is the member the signature closes over.
	Over string `json:"over"`
	// Digest is the SHA-256 of that member, in the clear, so that a reader can check the archive
	// without this installation - and Value is that digest sealed, which is what only this
	// installation could have produced.
	Digest string `json:"digest"`
	Value  string `json:"value"`
}

// Write produces the archive and answers its manifest.
//
// The order is the whole of the format's closing discipline: the entries, the manifest that names
// their digest, the signature over the manifest, and the checksums last. An export without
// `checksums.txt` is an unfinished export, whatever else is lying next to it.
func (a Archivist) Write(ctx context.Context, in ArchiveRequest) (ExportManifest, error) {
	if !in.Format.Valid() {
		return ExportManifest{}, shared.ErrInternal.WithDetail("audit.format_invalid")
	}

	store, err := a.Targets.OpenTarget(ctx, in.TenantID, in.TargetID)
	if err != nil {
		return ExportManifest{}, err
	}

	prefix := ArchiveName(in.TenantID, in.Period)
	dataName := exportDataStem + "." + in.Format.extension()
	checksums := archive.NewChecksums()

	written, err := a.writeEntries(ctx, store, prefix+"/"+dataName, in)
	if err != nil {
		return ExportManifest{}, err
	}
	if err := checksums.Add(dataName, written.digest); err != nil {
		return ExportManifest{}, err
	}

	manifest := ExportManifest{
		FormatVersion: ExportFormatVersion,
		ExportID:      in.ExportID.String(),
		TenantID:      in.TenantID.String(),
		GeneratedAt:   a.Clock.Now().UTC(),
		Product:       a.ProductVersion,
		From:          in.Period.From.UTC(),
		To:            in.Period.To.UTC(),
		Format:        string(in.Format),
		Member:        dataName,
		Entries:       written.entries,
		FirstSeq:      written.firstSeq,
		LastSeq:       written.lastSeq,
		FirstHash:     written.firstHash,
		LastHash:      written.lastHash,
		Encryption:    "none",
		Signed:        a.Encryptor != nil && a.Encryptor.ActiveKeyID() != "",
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return ExportManifest{}, shared.Internalf("audit: writing the export manifest: %w", err)
	}
	if err := a.put(ctx, store, prefix+"/"+exportManifestName, manifestBytes, checksums, exportManifestName); err != nil {
		return ExportManifest{}, err
	}

	if manifest.Signed {
		signature, err := a.sign(ctx, in.ExportID, manifestBytes)
		if err != nil {
			return ExportManifest{}, err
		}
		signatureBytes, err := json.MarshalIndent(signature, "", "  ")
		if err != nil {
			return ExportManifest{}, shared.Internalf("audit: writing the export signature: %w", err)
		}
		if err := a.put(ctx, store, prefix+"/"+exportSignatureName, signatureBytes, checksums, exportSignatureName); err != nil {
			return ExportManifest{}, err
		}
	}

	var closing bytesBuffer
	if err := checksums.Encode(&closing); err != nil {
		return ExportManifest{}, err
	}
	if _, err := store.Put(ctx, prefix+"/"+exportChecksumsName, closing.reader()); err != nil {
		return ExportManifest{}, err
	}
	return manifest, nil
}

// ArchiveName is the directory one export lives in at a target.
//
// The period is in the name rather than only in the manifest, so that an operator listing a target
// can see what each archive covers without opening any of them - which is the question somebody
// standing at a target is usually asking.
func ArchiveName(tenantID shared.ID, period repository.Period) string {
	return fmt.Sprintf("hubtask-audit-%s-%s-%s",
		tenantID.String(),
		period.From.UTC().Format("20060102T150405Z"),
		period.To.UTC().Format("20060102T150405Z"))
}

// member is what writing the entries produced.
type member struct {
	digest    string
	entries   int
	firstSeq  int64
	lastSeq   int64
	firstHash string
	lastHash  string
}

// writeEntries streams the trail into the target and digests it on the way past.
//
// A pipe rather than a file or a buffer: an export over four hundred days is as large as the tenant
// was busy, and holding it anywhere before writing it is the defect T-17 names. The producing side
// runs through SafeGo, which is the only place in this project a goroutine may start (rule 5).
func (a Archivist) writeEntries(
	ctx context.Context, store backupstorage.Store, key string, in ArchiveRequest,
) (member, error) {
	reader, writer := io.Pipe()
	// Buffered, so that the producer's last act never blocks on a reader that has gone.
	produced := make(chan produce, 1)

	concurrency.Go(ctx, exportComponent, func(ctx context.Context) {
		// The initial value is a failure, and it is what a panic leaves behind: the deferred close
		// then fails the Put rather than ending the stream cleanly, so that an export cut short
		// can never look complete at the target.
		out := produce{err: shared.Internalf("audit: the export writer stopped unexpectedly")}
		defer func() {
			writer.CloseWithError(out.err)
			produced <- out
		}()
		out.member, out.err = a.stream(ctx, writer, in)
	})

	if _, err := store.Put(ctx, key, reader); err != nil {
		// The producer may be blocked writing into a pipe nobody is reading any more. Closing the
		// reading end unblocks it with an error rather than leaving the goroutine there, and the
		// receive is what makes sure it is finished before this returns.
		_ = reader.CloseWithError(err)
		<-produced
		return member{}, err
	}

	out := <-produced
	return out.member, out.err
}

// produce is what the writing side finished with: the member it wrote, or why it did not.
type produce struct {
	member member
	err    error
}

// stream writes every entry of the period, in the format asked for, and keeps what the manifest
// needs.
func (a Archivist) stream(
	ctx context.Context, writer io.Writer, in ArchiveRequest,
) (member, error) {
	// Everything written to the file is written to the digest as well, once: the checksum of a
	// member has to be the checksum of what actually left, not of what was meant to.
	digest := sha256.New()

	rows, err := a.renderer(in.Format, io.MultiWriter(writer, digest))
	if err != nil {
		return member{}, err
	}

	var out member
	err = a.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: in.TenantID},
		func(ctx context.Context) error {
			// The transaction is held while bytes go to somebody else's machine, which
			// observability-reliability.md §8 keeps for the backup run. It is affordable for the
			// same reasons and one more of its own: this runs on the worker role rather than the
			// API path, it is read-only, and the period is closed - the trail is append-only, so
			// there is nothing here that could change under the walk and no snapshot to hold.
			return a.Trail.Walk(ctx, in.Period, func(record repository.Record) error {
				if out.entries == 0 {
					out.firstSeq, out.firstHash = record.Seq, hex.EncodeToString(record.Hash)
				}
				out.lastSeq, out.lastHash = record.Seq, hex.EncodeToString(record.Hash)
				out.entries++

				// One entry at a time rather than a page, because a walk has no page: the lookup
				// is a map read behind the port for every actor it has already seen.
				substituted, err := Pseudonymised(ctx, a.Pseudonyms, []repository.Record{record})
				if err != nil {
					return err
				}
				return rows(substituted[0])
			})
		})
	if err != nil {
		return member{}, err
	}
	if err := rows(repository.Record{}); err != nil {
		// The flush. A renderer that buffers - the CSV writer does - has to be told the last row
		// has gone past, and a zero record is that signal rather than a second callback.
		return member{}, err
	}

	out.digest = hex.EncodeToString(digest.Sum(nil))
	return out, nil
}

// renderer answers the function that writes one record, and flushes when it is handed the zero
// record.
func (a Archivist) renderer(format Format, to io.Writer) (func(repository.Record) error, error) {
	if format == FormatCSV {
		return csvRenderer(to)
	}
	return jsonLinesRenderer(to), nil
}

// jsonLinesRenderer writes the entry as the API answers it, one object per line.
//
// The same projection every channel reads, so that an export and a page of `GET /audit` cannot
// describe the same entry differently. What it does not carry is `prev_hash`: the linkage is
// checked by `:verify` against the live trail, and the manifest names the stretch of chain the
// archive covers.
func jsonLinesRenderer(to io.Writer) func(repository.Record) error {
	encoder := json.NewEncoder(to)
	return func(record repository.Record) error {
		if record.ID.IsZero() {
			return nil
		}
		if err := encoder.Encode(EntryOutput(record)); err != nil {
			return shared.Internalf("audit: writing an exported entry: %w", err)
		}
		return nil
	}
}

// exportColumns is the CSV file's shape, and it is fixed rather than derived from what an entry
// happens to carry: a spreadsheet somebody built a formula on must not gain or lose a column
// because an entry had no target.
var exportColumns = []string{
	"id", "seq", "occurred_at", "action", "outcome", "severity",
	"actor_type", "actor_id", "actor_label", "on_behalf_of",
	"target_type", "target_id", "target_label",
	"request_id", "trace_id", "ip_prefix", "user_agent_class", "api_client", "rule_id",
	"legal_basis", "changes", "hash",
}

func csvRenderer(to io.Writer) (func(repository.Record) error, error) {
	rows := csv.NewWriter(to)
	if err := rows.Write(exportColumns); err != nil {
		return nil, shared.Internalf("audit: writing the export header: %w", err)
	}

	return func(record repository.Record) error {
		if record.ID.IsZero() {
			rows.Flush()
			return rows.Error()
		}

		entry := record.Entry
		// The masked changes travel as JSON inside one column. A column per field would be a
		// column set that changed with the data, which is what a spreadsheet cannot have.
		changes, err := json.Marshal(changesOutput(entry.Changes))
		if err != nil {
			return shared.Internalf("audit: writing the changes of an exported entry: %w", err)
		}

		return rows.Write([]string{
			record.ID.String(), fmt.Sprint(record.Seq), entry.OccurredAt.UTC().Format(time.RFC3339Nano),
			string(entry.Action), string(entry.Outcome), string(entry.Severity),
			string(entry.ActorKind), entry.ActorID.String(), entry.ActorLabel, entry.OnBehalfOf.String(),
			entry.TargetType, entry.TargetID.String(), entry.TargetLabel,
			entry.Context.RequestID, entry.Context.TraceID, entry.Context.IPTruncated,
			entry.Context.UserAgentClass, entry.Context.APIClient, entry.Context.RuleID.String(),
			entry.LegalBasis, string(changes), hex.EncodeToString(record.Hash),
		})
	}, nil
}

// sign seals the manifest's digest under the installation's master key.
func (a Archivist) sign(ctx context.Context, exportID shared.ID, manifest []byte) (ExportSignature, error) {
	sum := sha256.Sum256(manifest)
	digest := hex.EncodeToString(sum[:])

	sealed, err := a.Encryptor.Seal(ctx, secret.New(digest),
		crypto.Purpose(signaturePurpose+exportID.String()))
	if err != nil {
		return ExportSignature{}, err
	}

	return ExportSignature{
		Algorithm: "AES-256-GCM over SHA-256, installation master key",
		KeyID:     sealed.KeyID,
		Over:      exportManifestName,
		Digest:    digest,
		Value:     base64.StdEncoding.EncodeToString(sealed.Ciphertext),
	}, nil
}

// put writes one small member and records its checksum.
func (a Archivist) put(
	ctx context.Context, store backupstorage.Store, key string, content []byte,
	checksums *archive.Checksums, name string,
) error {
	sum := sha256.Sum256(content)
	if err := checksums.Add(name, hex.EncodeToString(sum[:])); err != nil {
		return err
	}
	buffer := bytesBuffer{bytes: content}
	if _, err := store.Put(ctx, key, buffer.reader()); err != nil {
		return err
	}
	return nil
}

// bytesBuffer is the manifest and the checksums on their way to the target: both are kilobytes by
// construction, so they are the two members this format does hold in memory.
type bytesBuffer struct{ bytes []byte }

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.bytes = append(b.bytes, p...)
	return len(p), nil
}

func (b bytesBuffer) reader() io.Reader { return bytes.NewReader(b.bytes) }
