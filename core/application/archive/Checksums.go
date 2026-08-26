// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package archive

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Checksums is `checksums.txt`: one SHA-256 per member, and one over the manifest.
//
// The file is written **last**, and that is the decision this type carries. An archive is a
// directory of objects rather than one stream, so there is no single moment at which it becomes
// complete unless one is chosen - and a run that died between two members has to be
// distinguishable from one that finished. The last thing written is that marker: an archive
// without `checksums.txt` is an unfinished archive, whatever else is lying next to it, and the
// listing in §8.1 reports exactly that as its checksum status.
//
// The manifest is inside this file rather than the other way round for the same reason. The
// manifest names the members and their checksums, so it cannot name its own; something has to
// close over it, and closing over it last is what makes the closure meaningful.
//
// What this catches is corruption, not an attacker. Somebody who can rewrite a member at the
// target can rewrite this file too. Against them the defence is elsewhere and stronger: the
// members are encrypted with an authenticated cipher, so an altered byte fails to open at all.
// These checksums are for the bit that rotted, the transfer that truncated, and the disk that
// lied - which is what actually happens to backups.
type Checksums struct {
	// digests is path -> lower-case hexadecimal SHA-256. A map rather than a slice, because a
	// member written twice is a defect and a map makes it one rather than two lines.
	digests map[string]string
}

// NewChecksums starts an empty set.
func NewChecksums() *Checksums { return &Checksums{digests: map[string]string{}} }

// Add records one member's digest. A path recorded twice with different digests is a defect in
// the writer, and it is refused rather than silently kept.
func (c *Checksums) Add(path, digest string) error {
	if path == "" || !looksLikeDigest(digest) {
		return shared.ErrValidation.WithDetail(CodeChecksumsUnreadable).
			WithCause(errors.New("checksum line without a path or without a digest"))
	}
	if existing, seen := c.digests[path]; seen && existing != digest {
		return shared.Internalf("archive: %s written twice with different content", path)
	}
	c.digests[path] = digest
	return nil
}

// Digest answers what was recorded for a path.
func (c *Checksums) Digest(path string) (string, bool) {
	digest, found := c.digests[path]
	return digest, found
}

// Paths answers every recorded path, sorted.
func (c *Checksums) Paths() []string { return slices.Sorted(maps.Keys(c.digests)) }

// Encode writes the file in the shape `sha256sum` reads: the digest, two spaces, the path, sorted
// by path so that the same archive produces the same bytes twice.
//
// Reusing that shape is deliberate. An operator holding a directory at a target and no Hubtask can
// check it with a tool that has been on every machine for thirty years, which is worth more on the
// day the database is gone than a format of our own would be.
func (c *Checksums) Encode(w io.Writer) error {
	buffered := bufio.NewWriter(w)
	for _, path := range c.Paths() {
		if _, err := buffered.WriteString(c.digests[path] + "  " + path + "\n"); err != nil {
			return shared.Internalf("archive: write checksums: %w", err)
		}
	}
	if err := buffered.Flush(); err != nil {
		return shared.Internalf("archive: flush checksums: %w", err)
	}
	return nil
}

// checksumsMaxBytes bounds the file. A line is under 128 bytes and there are as many lines as
// there are data files plus one; a mebibyte is thousands of times more than that and still a
// ceiling on what somebody else's storage can hand back (T-17).
const checksumsMaxBytes = 1 << 20

// ParseChecksums reads the file back.
func ParseChecksums(r io.Reader) (*Checksums, error) {
	unreadable := func(cause error) error {
		return shared.ErrValidation.WithDetail(CodeChecksumsUnreadable).WithCause(cause)
	}

	checksums := NewChecksums()
	scanner := bufio.NewScanner(io.LimitReader(r, checksumsMaxBytes+1))
	var read int
	for scanner.Scan() {
		read += len(scanner.Bytes()) + 1
		if read > checksumsMaxBytes {
			return nil, unreadable(errors.New("checksums.txt is larger than any checksums.txt"))
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		digest, path, found := strings.Cut(line, "  ")
		if !found {
			return nil, unreadable(errors.New("a line without the two spaces sha256sum writes"))
		}
		if err := checksums.Add(path, digest); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, unreadable(err)
	}
	if len(checksums.digests) == 0 {
		return nil, unreadable(errors.New("no lines"))
	}
	return checksums, nil
}

// Verify checks one member against what was recorded, and says which member failed rather than
// only that something did.
//
// A path that is not in the file is a failure too. An archive that grew a member after the
// checksums were written is not an archive with an extra file in it - it is an archive somebody
// has been editing.
func (c *Checksums) Verify(path string, content io.Reader) error {
	recorded, found := c.digests[path]
	if !found {
		return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
			WithParams(map[string]string{"path": path, "reason": "not_listed"}).
			WithCause(errors.New("checksums.txt does not list " + path))
	}
	digest, err := Digest(content)
	if err != nil {
		return err
	}
	if digest != recorded {
		return shared.ErrValidation.WithDetail(CodeChecksumMismatch).
			WithParams(map[string]string{"path": path, "reason": "mismatch"}).
			WithCause(errors.New("the bytes at " + path + " are not the bytes that were written"))
	}
	return nil
}

// Digest is the SHA-256 of a stream, hexadecimal and lower case - the one spelling used for a
// checksum line, a media address and a manifest entry alike.
func Digest(r io.Reader) (string, error) {
	sum := sha256.New()
	if _, err := io.Copy(sum, r); err != nil {
		return "", shared.ErrUnavailable.WithDetail(CodeChecksumsUnreadable).WithCause(err)
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// Counter wraps a writer and answers, afterwards, how many bytes went through it and what their
// SHA-256 is.
//
// It exists because both numbers are needed for every member and neither may cost a second pass.
// A member is written once, to a target that is somebody else's machine; reading it back to hash
// it would double the traffic on the one operation that is already the slowest thing the system
// does.
type Counter struct {
	sum   hash.Hash
	bytes int64
}

// NewCounter starts counting.
func NewCounter() *Counter { return &Counter{sum: sha256.New()} }

// Write implements io.Writer. It hashes rather than stores: the member itself is going to the
// target, and nothing here holds a copy of it (T-17).
func (c *Counter) Write(p []byte) (int, error) {
	c.bytes += int64(len(p))
	return c.sum.Write(p)
}

// Bytes is how much went through.
func (c *Counter) Bytes() int64 { return c.bytes }

// Digest is the SHA-256 of everything that went through.
func (c *Counter) Digest() string { return hex.EncodeToString(c.sum.Sum(nil)) }

// The refusals of the checksum file.
const (
	// CodeChecksumsUnreadable is a `checksums.txt` that is not one.
	CodeChecksumsUnreadable = "backup.archive_checksums_unreadable"
	// CodeChecksumMismatch is a member whose bytes are not the bytes that were written, or one
	// the file does not list at all.
	CodeChecksumMismatch = "backup.archive_checksum_mismatch"
	// CodeArchiveIncomplete is an archive with no `checksums.txt`: a run that died before it
	// finished, or one still in progress. Distinct from a corrupt archive, because the answer is
	// different - this one is not damaged, it is simply not there yet.
	CodeArchiveIncomplete = "backup.archive_incomplete"
)
