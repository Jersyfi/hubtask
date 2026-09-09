// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package stream

import (
	"fmt"
	"net/http"
	"strings"
)

// Writer frames server-sent events onto a response, and flushes each one.
//
// The flush is the whole reason this type exists rather than a few Fprintf calls. net/http buffers
// a response, so an event that was written and not flushed is an event the client gets when the
// next four kilobytes arrive - which for a quiet workspace is minutes later, and which makes a
// stream indistinguishable from a broken one.
//
// Here rather than in either adapter because both write the same wire format: `GET /stream` carries
// change records to a browser and `GET /mcp` carries JSON-RPC notifications to an agent, and the
// framing - the field names, the blank line, the flush, the newline stripping - is the same
// protocol in both. What differs is what goes in the `data`, which is each adapter's own.
type Writer struct {
	w          http.ResponseWriter
	controller *http.ResponseController
}

// NewWriter takes the response over. It writes no header: the caller decides the status and the
// content type, because a stream that has begun cannot become a problem document.
func NewWriter(w http.ResponseWriter) *Writer {
	return &Writer{w: w, controller: http.NewResponseController(w)}
}

// Headers sets what every server-sent event stream needs, and is where the reasons live.
func Headers(header http.Header) {
	header.Set("Content-Type", "text/event-stream")
	// No store and no transform: an intermediary that cached this would serve one client's
	// records to another, and one that buffered it would hold every event until the connection
	// ended - which is the whole point of the connection.
	header.Set("Cache-Control", "no-store")
	header.Set("Connection", "keep-alive")
	// nginx buffers proxied responses by default and this is the header that turns it off. Sent
	// unconditionally: it means nothing to anything else.
	header.Set("X-Accel-Buffering", "no")
}

// Event writes one frame. An empty id or name leaves that field out, which is legal and is what a
// notification with no identity wants.
func (s *Writer) Event(id, name, data string) error {
	var out strings.Builder
	if id != "" {
		out.WriteString("id: ")
		out.WriteString(FieldValue(id))
		out.WriteString("\n")
	}
	if name != "" {
		out.WriteString("event: ")
		out.WriteString(FieldValue(name))
		out.WriteString("\n")
	}
	out.WriteString("data: ")
	// One line. Both callers send JSON, which has no newline outside a string; a payload that
	// spanned lines would have to be split across several `data:` lines to stay legal.
	out.WriteString(FieldValue(data))
	out.WriteString("\n\n")

	return s.Write(out.String())
}

// Comment writes a `:` line: the heartbeat, and the goodbye.
//
// A comment rather than an event, because a client must be able to ignore it without knowing what
// it is - the specification says a line beginning with a colon is discarded, which is exactly the
// contract a keep-alive needs.
func (s *Writer) Comment(text string) error {
	return s.Write(": " + FieldValue(text) + "\n\n")
}

// Retry tells a reconnecting client how long to wait, in milliseconds.
func (s *Writer) Retry(milliseconds int) error {
	return s.Write(fmt.Sprintf("retry: %d\n\n", milliseconds))
}

// Write puts one framed string on the wire and flushes it.
func (s *Writer) Write(frame string) error {
	if _, err := s.w.Write([]byte(frame)); err != nil {
		return err
	}
	// Errors are ignored on purpose: a writer that cannot flush is one that buffers, which is
	// slower rather than wrong, and the write above is what reports a connection that has gone.
	_ = s.controller.Flush()
	return nil
}

// FieldValue keeps a value on one line.
//
// A newline in an event's name, an identifier or a comment's text would end the field and let
// whatever follows be read as the next one - the same shape as a header injection, in a protocol
// whose separator is also a line break.
func FieldValue(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}
