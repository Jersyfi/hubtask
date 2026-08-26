// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package crypto

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	clockport "github.com/Jersyfi/hubtask/core/port/clock"
	port "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The layout of an encrypted stream, version 1:
//
//	[0]        version
//	[1:8]      the nonce prefix, drawn once per stream
//	[8:]       chunks: 65536 bytes of plaintext each, plus GCM's 16-byte tag
//
// Every chunk is sealed under a nonce of prefix ‖ counter ‖ final, so no two chunks of a stream
// and no two streams under one key ever share one. The counter is what stops chunks being
// reordered and the final flag is what stops the stream being cut short: the last chunk is sealed
// with a different byte from every other, so a truncated stream fails to open rather than opening
// to a prefix of itself.
//
// This is the construction Hoang, Reyhanitabar, Rogaway and Vizár published as STREAM. It is
// written out here rather than pulled in because it is thirty lines and a dependency is a supply
// chain decision (CLAUDE.md, "What you do not decide yourself").
const (
	streamVersion = 1

	// streamChunkBytes is the plaintext each chunk carries.
	//
	// 64 KiB is a compromise with two ends. Larger chunks mean less tag overhead and fewer GCM
	// calls; smaller chunks mean less memory held per concurrent stream and a finer granularity
	// for a reader that stops early. At this size the overhead is 0.02% and a backup run writing
	// four members at once holds a quarter of a mebibyte.
	streamChunkBytes = 64 << 10

	streamPrefixBytes = 7
	streamCounterMax  = 0xFFFFFFFF

	streamHeaderBytes = 1 + streamPrefixBytes
	streamCipherBytes = streamChunkBytes + tagBytes
)

// streamLabel is the additional-data prefix. Separate from the envelope's two labels, so that a
// chunk of a stream can never be presented as the body or the wrapped key of an envelope.
const streamLabel = "hubtask/stream/v1:"

// Stream is the archive's cipher: AES-256-GCM applied chunk by chunk, so that a member of any size
// is encrypted on its way to a target without ever being held (backup-restore.md §4).
//
// It shares a package with Envelope rather than living beside its caller because of the seam
// TestCryptographyStaysInItsAdapter enforces: one place in this repository names a cipher, has one
// nonce discipline, and is the one file that changes when open point S-2 moves the key material
// somewhere else.
type Stream struct {
	entropy clockport.Entropy
}

// NewStream builds the cipher. The entropy source is a port for the reason every source of
// non-determinism in this system is: a test that cannot fix the nonce cannot assert the bytes.
func NewStream(entropy clockport.Entropy) Stream { return Stream{entropy: entropy} }

var _ port.StreamCipher = Stream{}

// KeyBytes is 32 - AES-256, as §4 requires.
func (s Stream) KeyBytes() int { return dataKeyBytes }

func (s Stream) block(key secret.Bytes) (cipher.AEAD, error) {
	if key.Len() != dataKeyBytes {
		// Refused rather than stretched or truncated. A key of the wrong length is a caller that
		// derived the wrong thing, and quietly using the first 32 bytes of it would produce an
		// archive that opens only by the same accident.
		return nil, shared.ErrValidation.WithDetail("crypto.key_unusable").
			WithCause(fmt.Errorf("a stream key is %d bytes, got %d", dataKeyBytes, key.Len()))
	}
	block, err := aes.NewCipher(key.Reveal())
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("crypto.key_unusable").WithCause(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("crypto.key_unusable").WithCause(err)
	}
	return aead, nil
}

// nonce builds the nonce of one chunk: the stream's prefix, the chunk's number, and whether it is
// the last one. Twelve bytes, fixed width, nothing a caller could lie about.
func nonce(prefix []byte, counter uint32, final bool) []byte {
	out := make([]byte, nonceBytes)
	copy(out, prefix)
	binary.BigEndian.PutUint32(out[streamPrefixBytes:], counter)
	if final {
		out[nonceBytes-1] = 1
	}
	return out
}

// Seal wraps a writer. Every chunk is encrypted as it fills; nothing is held beyond one chunk.
func (s Stream) Seal(w io.Writer, key secret.Bytes, purpose port.Purpose) (io.WriteCloser, error) {
	aead, err := s.block(key)
	if err != nil {
		return nil, err
	}
	prefix, err := s.entropy.Bytes(streamPrefixBytes)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("crypto.entropy_unavailable").
			WithCause(fmt.Errorf("drawing a stream nonce: %w", err))
	}

	header := make([]byte, 0, streamHeaderBytes)
	header = append(header, streamVersion)
	header = append(header, prefix...)
	if _, err := w.Write(header); err != nil {
		return nil, shared.Internalf("crypto: write stream header: %w", err)
	}

	return &sealed{
		to:     w,
		aead:   aead,
		prefix: prefix,
		aad:    []byte(streamLabel + string(purpose)),
		buffer: make([]byte, 0, streamChunkBytes),
		out:    make([]byte, 0, streamCipherBytes),
	}, nil
}

type sealed struct {
	to     io.Writer
	aead   cipher.AEAD
	prefix []byte
	aad    []byte
	buffer []byte
	out    []byte

	counter uint32
	closed  bool
}

// Write fills the chunk buffer and emits a chunk whenever it is full and more is still coming.
//
// A full buffer is not emitted on the spot, and that is the whole trick: the last chunk carries a
// different nonce from every other, so which chunk is last has to be known before it is sealed.
// Holding a full buffer until the next byte arrives - or until Close says there is none - is how
// that is known without reading ahead.
func (s *sealed) Write(p []byte) (int, error) {
	if s.closed {
		return 0, shared.Internalf("crypto: write to a closed stream")
	}
	written := len(p)
	for len(p) > 0 {
		if len(s.buffer) == streamChunkBytes {
			if err := s.emit(false); err != nil {
				return 0, err
			}
		}
		take := min(streamChunkBytes-len(s.buffer), len(p))
		s.buffer = append(s.buffer, p[:take]...)
		p = p[take:]
	}
	return written, nil
}

// Close seals whatever is left as the final chunk, always - an empty plaintext still produces one,
// so that an empty member is an authenticated empty member rather than an empty file anybody could
// have put there.
func (s *sealed) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.emit(true)
}

func (s *sealed) emit(final bool) error {
	if s.counter == streamCounterMax && !final {
		// 2^32 chunks of 64 KiB is 256 tebibytes in one member. Reaching it means a defect
		// upstream rather than a large tenant, and continuing would reuse a nonce.
		return shared.Internalf("crypto: stream longer than %d chunks", uint64(streamCounterMax))
	}
	s.out = s.aead.Seal(s.out[:0], nonce(s.prefix, s.counter, final), s.buffer, s.aad)
	if _, err := s.to.Write(s.out); err != nil {
		return shared.Internalf("crypto: write stream chunk: %w", err)
	}
	s.buffer = s.buffer[:0]
	s.counter++
	return nil
}

// Open unwraps a reader. Nothing reaches the caller before its chunk has been authenticated.
func (s Stream) Open(r io.Reader, key secret.Bytes, purpose port.Purpose) (io.Reader, error) {
	aead, err := s.block(key)
	if err != nil {
		return nil, err
	}

	// The reader is buffered because the end of the stream is recognised by looking one byte
	// ahead: a chunk that fills completely is the last one only if nothing follows it.
	buffered := bufio.NewReaderSize(r, streamCipherBytes)

	header := make([]byte, streamHeaderBytes)
	if _, err := io.ReadFull(buffered, header); err != nil {
		return nil, shared.ErrValidation.WithDetail(portCodeCiphertextBroken).
			WithCause(fmt.Errorf("reading the stream header: %w", err))
	}
	if header[0] != streamVersion {
		return nil, shared.ErrValidation.WithDetail(portCodeCiphertextBroken).
			WithCause(fmt.Errorf("stream version %d, this build writes %d", header[0], streamVersion))
	}

	return &opened{
		from:   buffered,
		aead:   aead,
		prefix: header[1:],
		aad:    []byte(streamLabel + string(purpose)),
		in:     make([]byte, streamCipherBytes),
		plain:  make([]byte, 0, streamChunkBytes),
	}, nil
}

type opened struct {
	from   *bufio.Reader
	aead   cipher.AEAD
	prefix []byte
	aad    []byte
	in     []byte

	// buf backs the plaintext of one chunk and is reused for the next, which is what keeps a
	// stream of any length at one chunk of memory. plain is the part of it Read has not handed
	// out yet, resliced forward as it goes.
	buf     []byte
	plain   []byte
	counter uint32
	done    bool
}

// Read hands out the plaintext of chunks that have already been authenticated, and opens the next
// one when the last is used up.
func (o *opened) Read(p []byte) (int, error) {
	for len(o.plain) == 0 {
		if o.done {
			return 0, io.EOF
		}
		if err := o.next(); err != nil {
			return 0, err
		}
	}
	n := copy(p, o.plain)
	o.plain = o.plain[n:]
	return n, nil
}

func (o *opened) next() error {
	read, err := io.ReadFull(o.from, o.in)
	final := false
	switch {
	case errors.Is(err, io.EOF):
		// Nothing at all where a chunk was expected. Every stream ends with a final chunk, so
		// this is a stream that was cut at a chunk boundary - the most common way a transfer goes
		// wrong, and the one a plain cipher would not notice.
		return notAuthentic(errors.New("the stream ends without a final chunk"))
	case errors.Is(err, io.ErrUnexpectedEOF):
		final = true
	case err != nil:
		return shared.ErrUnavailable.WithDetail(portCodeCiphertextBroken).WithCause(err)
	default:
		// A full chunk. It is the last one only if nothing follows it.
		if _, peek := o.from.Peek(1); errors.Is(peek, io.EOF) {
			final = true
		} else if peek != nil {
			return shared.ErrUnavailable.WithDetail(portCodeCiphertextBroken).WithCause(peek)
		}
	}
	if read < tagBytes {
		return notAuthentic(errors.New("a chunk shorter than its own tag"))
	}

	plain, err := o.aead.Open(o.buf[:0], nonce(o.prefix, o.counter, final), o.in[:read], o.aad)
	if err != nil {
		// One answer for a wrong key, an altered byte, a reordered chunk, a truncated stream and
		// a member presented under another member's name. Telling them apart would tell somebody
		// probing which of their guesses was closer (core/port/crypto, CodeNotAuthentic).
		return notAuthentic(err)
	}
	o.buf = plain
	o.plain = plain
	o.counter++
	o.done = final
	return nil
}

func notAuthentic(cause error) error {
	return shared.ErrValidation.WithDetail(port.CodeNotAuthentic).WithCause(cause)
}
