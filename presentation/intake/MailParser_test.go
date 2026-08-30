// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/intake"
)

// The mail parser (G-11), the one boundary in this system that eats bytes nobody wrote for it.
// Every test here is either "an ordinary mail arrives whole" or "a crafted one costs a refusal".

// The ordinary mail: a text, an HTML alternative and two files. One entry's worth of everything.
func TestAMailWithTextHTMLAndTwoAttachmentsIsWalkedWhole(t *testing.T) {
	raw := mixedMail(t)

	parsed, err := intake.ParseMail(raw, intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	if parsed.Sender != "orders@example.org" {
		t.Errorf("the sender is %q", parsed.Sender)
	}
	if parsed.Subject != "Order #42 — please call back" {
		t.Errorf("the subject is %q", parsed.Subject)
	}
	if got := strings.TrimSpace(parsed.Text); got != "The customer asked for a call back." {
		t.Errorf("the text is %q, want the plain part", got)
	}
	// The two files, and the HTML alternative kept beside them as text.
	if len(parsed.Attachments) != 3 {
		t.Fatalf("%d attachments, want the two files and the kept alternative: %+v",
			len(parsed.Attachments), names(parsed.Attachments))
	}
	first, second := parsed.Attachments[0], parsed.Attachments[1]
	if first.FileName != "invoice.pdf" || first.ContentType != "application/pdf" {
		t.Errorf("the first attachment is %q of %q", first.FileName, first.ContentType)
	}
	if string(first.Content) != "%PDF-1.4 invoice" {
		t.Errorf("the first attachment carries %q", first.Content)
	}
	if second.FileName != "photo.png" || string(second.Content) != "\x89PNG photo" {
		t.Errorf("the second attachment is %q carrying %q", second.FileName, second.Content)
	}
}

// The HTML alternative is kept, and kept as text: the bytes are HTML source, and nothing in this
// system ever renders them. It travels as text/plain so that every consumer that meets it - the
// media route, a person reading the entry - meets text.
func TestTheHTMLAlternativeIsKeptAsText(t *testing.T) {
	parsed, err := intake.ParseMail(mixedMail(t), intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}

	kept := parsed.Attachments[len(parsed.Attachments)-1]
	if kept.FileName != intake.HTMLAlternativeFileName {
		t.Fatalf("the last attachment is %q, want the kept alternative", kept.FileName)
	}
	if kept.ContentType != "text/plain" {
		t.Errorf("the alternative is stored as %q, want text/plain", kept.ContentType)
	}
	if !strings.Contains(string(kept.Content), "<p>") {
		t.Errorf("the alternative lost its source: %q", kept.Content)
	}
	if strings.Contains(parsed.Text, "<p>") {
		t.Error("the HTML reached the body a person reads")
	}
}

// A mail with no plain part is not a mail without a body: the HTML source is the text, unrendered.
func TestAnHTMLOnlyMailBecomesItsSourceAsText(t *testing.T) {
	raw := []byte("From: a@example.org\r\nSubject: Hello\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n<p>Hello &amp; welcome</p>\r\n")

	parsed, err := intake.ParseMail(raw, intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if !strings.Contains(parsed.Text, "<p>Hello &amp; welcome</p>") {
		t.Errorf("the text is %q", parsed.Text)
	}
	if len(parsed.Attachments) != 0 {
		t.Errorf("an HTML-only mail kept %d attachments", len(parsed.Attachments))
	}
}

// The two transfer encodings mail actually uses, and the two charsets a parser without a character
// set library can honestly claim. A byte-for-rune reading is what everything else gets.
func TestEncodingsAreNormalised(t *testing.T) {
	for _, c := range []struct {
		name    string
		headers string
		body    string
		want    string
	}{
		{
			name:    "quoted-printable",
			headers: "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n",
			body:    "Caf=C3=A9 at 8=3A00\r\n",
			want:    "Café at 8:00",
		},
		{
			name:    "base64, wrapped as mail wraps it",
			headers: "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n",
			body:    wrapped(base64.StdEncoding.EncodeToString([]byte("A rather long line of text"))),
			want:    "A rather long line of text",
		},
		{
			name:    "latin-1, which is every charset this parser does not know",
			headers: "Content-Type: text/plain; charset=iso-8859-1\r\n",
			body:    "Caf\xe9\r\n",
			want:    "Café",
		},
		{
			name:    "no content type at all, which RFC 2045 says is text",
			headers: "",
			body:    "Just text\r\n",
			want:    "Just text",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte("From: a@example.org\r\nSubject: s\r\n" + c.headers + "\r\n" + c.body)

			parsed, err := intake.ParseMail(raw, intake.MailLimits{})
			if err != nil {
				t.Fatalf("parsing: %v", err)
			}
			if got := strings.TrimSpace(parsed.Text); got != c.want {
				t.Errorf("the text is %q, want %q", got, c.want)
			}
		})
	}
}

// An encoded-word subject is decoded, and a From that is not an address is kept as it was written:
// what the field is for is a person judging where something came from, and "whatever they wrote"
// answers that better than an empty string. It authenticates nothing either way.
func TestTheHeadersAreDecodedAndTheSenderIsNeverTrusted(t *testing.T) {
	raw := []byte("From: Not an address at all\r\n" +
		"Subject: =?utf-8?q?Caf=C3=A9_meeting?=\r\n\r\nBody\r\n")

	parsed, err := intake.ParseMail(raw, intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if parsed.Subject != "Café meeting" {
		t.Errorf("the subject is %q", parsed.Subject)
	}
	if parsed.Sender != "Not an address at all" {
		t.Errorf("the sender is %q, want what they wrote", parsed.Sender)
	}
}

// A file name is a label, never a location: a part claiming `../../etc/passwd` yields a name, and
// nothing that could ever be a path (T-11 - a storage key is never user text).
func TestAnAttachmentNameCannotClaimAPath(t *testing.T) {
	raw := multipartMail(t, "mixed", `--b
Content-Type: application/octet-stream
Content-Disposition: attachment; filename="../../etc/passwd"

secrets
--b--
`)

	parsed, err := intake.ParseMail(raw, intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(parsed.Attachments) != 1 {
		t.Fatalf("%d attachments", len(parsed.Attachments))
	}
	if got := parsed.Attachments[0].FileName; got != "passwd" {
		t.Errorf("the file name is %q, want the label without its path", got)
	}
}

// The bounds, each with its own code: "too big" and "too many" send whoever is looking to two
// different places, and neither of them is the answer for a payload nobody can parse.
func TestTheBoundsRefuseWithTheirOwnCodes(t *testing.T) {
	t.Run("an attachment over its bound", func(t *testing.T) {
		raw := multipartMail(t, "mixed", "--b\r\n"+
			"Content-Type: application/octet-stream\r\n"+
			"Content-Disposition: attachment; filename=\"big.bin\"\r\n\r\n"+
			strings.Repeat("x", 4096)+"\r\n--b--\r\n")

		_, err := intake.ParseMail(raw, intake.MailLimits{MaxAttachmentBytes: 1024})
		expectCode(t, err, intake.CodeMailAttachmentTooBig)
	})

	t.Run("a MIME bomb of two hundred parts", func(t *testing.T) {
		var body strings.Builder
		for range 200 {
			fmt.Fprintf(&body, "--b\r\nContent-Type: text/plain\r\n\r\npart\r\n")
		}
		body.WriteString("--b--\r\n")

		_, err := intake.ParseMail(multipartMail(t, "mixed", body.String()), intake.MailLimits{})
		expectCode(t, err, intake.CodeMailTooManyParts)
	})

	t.Run("more attachments than an entry may carry", func(t *testing.T) {
		var body strings.Builder
		for i := range 25 {
			fmt.Fprintf(&body, "--b\r\nContent-Type: application/octet-stream\r\n"+
				"Content-Disposition: attachment; filename=\"f%d.bin\"\r\n\r\nbytes\r\n", i)
		}
		body.WriteString("--b--\r\n")

		_, err := intake.ParseMail(multipartMail(t, "mixed", body.String()),
			intake.MailLimits{MaxAttachments: 20})
		expectCode(t, err, intake.CodeMailTooManyAttached)
	})

	t.Run("a tree deeper than the walk goes", func(t *testing.T) {
		// Built from the inside out: five multiparts wrapping one text, each one the single part
		// of the next. A mail is a tree, and a tree from a stranger is as deep as they like.
		body, contentType := "deep text\r\n", "text/plain"
		for depth := 5; depth >= 1; depth-- {
			boundary := fmt.Sprintf("b%d", depth)
			body = fmt.Sprintf("--%s\r\nContent-Type: %s\r\n\r\n%s--%s--\r\n",
				boundary, contentType, body, boundary)
			contentType = fmt.Sprintf("multipart/mixed; boundary=%q", boundary)
		}
		raw := []byte("From: a@example.org\r\nSubject: s\r\nContent-Type: " +
			contentType + "\r\n\r\n" + body)

		_, err := intake.ParseMail(raw, intake.MailLimits{MaxDepth: 3})
		expectCode(t, err, intake.CodeMailTooDeep)
	})
}

// A payload that is not a message at all. The intake stores the raw bytes rather than losing the
// delivery - a jumble exists to catch, and "unparseable" is a thing to catch - so this refusal has
// to be tellable apart from the bounds above.
func TestAPayloadThatIsNoMessageIsUnparseableRatherThanRefused(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(""),
		[]byte("\x00\x01\x02 not a mail at all"),
		[]byte("Subject without a body or a blank line"),
	} {
		_, err := intake.ParseMail(raw, intake.MailLimits{})
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("parsing %q answered %v", raw, err)
		}
		expectCode(t, err, intake.CodeMailUnparseable)
	}
}

// A truncated last part is the commonest damage a bridge does. What arrived before it is what the
// entry gets: the walk of that node ends, and the message is still a message.
func TestATruncatedPartKeepsWhatArrivedBeforeIt(t *testing.T) {
	raw := multipartMail(t, "mixed", "--b\r\nContent-Type: text/plain\r\n\r\nThe first part\r\n"+
		"--b\r\nContent-Type: application/octet-stream\r\n\r\ntrunc")

	parsed, err := intake.ParseMail(raw, intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if !strings.Contains(parsed.Text, "The first part") {
		t.Errorf("the text is %q, want what arrived before the damage", parsed.Text)
	}
}

// Control characters and invalid UTF-8 do not reach stored text. Not an escape and not a
// rejection: the text is stored and read back as text, so what is left to do is make sure what is
// stored is text at all.
func TestStoredTextIsText(t *testing.T) {
	raw := []byte("From: a@example.org\r\nSubject: s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" +
		"before\x00\x07after\r\n")

	parsed, err := intake.ParseMail(raw, intake.MailLimits{})
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if strings.ContainsAny(parsed.Text, "\x00\x07") {
		t.Errorf("a control character survived: %q", parsed.Text)
	}
	if !strings.Contains(parsed.Text, "beforeafter") {
		t.Errorf("the text is %q", parsed.Text)
	}
}

// The fuzz target: this is the one boundary that eats arbitrary bytes, so the property under test
// is the only one that can be stated about arbitrary bytes - it answers, it does not panic, and
// whatever it answers is bounded and is text.
func FuzzParseMail(f *testing.F) {
	f.Add(mixedMail(f))
	f.Add([]byte("From: a@example.org\r\nSubject: s\r\n\r\nplain\r\n"))
	f.Add([]byte("Content-Type: multipart/mixed; boundary=\"b\"\r\n\r\n--b\r\n\r\nx\r\n--b--\r\n"))
	f.Add([]byte("Content-Type: text/plain; charset=iso-8859-1\r\n\r\n\xe9\xff\x00\r\n"))
	f.Add([]byte("Content-Transfer-Encoding: base64\r\n\r\n!!!!not base64!!!!\r\n"))
	f.Add([]byte(""))
	f.Add([]byte("\x00"))

	limits := intake.MailLimits{
		MaxParts: 20, MaxDepth: 3, MaxAttachments: 4,
		MaxAttachmentBytes: 4096, MaxTextBytes: 4096,
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		parsed, err := intake.ParseMail(raw, limits)
		if err != nil {
			return
		}

		if int64(len(parsed.Text)) > limits.MaxTextBytes {
			t.Fatalf("the text is %d bytes, over the bound", len(parsed.Text))
		}
		if len(parsed.Attachments) > limits.MaxAttachments {
			t.Fatalf("%d attachments, over the bound", len(parsed.Attachments))
		}
		for _, attachment := range parsed.Attachments {
			if int64(len(attachment.Content)) > limits.MaxAttachmentBytes {
				t.Fatalf("an attachment is %d bytes, over the bound", len(attachment.Content))
			}
			if attachment.FileName == "" {
				t.Fatal("an attachment has no name, and a media object needs one")
			}
			if strings.ContainsAny(attachment.FileName, `/\`) {
				t.Fatalf("a file name claims a path: %q", attachment.FileName)
			}
		}
		for _, field := range []string{parsed.Text, parsed.Subject, parsed.Sender} {
			if strings.ContainsRune(field, 0) {
				t.Fatalf("a NUL reached stored text: %q", field)
			}
		}
	})
}

// mixedMail is the ordinary mail: a text, an HTML alternative and two files.
func mixedMail(t testing.TB) []byte {
	t.Helper()
	return []byte("From: Orders <orders@example.org>\r\n" +
		"Subject: =?utf-8?q?Order_=2342_=E2=80=94_please_call_back?=\r\n" +
		"Content-Type: multipart/mixed; boundary=\"outer\"\r\n\r\n" +
		"--outer\r\n" +
		"Content-Type: multipart/alternative; boundary=\"inner\"\r\n\r\n" +
		"--inner\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" +
		"The customer asked for a call back.\r\n" +
		"--inner\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		"<p>The customer asked for a call back.</p>\r\n" +
		"--inner--\r\n" +
		"--outer\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"invoice.pdf\"\r\n\r\n" +
		"%PDF-1.4 invoice\r\n" +
		"--outer\r\n" +
		"Content-Type: image/png\r\n" +
		"Content-Disposition: attachment; filename=\"photo.png\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString([]byte("\x89PNG photo")) + "\r\n" +
		"--outer--\r\n")
}

// multipartMail wraps a prepared body in the headers that make it one.
func multipartMail(t testing.TB, subtype, body string) []byte {
	t.Helper()
	// The body is written with whichever line ending the test found convenient; a message has
	// exactly one, and a mixture of the two is a message no reader agrees about.
	normalised := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	return []byte("From: a@example.org\r\nSubject: s\r\n" +
		"Content-Type: multipart/" + subtype + "; boundary=\"b\"\r\n\r\n" + normalised)
}

// wrapped breaks a base64 string the way a mail transfer agent does.
func wrapped(encoded string) string {
	var out strings.Builder
	for len(encoded) > 20 {
		out.WriteString(encoded[:20] + "\r\n")
		encoded = encoded[20:]
	}
	out.WriteString(encoded + "\r\n")
	return out.String()
}

func names(attachments []intake.MailAttachment) []string {
	out := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, attachment.FileName)
	}
	return out
}

func expectCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("the parse succeeded, want %s", want)
	}
	if got := shared.AsError(err).DetailCode; got != want {
		t.Errorf("the refusal is %q, want %q", got, want)
	}
}
