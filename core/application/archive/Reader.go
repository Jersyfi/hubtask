// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// MemberPurpose binds a member to its archive and its path, so that one lifted out of an archive
// and dropped into another stops opening (core/port/crypto, Purpose).
//
// A function shared by the writer and the reader rather than a string built twice: two spellings
// of this would be an archive that writes correctly and never opens, and the failure would look
// exactly like a wrong key.
func MemberPurpose(archiveID, path string) crypto.Purpose {
	return crypto.Purpose("backup/archive:" + archiveID + "/" + path)
}

// Description is one archive as the target knows it: what its manifest says, whether the run that
// wrote it finished, and how much room it takes.
//
// It is what `GET /backup-targets/{id}/backups` answers, and it is assembled from the target alone
// - no row in any database is consulted. That is the property §8.1 promises and the one that makes
// a restore possible after a total loss.
type Description struct {
	// Prefix is the archive's directory at the target, and what every other call here takes.
	Prefix   string
	Manifest Manifest
	// Complete is whether checksums.txt is there. An archive without it is a run that died or one
	// still going; it is not damaged, and the difference matters to whoever is deciding what to
	// restore from.
	Complete bool
	// Bytes is what the archive occupies at the target, members and media together.
	Bytes int64
}

// Reader opens archives at a target.
type Reader struct {
	store  backupstorage.Store
	cipher crypto.StreamCipher
}

// NewReader builds a reader for one target.
func NewReader(store backupstorage.Store, cipher crypto.StreamCipher) *Reader {
	return &Reader{store: store, cipher: cipher}
}

// List answers every archive beneath a prefix, newest first.
//
// One listing, not one per archive. A target is somebody else's machine and a listing is the
// expensive call; grouping the keys locally is free.
func (r *Reader) List(ctx context.Context, prefix string) ([]Description, error) {
	entries, err := r.store.List(ctx, prefix)
	if err != nil {
		return nil, err
	}

	// A directory is an archive when it holds a manifest. Anything else under the prefix belongs
	// to somebody else and is left alone - BK-8 requires that other files at a target stay
	// untouched, and not listing them is where that starts.
	grouped := map[string][]backupstorage.Entry{}
	for _, entry := range entries {
		if at, _, isManifest := strings.Cut(entry.Key, "/"+ManifestName); isManifest {
			grouped[at] = nil
		}
	}
	for _, entry := range entries {
		if at := archive(entry.Key); at != "" {
			if _, known := grouped[at]; known {
				grouped[at] = append(grouped[at], entry)
			}
		}
	}

	descriptions := make([]Description, 0, len(grouped))
	for at, own := range grouped {
		description, err := r.describe(ctx, at, own)
		if err != nil {
			// One unreadable archive does not hide the others. An operator looking for something
			// to restore from is worse served by an error than by a list with a gap in it, and
			// the gap is visible: the directory is there and not in the answer.
			continue
		}
		descriptions = append(descriptions, description)
	}
	slices.SortFunc(descriptions, func(a, b Description) int {
		return strings.Compare(b.Prefix, a.Prefix) // the name sorts by time, newest first
	})
	return descriptions, nil
}

// Describe reads one archive's manifest and says whether the run that wrote it finished.
func (r *Reader) Describe(ctx context.Context, prefix string) (Description, error) {
	entries, err := r.store.List(ctx, prefix+"/")
	if err != nil {
		return Description{}, err
	}
	return r.describe(ctx, prefix, entries)
}

func (r *Reader) describe(ctx context.Context, prefix string, entries []backupstorage.Entry) (Description, error) {
	content, err := r.store.Get(ctx, prefix+"/"+ManifestName)
	if err != nil {
		return Description{}, err
	}
	defer func() { _ = content.Close() }()

	manifest, err := ReadManifest(content)
	if err != nil {
		return Description{}, err
	}

	description := Description{Prefix: prefix, Manifest: manifest}
	for _, entry := range entries {
		description.Bytes += entry.Size
		if entry.Key == prefix+"/"+ChecksumsName {
			description.Complete = true
		}
	}
	return description, nil
}

// Chain answers the archives a restore has to read, newest first, back to the full archive at the
// root.
//
// It walks the parents by reading manifests rather than by matching identifiers against a listing,
// because a manifest names where its parent lies. A chain that does not end at a full archive is a
// refusal: restoring the part of a chain that happens to be present would restore a tenant to a
// state it was never in.
func (r *Reader) Chain(ctx context.Context, prefix string) ([]Description, error) {
	var chain []Description
	seen := map[string]bool{}

	for at := prefix; at != ""; {
		if seen[at] {
			return nil, shared.ErrValidation.WithDetail(CodeChainBroken).
				WithParams(map[string]string{"prefix": at, "reason": "cycle"}).
				WithCause(errors.New("the chain of parents is a cycle"))
		}
		seen[at] = true

		description, err := r.Describe(ctx, at)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return nil, shared.ErrValidation.WithDetail(CodeChainBroken).
					WithParams(map[string]string{"prefix": at, "reason": "missing"}).WithCause(err)
			}
			return nil, err
		}
		if !description.Complete {
			return nil, shared.ErrValidation.WithDetail(CodeArchiveIncomplete).
				WithParams(map[string]string{"prefix": at}).
				WithCause(errors.New("an archive in the chain has no checksums.txt"))
		}
		chain = append(chain, description)

		if description.Manifest.Mode == ModeFull {
			return chain, nil
		}
		if description.Manifest.ParentPrefix == "" {
			return nil, shared.ErrValidation.WithDetail(CodeChainBroken).
				WithParams(map[string]string{"prefix": at, "reason": "no_parent_prefix"}).
				WithCause(errors.New("an incremental that does not say where its parent lies"))
		}
		at = description.Manifest.ParentPrefix
	}
	return nil, shared.ErrValidation.WithDetail(CodeChainBroken).
		WithCause(errors.New("the chain does not end at a full archive"))
}

// Verify checks an archive at the target without restoring it, and without the archive key - which
// is what `POST /backups/{id}:verify` promises (backup-restore.md §3).
//
// Every member named in checksums.txt is read and hashed, including the manifest. Media are
// checked for presence rather than for content when the archive is encrypted: their name is the
// digest of the plaintext, and the bytes at the target are the ciphertext. On an unencrypted
// archive the name is checkable and is checked - the address is the checksum, which is the second
// thing content addressing pays for.
func (r *Reader) Verify(ctx context.Context, prefix string) error {
	description, err := r.Describe(ctx, prefix)
	if err != nil {
		return err
	}
	if !description.Complete {
		return shared.ErrValidation.WithDetail(CodeArchiveIncomplete).
			WithParams(map[string]string{"prefix": prefix}).
			WithCause(errors.New("no checksums.txt"))
	}

	listed, err := r.checksums(ctx, prefix)
	if err != nil {
		return err
	}

	// Every member the manifest names has to be in checksums.txt with the same digest. The two
	// files are written by the same run and disagreeing is not corruption of one of them - it is
	// an archive that has been assembled from parts.
	for _, file := range description.Manifest.Files {
		recorded, found := listed.Digest(file.Path)
		if !found || recorded != file.SHA256 {
			return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
				WithParams(map[string]string{"path": file.Path, "reason": "manifest_disagrees"}).
				WithCause(errors.New("the manifest and checksums.txt name different bytes"))
		}
	}

	for _, path := range listed.Paths() {
		if err := r.verifyOne(ctx, listed, prefix, path); err != nil {
			return err
		}
	}
	return r.verifyMedia(ctx, description)
}

func (r *Reader) verifyOne(ctx context.Context, listed *Checksums, prefix, path string) error {
	content, err := r.store.Get(ctx, prefix+"/"+path)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
				WithParams(map[string]string{"path": path, "reason": "missing"}).WithCause(err)
		}
		return err
	}
	defer func() { _ = content.Close() }()
	return listed.Verify(path, content)
}

// verifyMedia checks that every medium is there, and - on an unencrypted archive - that its bytes
// are the bytes its name claims.
func (r *Reader) verifyMedia(ctx context.Context, description Description) error {
	if description.Manifest.MediaCount == 0 {
		return nil
	}
	entries, err := r.store.List(ctx, description.Prefix+"/"+MediaPrefix)
	if err != nil {
		return err
	}
	if int64(len(entries)) != description.Manifest.MediaCount {
		return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
			WithParams(map[string]string{"path": MediaPrefix, "reason": "count"}).
			WithCause(errors.New("the archive holds a different number of media than the manifest names"))
	}
	if description.Manifest.Encryption.IsEncrypted() {
		return nil
	}
	for _, entry := range entries {
		digest := entry.Key[strings.LastIndex(entry.Key, "/")+1:]
		content, err := r.store.Get(ctx, entry.Key)
		if err != nil {
			return err
		}
		actual, err := Digest(content)
		_ = content.Close()
		if err != nil {
			return err
		}
		if actual != digest {
			return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
				WithParams(map[string]string{"path": MediaName(digest), "reason": "address"}).
				WithCause(errors.New("a medium is not the bytes its address names"))
		}
	}
	return nil
}

// Records hands out one entity's records, in the order they were written.
//
// The member's checksum is verified as it is read rather than beforehand. A separate pass would
// read the whole file twice over somebody else's network, and it would also verify a different
// read from the one that was used - which is exactly the gap a flaky target lives in.
func (r *Reader) Records(
	ctx context.Context, description Description, entity Entity, key secret.Bytes, yield func(Record) error,
) error {
	path := DataName(entity.Name)
	file, found := fileOf(description.Manifest, path)
	if !found {
		// A member the manifest does not name is not read. On the optional audit trail that is
		// the ordinary case; on anything else it is an archive somebody has added a file to.
		if entity.Optional {
			return nil
		}
		return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
			WithParams(map[string]string{"path": path, "reason": "not_in_manifest"}).
			WithCause(errors.New("the manifest does not name " + path))
	}

	stored, err := r.store.Get(ctx, description.Prefix+"/"+path)
	if err != nil {
		return err
	}
	defer func() { _ = stored.Close() }()

	counter := NewCounter()
	plaintext, err := r.decrypt(io.TeeReader(stored, counter), description.Manifest, path, key)
	if err != nil {
		return err
	}
	if err := ReadRecords(plaintext, yield); err != nil {
		return err
	}
	// Whatever the cipher left unread has still to go past the counter, or the digest is of a
	// prefix of the member.
	if _, err := io.Copy(io.Discard, io.TeeReader(stored, counter)); err != nil {
		return shared.ErrUnavailable.WithDetail(backupstorage.CodeTargetFailed).WithCause(err)
	}
	if counter.Digest() != file.SHA256 {
		return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
			WithParams(map[string]string{"path": path, "reason": "mismatch"}).
			WithCause(errors.New("the bytes read are not the bytes the manifest names"))
	}
	return nil
}

// Medium opens one medium, searching the chain from newest to oldest.
//
// That search is the other half of "an incremental does not re-transfer a file that did not
// change": the file lives in whichever archive first referenced it, and a restore finds it by
// looking rather than by every archive carrying a copy.
func (r *Reader) Medium(
	ctx context.Context, chain []Description, digest string, key secret.Bytes,
) (io.ReadCloser, error) {
	if !looksLikeDigest(digest) {
		return nil, shared.ErrValidation.WithDetail(CodeRecordInvalid).
			WithParams(map[string]string{"reason": "blobs.sha256"}).
			WithCause(errors.New("not a content address"))
	}

	for _, description := range chain {
		content, err := r.store.Get(ctx, description.Prefix+"/"+MediaName(digest))
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		plaintext, err := r.decrypt(content, description.Manifest, MediaName(digest), key)
		if err != nil {
			_ = content.Close()
			return nil, err
		}
		return closing{Reader: plaintext, under: content}, nil
	}
	return nil, shared.ErrNotFound.WithDetail(CodeMediaMissing).
		WithParams(map[string]string{"sha256": digest})
}

// decrypt puts a member through the cipher, or hands it back untouched on an unencrypted archive.
func (r *Reader) decrypt(
	stored io.Reader, manifest Manifest, path string, key secret.Bytes,
) (io.Reader, error) {
	if !manifest.Encryption.IsEncrypted() {
		if key.Len() > 0 {
			// A key offered for an archive that has none is a caller confusing two archives, and
			// the confusion is worth stopping: the next thing they do is trust the result.
			return nil, shared.ErrValidation.WithDetail(CodeArchiveKeyUnexpected)
		}
		return stored, nil
	}
	if key.Len() == 0 {
		return nil, shared.ErrValidation.WithDetail(CodeArchiveKeyRequired).
			WithParams(map[string]string{"key_id": manifest.Encryption.KeyID})
	}
	return r.cipher.Open(stored, key, MemberPurpose(manifest.ArchiveID, path))
}

// closing keeps the target's stream alive for as long as the plaintext is being read, and closes
// it afterwards. Without it the caller would hold a decrypting reader over a connection nobody
// owns.
type closing struct {
	io.Reader
	under io.Closer
}

func (c closing) Close() error { return c.under.Close() }

func (r *Reader) checksums(ctx context.Context, prefix string) (*Checksums, error) {
	content, err := r.store.Get(ctx, prefix+"/"+ChecksumsName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = content.Close() }()
	return ParseChecksums(content)
}

func fileOf(manifest Manifest, path string) (File, bool) {
	index := slices.IndexFunc(manifest.Files, func(f File) bool { return f.Path == path })
	if index < 0 {
		return File{}, false
	}
	return manifest.Files[index], true
}

// archive answers which archive a key belongs to, or "" for a key that is not inside one.
func archive(key string) string {
	for _, member := range []string{"/" + ManifestName, "/" + ChecksumsName, "/" + DataPrefix, "/" + MediaPrefix} {
		if at := strings.LastIndex(key, member); at >= 0 {
			return key[:at]
		}
	}
	return ""
}

// The refusals of a read.
const (
	// CodeChainBroken is an incremental whose parent is missing, or a chain that does not end at
	// a full archive. Restoring the part that happens to be present would restore a tenant to a
	// state it was never in.
	CodeChainBroken = "backup.archive_chain_broken"
	// CodeArchiveKeyRequired is an encrypted archive read without its key.
	CodeArchiveKeyRequired = "backup.archive_key_required"
	// CodeArchiveKeyUnexpected is a key offered for an archive that has none - a caller confusing
	// two archives, which is worth stopping because the next thing they do is trust the result.
	CodeArchiveKeyUnexpected = "backup.archive_key_unexpected"
)
