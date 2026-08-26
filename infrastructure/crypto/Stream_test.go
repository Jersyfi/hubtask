// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// The chunk size of the construction. Named here so that the sizes below are visibly about the
// boundaries rather than about round numbers.
const chunk = 64 << 10

const member = port.Purpose("backup/archive:0192f000-0000-7000-8000-00000000000a/data/comments.jsonl")

func streamKey(t *testing.T, seed byte) secret.Bytes {
	t.Helper()
	material := make([]byte, 32)
	for i := range material {
		material[i] = seed + byte(i)
	}
	return secret.NewBytes(material)
}

func cipherStream(seed byte) crypto.Stream {
	return crypto.NewStream(clockport.FixedEntropy{Seed: seed})
}

func seal(t *testing.T, s crypto.Stream, key secret.Bytes, purpose port.Purpose, plaintext []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	writer, err := s.Seal(&out, key, purpose)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return out.Bytes()
}

func open(t *testing.T, s crypto.Stream, key secret.Bytes, purpose port.Purpose, ciphertext []byte) ([]byte, error) {
	t.Helper()
	reader, err := s.Open(bytes.NewReader(ciphertext), key, purpose)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(reader)
}

func payload(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte('a' + i%26)
	}
	return out
}

// The sizes that matter are the ones around a chunk boundary: a stream whose plaintext is an exact
// multiple of the chunk still has to end with a final chunk, or truncation stops being detectable.
func TestAStreamOfAnySizeSurvivesTheRoundTrip(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)

	for _, size := range []int{0, 1, 1000, chunk - 1, chunk, chunk + 1, 2 * chunk, 3*chunk + 17} {
		plaintext := payload(size)

		read, err := open(t, stream, key, member, seal(t, stream, key, member, plaintext))
		if err != nil {
			t.Fatalf("%d bytes: open: %v", size, err)
		}
		if !bytes.Equal(read, plaintext) {
			t.Fatalf("%d bytes: %d came back", size, len(read))
		}
	}
}

// BK-2's first half: without the key the archive is bytes.
func TestWithoutTheKeyAnArchiveIsUnreadable(t *testing.T) {
	stream := cipherStream(7)
	plaintext := payload(3 * chunk)
	ciphertext := seal(t, stream, streamKey(t, 1), member, plaintext)

	if bytes.Contains(ciphertext, plaintext[:64]) {
		t.Fatal("the plaintext is visible in the ciphertext")
	}

	_, err := open(t, stream, streamKey(t, 2), member, ciphertext)
	if err == nil {
		t.Fatal("another key opened the stream")
	}
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("category: %v", err)
	}
}

// A truncated transfer is the most common way an archive goes wrong. A cipher that could not tell
// a finished stream from a cut one would restore three quarters of a tenant without saying so.
func TestATruncatedStreamFailsRatherThanEndingQuietly(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	full := seal(t, stream, key, member, payload(3*chunk))

	// Cut at a chunk boundary, which is what a transfer that died between two writes leaves.
	boundary := 8 + 2*(chunk+16)
	if _, err := open(t, stream, key, member, full[:boundary]); err == nil {
		t.Fatal("a stream cut at a chunk boundary opened")
	}
	// And cut in the middle of a chunk.
	if _, err := open(t, stream, key, member, full[:boundary+100]); err == nil {
		t.Fatal("a stream cut inside a chunk opened")
	}
	// A stream with nothing but a header is a run that died before it wrote anything.
	if _, err := open(t, stream, key, member, full[:8]); err == nil {
		t.Fatal("a stream with no chunks at all opened")
	}
}

// A writer that was never closed wrote no final chunk. Reading it back must fail rather than
// return the part that happened to arrive.
func TestAStreamThatWasNeverClosedIsNotAStream(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)

	var out bytes.Buffer
	writer, err := stream.Seal(&out, key, member)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := writer.Write(payload(2 * chunk)); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := open(t, stream, key, member, out.Bytes()); err == nil {
		t.Fatal("an unclosed stream opened")
	}
}

func TestOneFlippedBitIsFound(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	ciphertext := seal(t, stream, key, member, payload(2*chunk+5))

	for _, at := range []int{0, 3, 9, chunk, chunk + 40, len(ciphertext) - 1} {
		altered := bytes.Clone(ciphertext)
		altered[at] ^= 0x01

		if _, err := open(t, stream, key, member, altered); err == nil {
			t.Fatalf("a flipped bit at %d went unnoticed", at)
		}
	}
}

// The purpose binds a member to its place. A data file presented under another data file's name
// fails exactly as a flipped bit does - which is what stops an archive being reassembled from
// members of two different runs.
func TestAMemberPresentedUnderAnotherNameDoesNotOpen(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	ciphertext := seal(t, stream, key, member, payload(1000))

	other := port.Purpose("backup/archive:0192f000-0000-7000-8000-00000000000a/data/labels.jsonl")
	if _, err := open(t, stream, key, other, ciphertext); err == nil {
		t.Fatal("a member opened under another member's purpose")
	}
	if _, err := open(t, stream, key, member, ciphertext); err != nil {
		t.Fatalf("the same member under its own purpose: %v", err)
	}
}

// Chunks carry their number, so a stream reassembled in the wrong order does not open.
func TestReorderedChunksDoNotOpen(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	ciphertext := seal(t, stream, key, member, payload(3*chunk))

	const header, sealedChunk = 8, chunk + 16
	swapped := bytes.Clone(ciphertext)
	first := bytes.Clone(swapped[header : header+sealedChunk])
	second := bytes.Clone(swapped[header+sealedChunk : header+2*sealedChunk])
	copy(swapped[header:], second)
	copy(swapped[header+sealedChunk:], first)

	if _, err := open(t, stream, key, member, swapped); err == nil {
		t.Fatal("two swapped chunks opened")
	}
}

// A key of the wrong length is a caller that derived the wrong thing. Using the first 32 bytes of
// it would produce an archive that opens only by the same accident.
func TestAKeyOfTheWrongLengthIsRefused(t *testing.T) {
	stream := cipherStream(7)

	for _, length := range []int{0, 16, 31, 33, 64} {
		short := secret.NewBytes(make([]byte, length))
		if _, err := stream.Seal(io.Discard, short, member); err == nil {
			t.Fatalf("a %d-byte key sealed", length)
		}
		if _, err := stream.Open(strings.NewReader("x"), short, member); err == nil {
			t.Fatalf("a %d-byte key opened", length)
		}
	}
	if stream.KeyBytes() != 32 {
		t.Fatalf("KeyBytes %d, want 32 - §4 says AES-256", stream.KeyBytes())
	}
}

// A stream version this build does not know is refused before anything is decrypted, for the
// reason the manifest's format version is: a partial read of a shape that is guessed is worse
// than no read at all.
func TestAStreamVersionFromTheFutureIsRefused(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	ciphertext := seal(t, stream, key, member, payload(100))
	ciphertext[0] = 99

	_, err := stream.Open(bytes.NewReader(ciphertext), key, member)
	if err == nil {
		t.Fatal("a stream version from the future was accepted")
	}
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != port.CodeCiphertextMalformed {
		t.Fatalf("detail code: %v", err)
	}
}

// The entropy source failing is an error rather than a nonce of zeroes.
func TestAStreamWithoutEntropyIsRefused(t *testing.T) {
	stream := crypto.NewStream(exhausted{})

	if _, err := stream.Seal(io.Discard, streamKey(t, 1), member); !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("sealing with no entropy: %v", err)
	}
}

// Nothing of the plaintext reaches the caller before its chunk has been authenticated. A reader
// that handed out bytes and failed afterwards would have already written them into a restore.
func TestNothingIsReturnedBeforeItIsAuthentic(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	ciphertext := seal(t, stream, key, member, payload(2*chunk))
	ciphertext[8+16] ^= 0x01 // inside the first chunk

	reader, err := stream.Open(bytes.NewReader(ciphertext), key, member)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	buffer := make([]byte, 1)
	if n, err := reader.Read(buffer); err == nil || n != 0 {
		t.Fatalf("read %d bytes of an altered chunk (err %v)", n, err)
	}
}

// One byte at a time in, one byte at a time out: the writer's chunking must not depend on how the
// caller happens to slice its writes.
func TestTheChunkingDoesNotDependOnHowTheCallerWrites(t *testing.T) {
	stream := cipherStream(7)
	key := streamKey(t, 1)
	plaintext := payload(chunk + 3)

	var atOnce bytes.Buffer
	writer, err := stream.Seal(&atOnce, key, member)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	var oneAtATime bytes.Buffer
	writer, err = stream.Seal(&oneAtATime, key, member)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	for i := range plaintext {
		if _, err := writer.Write(plaintext[i : i+1]); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if !bytes.Equal(atOnce.Bytes(), oneAtATime.Bytes()) {
		t.Fatal("the ciphertext depends on how the caller sliced its writes")
	}
}
