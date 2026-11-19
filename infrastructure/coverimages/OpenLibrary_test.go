// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package coverimages

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/port/httpclient"
)

// fakeClient answers by the address asked (a key ending in "?" by its prefix), and keeps what was asked.
type fakeClient struct {
	mu      sync.Mutex
	asked   []string
	answers map[string]httpclient.Response
}

func (f *fakeClient) Do(ctx context.Context, req httpclient.Request) (httpclient.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, req.URL)
	if _, has := ctx.Deadline(); !has || req.TargetClass != targetClass {
		return httpclient.Response{}, errors.New("a call without a deadline or its class")
	}
	if answer, ok := f.answers[req.URL]; ok {
		return answer, nil
	}
	for prefix, answer := range f.answers {
		if strings.HasSuffix(prefix, "?") && strings.HasPrefix(req.URL, prefix) {
			return answer, nil
		}
	}
	return httpclient.Response{Status: 404}, nil
}

func jpeg(body string) httpclient.Response {
	return httpclient.Response{Status: 200, Header: map[string][]string{"Content-Type": {"image/jpeg"}}, Body: []byte(body)}
}

const one, two = "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"

// The library is asked at the configured address, a trailing slash or not.
func TestTheLibraryIsAskedAtTheConfiguredAddress(t *testing.T) {
	for _, configured := range []string{"https://library.test/v1", "https://library.test/v1/"} {
		if got := NewOpenLibrary(&fakeClient{}, configured, time.Second).Base; got != "https://library.test/v1" {
			t.Errorf("NewOpenLibrary(%q).Base = %q", configured, got)
		}
	}
}

// A search finds openly licensed pictures with who made them and a preview fetched by the server;
// a picture whose preview fails is left out, and an id that is no id never reaches an address.
func TestASearchFindsPicturesWithTheirMakerAndPreview(t *testing.T) {
	base := "https://library.test/v1"
	client := &fakeClient{answers: map[string]httpclient.Response{
		base + "/images/?": {Status: 200, Body: []byte(`{"results":[` +
			`{"id":"` + one + `","creator":"Ada  Lovelace"},{"id":"` + two + `","creator":"B"},{"id":"../x","creator":"C"}]}`)},
		base + "/images/" + one + "/thumb/": jpeg("preview"),
	}}
	library := OpenLibrary{Client: client, Base: base, Timeout: time.Second}
	pictures, err := library.Search(context.Background(), " fjord ")
	if err != nil || len(pictures) != 1 {
		t.Fatalf("Search = %+v, %v", pictures, err)
	}
	if pictures[0].ID != one || pictures[0].Creator != "Ada Lovelace" || string(pictures[0].Preview) != "preview" || pictures[0].PreviewType != "image/jpeg" {
		t.Fatalf("picture = %+v", pictures[0])
	}
	if !strings.Contains(client.asked[0], "q=fjord") || !strings.Contains(client.asked[0], "license_type=commercial%2Cmodification") {
		t.Fatalf("asked %s", client.asked[0])
	}
	for _, asked := range client.asked {
		if strings.Contains(asked, "../x") {
			t.Fatalf("a malformed id reached an address: %s", asked)
		}
	}
	if none, err := library.Search(context.Background(), "  "); err != nil || none != nil || len(client.asked) != 3 {
		t.Fatalf("an empty search asked the library: %v %v", none, err)
	}
}

// Fetching takes the picture itself, or the library's full-size rendition where the picture
// cannot be read; an answer that is no picture is refused.
func TestFetchingTakesThePictureOrItsFullRendition(t *testing.T) {
	base := "https://library.test/v1"
	client := &fakeClient{answers: map[string]httpclient.Response{
		base + "/images/" + one + "/":                      {Status: 200, Body: []byte(`{"creator":"Ada","url":"https://pictures.test/a.jpg"}`)},
		"https://pictures.test/a.jpg":                      jpeg("big"),
		base + "/images/" + two + "/thumb/?full_size=true": jpeg("rendition"),
		base + "/images/" + two + "/":                      {Status: 200, Body: []byte(`{"creator":"B","url":"https://pictures.test/gone.jpg"}`)},
	}}
	library := OpenLibrary{Client: client, Base: base, Timeout: time.Second}
	image, err := library.Fetch(context.Background(), one)
	if err != nil || string(image.Content) != "big" || image.Creator != "Ada" || image.ContentType != "image/jpeg" {
		t.Fatalf("Fetch = %+v, %v", image, err)
	}
	image, err = library.Fetch(context.Background(), two)
	if err != nil || string(image.Content) != "rendition" {
		t.Fatalf("Fetch fallback = %+v, %v", image, err)
	}
	if _, err := library.Fetch(context.Background(), "../etc"); err == nil {
		t.Fatal("a malformed id was fetched")
	}
	client.answers["https://pictures.test/a.jpg"] = httpclient.Response{Status: 200, Header: map[string][]string{"Content-Type": {"text/html"}}, Body: []byte("<html>")}
	delete(client.answers, base+"/images/"+two+"/thumb/?full_size=true")
	client.answers[base+"/images/"+one+"/thumb/?full_size=true"] = httpclient.Response{Status: 500}
	if _, err := library.Fetch(context.Background(), one); err == nil {
		t.Fatal("an answer that is no picture was taken")
	}
}
