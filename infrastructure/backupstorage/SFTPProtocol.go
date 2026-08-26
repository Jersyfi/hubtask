// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// A minimal SFTP version 3 client, over the SSH transport golang.org/x/crypto/ssh provides.
//
// Hand-written for the reason SigV4 is: an SFTP library is a new third-party dependency, and the
// milestone allowed exactly one new direct dependency - golang.org/x/crypto, whose ssh package is
// what carries the bytes here (milestone decision 3). What is implemented is the eleven packet
// types a backup target needs and nothing else: no symbolic links, no permissions, no rename, no
// version 4, 5 or 6 extensions. Version 3 is what every server in the field speaks, and the parts
// used here have not changed since 2001.
//
// The framing is the whole of the protocol: a 32-bit length, a type byte, a 32-bit request
// identifier, and then fields that are either fixed-width integers or a 32-bit length followed by
// that many bytes. Everything below is that sentence applied eleven times.

// The packet types this client sends and understands.
const (
	fxpInit    = 1
	fxpVersion = 2
	fxpOpen    = 3
	fxpClose   = 4
	fxpRead    = 5
	fxpWrite   = 6
	fxpOpendir = 11
	fxpReaddir = 12
	fxpRemove  = 13
	fxpMkdir   = 14
	fxpStat    = 17
	fxpStatus  = 101
	fxpHandle  = 102
	fxpData    = 103
	fxpName    = 104
	fxpAttrs   = 105
)

// The status codes worth telling apart. Everything else is a failure of the target's, reported as
// one code, because a caller cannot act on the difference between "quota exceeded" and "no space".
const (
	statusOK         = 0
	statusEOF        = 1
	statusNoSuchFile = 2
	statusPermission = 3
)

// The open flags, from the same document.
const (
	openRead     = 0x00000001
	openWrite    = 0x00000002
	openCreate   = 0x00000008
	openTruncate = 0x00000010
)

// The attribute flags. Only two are read: how large a file is, and when it was last written.
const (
	attrSize       = 0x00000001
	attrUIDGID     = 0x00000002
	attrPermission = 0x00000004
	attrACModTime  = 0x00000008
	attrExtended   = 0x80000000
)

// sftpVersion is what this client speaks. Three, and a server answering anything else is refused
// rather than negotiated with: a version this code has not been written against would be read
// with the wrong field widths, which is a silent corruption rather than an error.
const sftpVersion = 3

// maxPacket bounds what will be read into memory from one packet. An SFTP server is somebody
// else's machine, and a length prefix is the classic way to ask a client for a gigabyte (T-17).
const maxPacket = 1 << 20

// chunkSize is how much of a file travels per request. Thirty-two kibibytes is what every server
// in the field accepts; OpenSSH takes more, and a value that only OpenSSH takes is a value that
// fails at somebody's NAS.
const chunkSize = 32 << 10

// sftpError is a status the server sent. It carries the code and never the server's message: an
// SFTP failure message repeats the path, and a path names a tenant and a moment (rule 10).
type sftpError struct{ code uint32 }

func (e sftpError) Error() string { return fmt.Sprintf("the target answered status %d", e.code) }

func (e sftpError) missing() bool { return e.code == statusNoSuchFile }
func (e sftpError) refused() bool { return e.code == statusPermission }

// sftpSession is one SFTP conversation over one SSH channel.
//
// Requests are issued one at a time, under a mutex. That costs throughput on a link with latency -
// a pipelining client keeps several writes in flight - and it buys a client that is a page of code
// with no request-multiplexing state machine to get wrong. A backup runs on a schedule and not on
// somebody's screen; correctness is worth more here than the round trips.
type sftpSession struct {
	mu     sync.Mutex
	stream io.ReadWriteCloser
	nextID uint32
}

// newSFTPSession performs the version handshake.
func newSFTPSession(stream io.ReadWriteCloser) (*sftpSession, error) {
	session := &sftpSession{stream: stream, nextID: 1}

	if err := session.send(fxpInit, encodeUint32(nil, sftpVersion)); err != nil {
		return nil, err
	}
	kind, payload, err := session.receive()
	if err != nil {
		return nil, err
	}
	if kind != fxpVersion || len(payload) < 4 || binary.BigEndian.Uint32(payload) != sftpVersion {
		return nil, errors.New("the target does not speak SFTP version 3")
	}
	return session, nil
}

func (s *sftpSession) Close() error { return s.stream.Close() }

// open opens a file and answers its handle.
func (s *sftpSession) open(path string, flags uint32) ([]byte, error) {
	body := encodeString(nil, path)
	body = encodeUint32(body, flags)
	// No attributes: the file's mode is the server's business, and a client that set one would
	// be deciding a NAS's umask for it.
	body = encodeUint32(body, 0)

	kind, payload, err := s.request(fxpOpen, body)
	if err != nil {
		return nil, err
	}
	if kind != fxpHandle {
		return nil, unexpected(kind)
	}
	handle, _, ok := decodeString(payload)
	if !ok {
		return nil, errors.New("the target answered a handle that is not one")
	}
	return handle, nil
}

func (s *sftpSession) closeHandle(handle []byte) error {
	_, _, err := s.request(fxpClose, encodeBytes(nil, handle))
	return err
}

// readAt reads one chunk. An empty answer with no error is the end of the file.
func (s *sftpSession) readAt(handle []byte, offset uint64, length uint32) ([]byte, error) {
	body := encodeBytes(nil, handle)
	body = encodeUint64(body, offset)
	body = encodeUint32(body, length)

	kind, payload, err := s.request(fxpRead, body)
	if err != nil {
		var status sftpError
		if errors.As(err, &status) && status.code == statusEOF {
			return nil, io.EOF
		}
		return nil, err
	}
	if kind != fxpData {
		return nil, unexpected(kind)
	}
	data, _, ok := decodeString(payload)
	if !ok {
		return nil, errors.New("the target answered data that is not")
	}
	return data, nil
}

func (s *sftpSession) writeAt(handle []byte, offset uint64, data []byte) error {
	body := encodeBytes(nil, handle)
	body = encodeUint64(body, offset)
	body = encodeBytes(body, data)

	_, _, err := s.request(fxpWrite, body)
	return err
}

// fileInfo is what this client reads of a file's attributes: the two facts a listing needs.
type fileInfo struct {
	name     string
	size     int64
	modified time.Time
	isDir    bool
}

func (s *sftpSession) stat(path string) (fileInfo, error) {
	kind, payload, err := s.request(fxpStat, encodeString(nil, path))
	if err != nil {
		return fileInfo{}, err
	}
	if kind != fxpAttrs {
		return fileInfo{}, unexpected(kind)
	}
	info, _, ok := decodeAttrs(payload)
	if !ok {
		return fileInfo{}, errors.New("the target answered attributes that are not")
	}
	return info, nil
}

// readdir lists one directory. Several READDIR requests may be needed; the end is a status of EOF.
func (s *sftpSession) readdir(path string) ([]fileInfo, error) {
	kind, payload, err := s.request(fxpOpendir, encodeString(nil, path))
	if err != nil {
		return nil, err
	}
	if kind != fxpHandle {
		return nil, unexpected(kind)
	}
	handle, _, ok := decodeString(payload)
	if !ok {
		return nil, errors.New("the target answered a handle that is not one")
	}
	defer func() { _ = s.closeHandle(handle) }()

	var found []fileInfo
	for {
		kind, payload, err := s.request(fxpReaddir, encodeBytes(nil, handle))
		if err != nil {
			var status sftpError
			if errors.As(err, &status) && status.code == statusEOF {
				return found, nil
			}
			return nil, err
		}
		if kind != fxpName {
			return nil, unexpected(kind)
		}

		names, ok := decodeNames(payload)
		if !ok {
			return nil, errors.New("the target answered a listing that is not one")
		}
		if len(names) == 0 {
			return found, nil
		}
		found = append(found, names...)
		if len(found) > 100_000 {
			// A directory nobody put a hundred thousand archives in is a server answering in a
			// loop, and following it would never end.
			return nil, errors.New("the target listed more entries than a backup directory holds")
		}
	}
}

func (s *sftpSession) remove(path string) error {
	_, _, err := s.request(fxpRemove, encodeString(nil, path))
	return err
}

func (s *sftpSession) mkdir(path string) error {
	body := encodeString(nil, path)
	body = encodeUint32(body, 0)
	_, _, err := s.request(fxpMkdir, body)
	return err
}

// request sends one packet and reads its answer. A status of OK is success; anything else is the
// server saying no.
func (s *sftpSession) request(kind byte, body []byte) (byte, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextID
	s.nextID++

	if err := s.send(kind, append(encodeUint32(nil, id), body...)); err != nil {
		return 0, nil, err
	}
	answer, payload, err := s.receive()
	if err != nil {
		return 0, nil, err
	}
	if len(payload) < 4 {
		return 0, nil, errors.New("the target answered a packet with no request identifier")
	}
	if binary.BigEndian.Uint32(payload) != id {
		// One request at a time means the answer is this request's or the conversation is lost.
		return 0, nil, errors.New("the target answered a request that was not asked")
	}
	payload = payload[4:]

	if answer == fxpStatus {
		if len(payload) < 4 {
			return 0, nil, errors.New("the target answered a status with no code")
		}
		code := binary.BigEndian.Uint32(payload)
		if code == statusOK {
			return answer, payload, nil
		}
		return 0, nil, sftpError{code: code}
	}
	return answer, payload, nil
}

// send writes one framed packet.
//
// The length is checked against the same bound the reading side applies, rather than trusted
// because this client built the body. Everything that reaches here is bounded by construction -
// a write is one chunkSize, a path is a path - but "bounded by construction" is a property of the
// callers rather than of this function, and the frame header is a uint32: a body that somehow
// exceeded it would not fail, it would wrap, and the target would read the next four gigabytes as
// packets that were never sent.
func (s *sftpSession) send(kind byte, body []byte) error {
	// The length the header announces: the kind byte plus the body. Bound once, into a variable,
	// and every arithmetic below reads that variable - a check on one expression and an allocation
	// from an equivalent one is a bound a reader has to reconstruct, and an analyser cannot.
	length := len(body) + 1
	if length > maxPacket {
		return fmt.Errorf("this client tried to send a packet of %d bytes", length)
	}

	frame := make([]byte, 4+length)
	// No suppression needed any more: the bound above is what gosec was being asked to take
	// on trust before.
	binary.BigEndian.PutUint32(frame, uint32(length))
	frame[4] = kind
	copy(frame[5:], body)

	if _, err := s.stream.Write(frame); err != nil {
		return err
	}
	return nil
}

// receive reads one framed packet, refusing a length this process will not hold.
func (s *sftpSession) receive() (byte, []byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(s.stream, header[:]); err != nil {
		return 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length == 0 || length > maxPacket {
		return 0, nil, fmt.Errorf("the target announced a packet of %d bytes", length)
	}

	packet := make([]byte, length)
	if _, err := io.ReadFull(s.stream, packet); err != nil {
		return 0, nil, err
	}
	return packet[0], packet[1:], nil
}

func unexpected(kind byte) error {
	return fmt.Errorf("the target answered packet type %d", kind)
}

// The wire encoding: fixed-width integers, and strings as a length and that many bytes.

func encodeUint32(to []byte, value uint32) []byte {
	return binary.BigEndian.AppendUint32(to, value)
}

func encodeUint64(to []byte, value uint64) []byte {
	return binary.BigEndian.AppendUint64(to, value)
}

func encodeBytes(to []byte, value []byte) []byte {
	to = binary.BigEndian.AppendUint32(to, uint32(len(value))) //nolint:gosec // G115: bounded by chunkSize
	return append(to, value...)
}

func encodeString(to []byte, value string) []byte {
	return encodeBytes(to, []byte(value))
}

func decodeString(from []byte) ([]byte, []byte, bool) {
	if len(from) < 4 {
		return nil, nil, false
	}
	length := binary.BigEndian.Uint32(from)
	// The comparison is done in the wider of the two: a length prefix is a 32-bit number from
	// somebody else's machine, and a slice bound taken from it without this check is how a
	// parser is asked to read past the end of a packet.
	if int64(length) > int64(len(from)-4) {
		return nil, nil, false
	}
	return from[4 : 4+length], from[4+length:], true
}

// decodeAttrs reads the attribute block, skipping every field this client does not use. The
// skipping is the point: a block whose optional fields were not stepped over leaves the parser
// pointing at the middle of a number.
func decodeAttrs(from []byte) (fileInfo, []byte, bool) {
	if len(from) < 4 {
		return fileInfo{}, nil, false
	}
	flags := binary.BigEndian.Uint32(from)
	from = from[4:]

	var info fileInfo
	if flags&attrSize != 0 {
		if len(from) < 8 {
			return fileInfo{}, nil, false
		}
		size := binary.BigEndian.Uint64(from)
		if size > 1<<62 {
			return fileInfo{}, nil, false
		}
		info.size = int64(size)
		from = from[8:]
	}
	if flags&attrUIDGID != 0 {
		if len(from) < 8 {
			return fileInfo{}, nil, false
		}
		from = from[8:]
	}
	if flags&attrPermission != 0 {
		if len(from) < 4 {
			return fileInfo{}, nil, false
		}
		// The one permission bit that is read: S_IFDIR, which is how version 3 says "directory".
		info.isDir = binary.BigEndian.Uint32(from)&0o170000 == 0o040000
		from = from[4:]
	}
	if flags&attrACModTime != 0 {
		if len(from) < 8 {
			return fileInfo{}, nil, false
		}
		info.modified = time.Unix(int64(binary.BigEndian.Uint32(from[4:])), 0).UTC()
		from = from[8:]
	}
	if flags&attrExtended != 0 {
		if len(from) < 4 {
			return fileInfo{}, nil, false
		}
		count := binary.BigEndian.Uint32(from)
		from = from[4:]
		for range count {
			var ok bool
			if _, from, ok = decodeString(from); !ok {
				return fileInfo{}, nil, false
			}
			if _, from, ok = decodeString(from); !ok {
				return fileInfo{}, nil, false
			}
		}
	}
	return info, from, true
}

// decodeNames reads a READDIR answer: a count, then that many (name, long name, attributes).
func decodeNames(from []byte) ([]fileInfo, bool) {
	if len(from) < 4 {
		return nil, false
	}
	count := binary.BigEndian.Uint32(from)
	from = from[4:]
	if count > 10_000 {
		return nil, false
	}

	found := make([]fileInfo, 0, count)
	for range count {
		name, rest, ok := decodeString(from)
		if !ok {
			return nil, false
		}
		// The long name is the `ls -l` line servers send for humans. It is skipped rather than
		// parsed: it is free text in the server's locale, and the attributes that follow are the
		// machine-readable answer to the same question.
		_, rest, ok = decodeString(rest)
		if !ok {
			return nil, false
		}
		info, rest2, ok := decodeAttrs(rest)
		if !ok {
			return nil, false
		}
		info.name = string(name)
		found = append(found, info)
		from = rest2
	}
	return found, true
}
