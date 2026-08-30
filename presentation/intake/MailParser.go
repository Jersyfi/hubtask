// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The mail parser (G-11): arbitrary bytes from the internet become the four things a jumble entry
// is made of - a sender, a subject, a text and some attachments.
//
// It is the durable half of the mail intake and it is transport-independent on purpose. What
// arrives in 0.5.0 is a bridge posting the message to a token-protected URL; what AM-1 leaves open
// is IMAP polling, and an IMAP client would produce the same bytes for the same function. That is
// the port cut: a second transport is a second producer, never a second parser.
//
// It is also the one boundary in this system that eats bytes nobody wrote for it, so every rule
// here is a refusal rather than a repair:
//
//   - Nothing is allocated before it is bounded. Every part is copied through a limited reader, so
//     a part claiming to be a gigabyte costs the bound plus one byte and a refusal.
//   - The walk is bounded in breadth and in depth. A mail is a tree, and a tree from a stranger is
//     a tree that can be as deep and as wide as they like (T-17's reasoning, applied to MIME).
//   - No HTML is rendered, parsed or followed. An HTML part is text and stays text.
//   - The sender is data. `From` is a line somebody typed; what authenticates is the token on the
//     URL, and the parser hands the address on with no more standing than the subject has.
//
// A message that defeats all of this is not dropped: ParseMail answers ErrUnparseable and the
// intake stores the raw payload as an entry instead. A jumble exists to catch, and "unparseable"
// is a thing to catch.

// MailLimits bounds what one parse may cost. Zero means the default rather than "no limit": an
// unbounded parse is exactly the thing this file exists to prevent, so forgetting to set a bound
// gives the documented one.
type MailLimits struct {
	// MaxParts is how many MIME parts one message may have, counted across the whole tree.
	MaxParts int
	// MaxDepth is how deeply parts may nest. A multipart/mixed holding a multipart/alternative is
	// depth two, which is what an ordinary mail with a text, an HTML alternative and a file looks
	// like; four leaves room for the unordinary one.
	MaxDepth int
	// MaxAttachments is how many files one mail may carry into the jumble.
	MaxAttachments int
	// MaxAttachmentBytes bounds one attachment, decoded.
	MaxAttachmentBytes int64
	// MaxTextBytes bounds the text this becomes, decoded. The jumble's own bound on a body is the
	// domain's (jumble.MaxBodyBytes); this one is the parser's, so an over-long part is cut here
	// rather than refused three layers later.
	MaxTextBytes int64
}

// The documented defaults. Mail-shaped rather than generous: the numbers are what an ordinary
// message needs with room to spare, and the point of a bound is that the extraordinary one is
// refused rather than served.
const (
	DefaultMaxMailParts          = 100
	DefaultMaxMailDepth          = 4
	DefaultMaxMailAttachments    = 20
	DefaultMaxMailAttachmentSize = 10 << 20
	DefaultMaxMailTextBytes      = 64 << 10
)

func (l MailLimits) withDefaults() MailLimits {
	if l.MaxParts <= 0 {
		l.MaxParts = DefaultMaxMailParts
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = DefaultMaxMailDepth
	}
	if l.MaxAttachments <= 0 {
		l.MaxAttachments = DefaultMaxMailAttachments
	}
	if l.MaxAttachmentBytes <= 0 {
		l.MaxAttachmentBytes = DefaultMaxMailAttachmentSize
	}
	if l.MaxTextBytes <= 0 {
		l.MaxTextBytes = DefaultMaxMailTextBytes
	}
	return l
}

// Mail is one message, parsed into what a jumble entry is made of.
type Mail struct {
	// Sender is the address the message claims to come from, decoded and normalised. Data, never
	// an identity: a From header authenticates nothing (security.md §4), and every consumer of
	// this field treats it as text somebody typed.
	Sender string
	// Subject is the decoded subject line, RFC 2047 words included.
	Subject string
	// Text is what a person reads: the plain part where there is one, and the HTML part's source
	// as text where there is not. Never rendered markup - an HTML mail read here is somebody
	// reading tags, which is the honest thing for an inbox that must not run them.
	Text string
	// Attachments are the files the mail carried, decoded and bounded. The HTML alternative of a
	// mail that also had a plain part is one of them, carried as text: keeping it is what "stored
	// alongside" means, and keeping it out of Text is what stops a second copy of the same message
	// from being the message.
	Attachments []MailAttachment
}

// MailAttachment is one file out of a message.
type MailAttachment struct {
	FileName    string
	ContentType string
	Content     []byte
}

// HTMLAlternativeFileName is what the kept HTML alternative is called. Stable, because a person
// looking at an entry with two attachments has to be able to tell which one is the mail.
const HTMLAlternativeFileName = "message.html"

// The refusals. Codes rather than sentences (rule 8), and one per bound, because "too big" and
// "too many" send whoever is looking to two different places.
const (
	CodeMailUnparseable      = "mail.unparseable"
	CodeMailTooManyParts     = "mail.too_many_parts"
	CodeMailTooDeep          = "mail.too_deeply_nested"
	CodeMailTooManyAttached  = "mail.too_many_attachments"
	CodeMailAttachmentTooBig = "mail.attachment_too_large"
)

// ErrUnparseable is a payload that is not a message this parser can walk. The intake answers it by
// storing the raw bytes rather than by refusing the delivery - see MailIntake.
var ErrUnparseable = shared.ErrValidation.WithDetail(CodeMailUnparseable)

// ParseMail walks one message.
//
// The bounds are refusals with their own codes, and they are not the same answer as ErrUnparseable:
// a mail with two hundred parts is a mail this installation declines to take, and one whose shape
// the parser cannot follow is a mail it takes as bytes. Telling an operator which of the two
// happened is the difference between "raise the bound" and "look at the entry".
func ParseMail(raw []byte, limits MailLimits) (Mail, error) {
	limits = limits.withDefaults()

	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Mail{}, ErrUnparseable
	}

	parsed := Mail{
		Sender:  senderOf(message.Header),
		Subject: decodeHeader(message.Header.Get("Subject")),
	}

	walk := &mailWalk{limits: limits}
	if err := walk.part(partHeaders{
		contentType: message.Header.Get("Content-Type"),
		disposition: message.Header.Get("Content-Disposition"),
		encoding:    message.Header.Get("Content-Transfer-Encoding"),
	}, message.Body, 0); err != nil {
		return Mail{}, err
	}

	parsed.Text = walk.text()
	parsed.Attachments = walk.keptHTML()
	return parsed, nil
}

// mailWalk carries what one walk has seen. A struct rather than parameters threaded through the
// recursion, because the bounds are about the whole message and not about one branch: two hundred
// parts spread over four branches is still two hundred parts.
type mailWalk struct {
	limits      MailLimits
	parts       int
	plain       string
	html        string
	attachments []MailAttachment
}

// text is the body a person reads: the plain part, and the HTML source where there was none.
func (w *mailWalk) text() string {
	if w.plain != "" {
		return w.plain
	}
	return w.html
}

// keptHTML is the attachments plus the HTML alternative of a mail that also had a plain part.
//
// Its own step rather than a branch inside the walk, so that the decision is visible where it is
// read: an HTML part beside a plain one says the same thing twice, so it does not belong in the
// body a person reads - and it is not thrown away either, because "the same thing" is the sender's
// claim and not something this parser can check. It is carried as `text/plain`, because the bytes
// are HTML *source*: every consumer that meets them - the media route with its `attachment`
// disposition and its `nosniff`, a person reading the entry - meets text, and nothing in this
// system ever renders them.
//
// A mail that filled the attachment bound keeps its files and loses the alternative. The files are
// what somebody sent; the alternative is a second copy of what they wrote.
func (w *mailWalk) keptHTML() []MailAttachment {
	if w.html == "" || w.plain == "" || len(w.attachments) >= w.limits.MaxAttachments {
		return w.attachments
	}
	return append(w.attachments, MailAttachment{
		FileName: HTMLAlternativeFileName, ContentType: "text/plain", Content: []byte(w.html),
	})
}

// partHeaders are the three headers a walk reads off a node. A struct rather than three
// parameters, because a fourth would be a fourth place to forget one - and because the disposition
// is the header that decides whether a part is text or a file, which is easy to leave out.
type partHeaders struct {
	contentType string
	disposition string
	encoding    string
}

// part walks one node of the tree: a leaf becomes text or an attachment, a multipart becomes its
// children.
func (w *mailWalk) part(headers partHeaders, body io.Reader, depth int) error {
	if depth > w.limits.MaxDepth {
		return refusal(CodeMailTooDeep)
	}
	w.parts++
	if w.parts > w.limits.MaxParts {
		return refusal(CodeMailTooManyParts)
	}

	mediaType, params := mediaTypeOf(headers.contentType)
	if strings.HasPrefix(mediaType, "multipart/") {
		return w.multipart(body, params["boundary"], depth)
	}
	return w.leaf(mediaType, params, headers, body)
}

// multipart walks the children of one multipart node.
//
// A part the reader cannot make sense of ends the walk of *this* node rather than the message: a
// truncated last part is the commonest damage a bridge does, and what has been read by then is
// what the entry gets. That is the same decision ErrUnparseable makes one level up - keep what
// arrived - taken where there is something to keep.
func (w *mailWalk) multipart(body io.Reader, boundary string, depth int) error {
	if boundary == "" {
		return refusal(CodeMailUnparseable)
	}

	reader := multipart.NewReader(body, boundary)
	for {
		part, err := reader.NextRawPart()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return nil
		}

		err = w.part(partHeaders{
			contentType: part.Header.Get("Content-Type"),
			disposition: part.Header.Get("Content-Disposition"),
			encoding:    part.Header.Get("Content-Transfer-Encoding"),
		}, part, depth+1)
		_ = part.Close()
		if err != nil {
			return err
		}
	}
}

// leaf turns one non-multipart part into text or into an attachment.
//
// What separates the two is a name and a disposition, in that order: a text part with a file name
// is a file that happens to be text - a log somebody attached - and a part disposed as an
// attachment is one whatever it claims to be.
func (w *mailWalk) leaf(
	mediaType string, params map[string]string, headers partHeaders, body io.Reader,
) error {
	disposition, dispositionParams := dispositionOf(headers.disposition)
	name := fileNameOf(dispositionParams, params)
	isText := mediaType == "text/plain" || mediaType == "text/html" || mediaType == ""

	if isText && name == "" && disposition != "attachment" {
		// The bound is applied to the read rather than after it: an unbounded copy of a part
		// claiming to be a gigabyte is the allocation this parser exists not to make.
		content, err := decode(body, headers.encoding, w.limits.MaxTextBytes)
		if err != nil {
			return nil
		}
		// Bounded again after normalising, because normalising can grow it: a part read as
		// ISO-8859-1 is one UTF-8 rune per byte, and a byte over 0x7f is two bytes of text. The
		// read bound is what stops the allocation; this one is what makes the answer bounded.
		text := cut(normaliseText(content, params["charset"]), w.limits.MaxTextBytes)
		switch {
		case mediaType == "text/html":
			if w.html == "" {
				w.html = text
			}
		case w.plain == "":
			w.plain = text
		}
		return nil
	}

	content, err := decode(body, headers.encoding, w.limits.MaxAttachmentBytes)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return refusal(CodeMailAttachmentTooBig)
		}
		// Undecodable bytes are not a reason to lose the rest of the mail: the part is skipped and
		// what is left of the message still becomes an entry.
		return nil
	}
	if len(content) == 0 {
		return nil
	}
	if len(w.attachments) >= w.limits.MaxAttachments {
		return refusal(CodeMailTooManyAttached)
	}
	if name == "" {
		name = unnamedAttachmentName(mediaType)
	}

	w.attachments = append(w.attachments, MailAttachment{
		FileName: name, ContentType: mediaType, Content: content,
	})
	return nil
}

// errTooLarge separates the one decode failure that is a bound from the ones that are damage.
var errTooLarge = errors.New("part exceeds its bound")

// decode reads one part under its transfer encoding, bounded.
//
// The limit is checked by reading one byte more than it allows: a part that fills the bound
// exactly is fine, and one that has anything after it is refused - without the whole of it ever
// being in memory.
func decode(body io.Reader, encoding string, limit int64) ([]byte, error) {
	reader := body
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, newlineStripper{body})
	case "quoted-printable":
		reader = quotedprintable.NewReader(body)
	}

	content, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errTooLarge
	}
	return content, nil
}

// newlineStripper drops the line breaks base64 in mail is wrapped at. The standard decoder refuses
// them, and a mail without them would be the unusual one.
type newlineStripper struct{ inner io.Reader }

func (s newlineStripper) Read(into []byte) (int, error) {
	read, err := s.inner.Read(into)
	if read > 0 {
		kept := into[:0]
		for _, b := range into[:read] {
			if b != '\r' && b != '\n' {
				kept = append(kept, b)
			}
		}
		read = len(kept)
	}
	return read, err
}

// senderOf is the address the message claims, decoded.
//
// The raw header where it does not parse as an address, and that is deliberate: what this field is
// for is a person judging where something came from, and "whatever they wrote" answers that better
// than an empty string does. It authenticates nothing either way.
func senderOf(header mail.Header) string {
	raw := header.Get("From")
	if raw == "" {
		return ""
	}
	if address, err := mail.ParseAddress(raw); err == nil {
		return address.Address
	}
	return strings.TrimSpace(decodeHeader(raw))
}

// decodeHeader turns RFC 2047 encoded words into text, and leaves what it cannot decode alone.
func decodeHeader(value string) string {
	decoder := mime.WordDecoder{CharsetReader: charsetReader}
	decoded, err := decoder.DecodeHeader(value)
	if err != nil {
		return sanitiseText(value)
	}
	return sanitiseText(decoded)
}

// mediaTypeOf reads a Content-Type, and answers text/plain for one it cannot: an untyped part in
// mail is text, which is what RFC 2045 §5.2 says a missing header means.
func mediaTypeOf(value string) (string, map[string]string) {
	if strings.TrimSpace(value) == "" {
		return "text/plain", map[string]string{}
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return "text/plain", map[string]string{}
	}
	if params == nil {
		params = map[string]string{}
	}
	return strings.ToLower(mediaType), params
}

// dispositionOf reads a Content-Disposition, and answers nothing for a header it cannot: an
// unreadable disposition is not a reason to lose a part.
func dispositionOf(value string) (string, map[string]string) {
	if strings.TrimSpace(value) == "" {
		return "", map[string]string{}
	}
	disposition, params, err := mime.ParseMediaType(value)
	if err != nil {
		return "", map[string]string{}
	}
	if params == nil {
		params = map[string]string{}
	}
	return strings.ToLower(disposition), params
}

// fileNameOf is the name a part carries, from either of the two places one can be written - the
// disposition first, because that is where RFC 2183 puts it - and stripped of every path it might
// claim: a name is a label here, never a location (T-11 - a storage key is never user text, and
// this one never becomes one).
func fileNameOf(disposition, contentType map[string]string) string {
	name := disposition["filename"]
	if name == "" {
		name = contentType["name"]
	}
	name = decodeHeader(name)
	if index := strings.LastIndexAny(name, `/\`); index >= 0 {
		name = name[index+1:]
	}
	name = strings.TrimSpace(sanitiseText(name))
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// unnamedAttachmentName is what a file with no name is called. It has to have one: the media
// object carries a file name, and a person looking at an entry has to see something.
func unnamedAttachmentName(mediaType string) string {
	extension := ""
	if extensions, err := mime.ExtensionsByType(mediaType); err == nil && len(extensions) > 0 {
		extension = extensions[0]
	}
	return "attachment" + extension
}

// normaliseText turns a decoded part into UTF-8 text.
//
// Two charsets are handled outright and everything else is treated as ISO-8859-1, which is what a
// byte-for-rune reading is. That is a decision rather than a gap: the alternative is a character
// set library, which is a dependency, and a dependency is not taken in passing (CLAUDE.md). A
// mis-decoded accent in an inbox entry is a legible message with a wrong character in it; a
// dependency taken without a decision is a supply chain nobody chose.
func normaliseText(content []byte, charset string) string {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		if utf8.Valid(content) {
			return sanitiseText(string(content))
		}
	}
	return sanitiseText(latin1(content))
}

// cut shortens text to a byte bound, on a rune boundary: a string cut mid-rune is a string with an
// invalid byte in it, which is the one thing sanitiseText was for.
func cut(value string, limit int64) string {
	if int64(len(value)) <= limit {
		return value
	}
	cutAt := int(limit)
	for cutAt > 0 && !utf8.RuneStart(value[cutAt]) {
		cutAt--
	}
	return value[:cutAt]
}

// latin1 reads bytes as ISO-8859-1, where every byte is the rune of the same number.
func latin1(content []byte) string {
	var out strings.Builder
	out.Grow(len(content))
	for _, b := range content {
		out.WriteRune(rune(b))
	}
	return out.String()
}

// sanitiseText drops what has no business in stored text: the C0 controls except tab, carriage
// return and newline, and every byte that is not valid UTF-8.
//
// Not an escape and not a rejection. The text is stored and read back as text, never rendered as
// markup and never interpreted as instructions (ai-first.md), so what is left to do is make sure
// what is stored is text at all.
func sanitiseText(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for _, r := range value {
		switch {
		case r == utf8.RuneError:
			continue
		case r == '\t' || r == '\n' || r == '\r':
			out.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			continue
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// charsetReader lets the header decoder read the charsets normaliseText knows, and refuse the
// rest rather than guess in a place where a guess is a header.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "iso8859-1", "windows-1252":
		content, err := io.ReadAll(io.LimitReader(input, DefaultMaxMailTextBytes))
		if err != nil {
			return nil, err
		}
		return strings.NewReader(latin1(content)), nil
	}
	return nil, errors.New("unsupported charset")
}

// refusal is one of the parser's bounds, as an error a caller can put a message code on.
func refusal(code string) error {
	return shared.ErrValidation.WithDetail(code).
		WithFields(shared.FieldError{Path: "/body", Code: code})
}
