// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package realtime serves a live document over a WebSocket (offline-sync.md §13): the framing of
// the editors' shared-editing protocol, one connection's read and write loops, and the in-process
// relay of awareness.
//
// The adapter decides nothing (rule 2): every update it stores and every update it
// hands out goes through the realtime service, which authorises it.
package realtime

import (
	"errors"
)

// A message is a variable-length unsigned integer naming its type, then the type's payload. Sync
// and awareness are the editors' protocol; the two above them are this server's, and an editor
// that does not know them ignores them.
const (
	messageSync      = 0
	messageAwareness = 1
	// messageCompact is a writer's snapshot: the place it covers, then the merged state.
	messageCompact = 100
	// messageCompactionDue asks a writer for a snapshot: the head the snapshot may cover.
	messageCompactionDue = 101
)

// The sync message's own types.
const (
	syncStep1  = 0
	syncStep2  = 1
	syncUpdate = 2
)

var errMalformed = errors.New("realtime: malformed message")

// Message is one decoded message.
type Message struct {
	Type    uint64
	Sync    uint64
	Seq     uint64
	Payload []byte
}

// Decode reads one message. A payload is a length-prefixed byte string in every type this server
// reads; anything after it is ignored, as the protocol allows.
func Decode(raw []byte) (Message, error) {
	kind, rest, err := readUvarint(raw)
	if err != nil {
		return Message{}, err
	}
	message := Message{Type: kind}
	switch kind {
	case messageSync:
		if message.Sync, rest, err = readUvarint(rest); err != nil {
			return Message{}, err
		}
	case messageCompact:
		if message.Seq, rest, err = readUvarint(rest); err != nil {
			return Message{}, err
		}
	case messageAwareness:
	default:
		// A type this server does not speak: answered by ignoring it.
		return message, nil
	}
	message.Payload, _, err = readBytes(rest)
	return message, err
}

// SyncStep1 is the server's state vector. The server keeps none - it never reads an update - so it
// sends the empty one, and a writer answers with everything it holds, its offline edits included.
func SyncStep1() []byte {
	return appendBytes(appendUvarint(appendUvarint(nil, messageSync), syncStep1), []byte{0})
}

// SyncStep2 carries a whole state: the snapshot a connection starts from.
func SyncStep2(state []byte) []byte {
	return appendBytes(appendUvarint(appendUvarint(nil, messageSync), syncStep2), state)
}

// SyncUpdate carries one update of the log.
func SyncUpdate(update []byte) []byte {
	return appendBytes(appendUvarint(appendUvarint(nil, messageSync), syncUpdate), update)
}

// Awareness carries one awareness message as a reader sent it.
func Awareness(payload []byte) []byte {
	return appendBytes(appendUvarint(nil, messageAwareness), payload)
}

// CompactionDue asks a writer for a snapshot up to the head.
func CompactionDue(head int64) []byte {
	return appendUvarint(appendUvarint(nil, messageCompactionDue), uint64(head)) //nolint:gosec // a head is never negative
}

func appendUvarint(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

func appendBytes(out, payload []byte) []byte {
	return append(appendUvarint(out, uint64(len(payload))), payload...)
}

// readUvarint reads the protocol's integer: seven bits a byte, low first. Bounded to ten bytes,
// the most a 64-bit value takes.
func readUvarint(in []byte) (uint64, []byte, error) {
	var value uint64
	for i := 0; i < len(in) && i < 10; i++ {
		b := in[i]
		value |= uint64(b&0x7f) << (7 * uint(i))
		if b < 0x80 {
			return value, in[i+1:], nil
		}
	}
	return 0, nil, errMalformed
}

func readBytes(in []byte) ([]byte, []byte, error) {
	length, rest, err := readUvarint(in)
	if err != nil {
		return nil, nil, err
	}
	if length > uint64(len(rest)) {
		return nil, nil, errMalformed
	}
	return rest[:length], rest[length:], nil
}
