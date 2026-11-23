// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	usecasemedia "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

var (
	cardPicture = shared.MustParseID("0192f000-0000-7000-8000-000000000601")
	cardTenant  = shared.MustParseID("0192f000-0000-7000-8000-00000000060a")
)

// cardPictures answers the one picture the card at one host draws, and records what it was asked.
type cardPictures struct {
	host  string
	asked []string
}

func (c *cardPictures) Picture(_ context.Context, host string, id shared.ID) (shared.ID, error) {
	c.asked = append(c.asked, host)
	if host != c.host || id != cardPicture {
		return "", shared.ErrNotFound.WithDetail("media.not_found")
	}
	return cardTenant, nil
}

func pictureDoor(reader *cardPictures, content *mediaContent) *RestController {
	controller := NewRestController()
	controller.Media = &MediaDoor{Content: content, SignIn: reader}
	return controller
}

func getPicture(t *testing.T, controller *RestController, host string, id shared.ID) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/auth/workspace-pictures/"+id.String(), nil)
	request.Host = host
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// UC-INS-22 check 8: the picture the card at the workspace's address draws is streamed as the image
// it was stored as, read in the workspace the application answered, never sniffed into markup and
// kept as long as its address stands.
func TestTheCardsPictureIsStreamedAsTheImageItWasStoredAs(t *testing.T) {
	reader := &cardPictures{host: "choir.example.org"}
	content := &mediaContent{served: usecasemedia.Served{Content: storage.Object{
		Content: io.NopCloser(strings.NewReader("PNG")), Size: 3, ContentType: "image/png",
	}}}
	answer := getPicture(t, pictureDoor(reader, content), "choir.example.org", cardPicture)

	if answer.Code != http.StatusOK || answer.Body.String() != "PNG" {
		t.Fatalf("answered %d %q", answer.Code, answer.Body)
	}
	if content.grant != (usecasemedia.Grant{TenantID: cardTenant, MediaID: cardPicture}) {
		t.Errorf("read as %+v, want the workspace the application answered", content.grant)
	}
	for name, want := range map[string]string{
		"Content-Type":            "image/png",
		"Content-Length":          "3",
		"Content-Disposition":     "inline",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox; default-src 'none'",
		"Cache-Control":           "public, max-age=31536000, immutable",
	} {
		if got := answer.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// UC-INS-22 check 8: what the application refuses - another workspace's picture, one replaced, an
// address that names no workspace - is a 404 and nothing is read; a picture whose bytes are gone is
// the same 404, with no identifier in it.
func TestEveryRefusedPictureIsTheSame404(t *testing.T) {
	reader := &cardPictures{host: "choir.example.org"}
	content := &mediaContent{}
	door := pictureDoor(reader, content)

	other := shared.MustParseID("0192f000-0000-7000-8000-000000000602")
	for _, asked := range []struct {
		host string
		id   shared.ID
	}{{"choir.example.org", other}, {"other.example.org", cardPicture}} {
		answer := getPicture(t, door, asked.host, asked.id)
		if answer.Code != http.StatusNotFound {
			t.Errorf("%s at %s answered %d, want 404", asked.id, asked.host, answer.Code)
		}
	}
	if content.grant != (usecasemedia.Grant{}) {
		t.Errorf("a refused picture was read: %+v", content.grant)
	}

	content.err = shared.ErrNotFound.WithDetail("media.not_found").
		WithParams(map[string]string{"media_id": cardPicture.String()})
	gone := getPicture(t, door, "choir.example.org", cardPicture)
	if gone.Code != http.StatusNotFound || strings.Contains(gone.Body.String(), cardPicture.String()) {
		t.Errorf("bytes that are gone answered %d %s", gone.Code, gone.Body)
	}
}

// A public route still verifies a presented bearer (known-traps.md): a revoked token sent to the
// picture's route is refused as on every public route, and none sent is served anonymously.
func TestThePicturesRouteIsPublicAndStillVerifiesABearer(t *testing.T) {
	route := http.MethodGet + " " + APIBasePath + "/auth/workspace-pictures/{mediaId}"
	if !PublicRoutes[route] {
		t.Fatal("the picture's route needs a credential")
	}
	serve := func(auth *authenticator, bearer string) (*httptest.ResponseRecorder, bool) {
		mux := NewMux()
		reached := false
		mux.HandleFunc(route, func(http.ResponseWriter, *http.Request) { reached = true })
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			APIBasePath+"/auth/workspace-pictures/"+cardPicture.String(), nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		response := httptest.NewRecorder()
		Authenticated{
			Routes: mux, Authenticator: auth,
			Locale: env.LocaleConfig{DefaultLocale: "en", DefaultTimeZone: "UTC"},
			Next:   mux,
		}.ServeHTTP(response, r)
		return response, reached
	}

	if response, reached := serve(&authenticator{}, ""); !reached {
		t.Fatalf("an anonymous request was refused: %d", response.Code)
	}
	revoked := &authenticator{err: shared.ErrUnauthenticated.WithDetail("access.token_revoked")}
	response, reached := serve(revoked, credential)
	if reached || response.Code != http.StatusUnauthorized || revoked.calls != 1 {
		t.Errorf("a revoked bearer answered %d, reached %v, asked %d times", response.Code, reached, revoked.calls)
	}
}
