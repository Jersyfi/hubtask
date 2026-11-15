// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package coverimages asks an openly licensed image library for a page's cover (E-18, UC-DOC-16
// check 5): the one library Hubtask knows, reached only through the guarded client (rule 6),
// each call under a deadline (rule 7). It needs no key; nothing it logs holds the query.
package coverimages

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/coverimages"
	"github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// targetClass is the outbound metric's label for every call here.
const targetClass = "cover_images"

// pageSize is how many pictures one search shows: two rows of the picker's grid.
const pageSize = 12

// The library's ids are UUIDs; anything else never reaches a URL.
var pictureID = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// OpenLibrary is the library's HTTP client. Base is its API root, without a trailing slash.
type OpenLibrary struct {
	Client  httpclient.Port
	Base    string
	Timeout time.Duration
}

// NewOpenLibrary answers the library at its public address.
func NewOpenLibrary(client httpclient.Port, timeout time.Duration) OpenLibrary {
	return OpenLibrary{Client: client, Base: "https://api.openverse.org/v1", Timeout: timeout}
}

type found struct {
	Results []struct {
		ID      string `json:"id"`
		Creator string `json:"creator"`
		URL     string `json:"url"`
	} `json:"results"`
}

// Search finds pictures that may be changed and used commercially, with a preview of each fetched
// beside one another; a preview that fails leaves its picture out.
func (o OpenLibrary) Search(ctx context.Context, query string) ([]coverimages.Picture, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("page_size", fmt.Sprint(pageSize))
	params.Set("license_type", "commercial,modification")
	params.Set("mature", "false")
	var answer found
	if err := o.getJSON(ctx, o.Base+"/images/?"+params.Encode(), &answer); err != nil {
		return nil, err
	}
	pictures := make([]coverimages.Picture, len(answer.Results))
	var wait sync.WaitGroup
	for i, result := range answer.Results {
		if !pictureID.MatchString(result.ID) {
			continue
		}
		pictures[i] = coverimages.Picture{ID: result.ID, Creator: creatorOf(result.Creator)}
		wait.Add(1)
		concurrency.Go(ctx, "coverimages.preview", func(ctx context.Context) {
			defer wait.Done()
			body, kind, err := o.getImage(ctx, o.Base+"/images/"+result.ID+"/thumb/")
			if err == nil {
				pictures[i].Preview, pictures[i].PreviewType = body, kind
			}
		})
	}
	wait.Wait()
	out := pictures[:0]
	for _, picture := range pictures {
		if len(picture.Preview) > 0 {
			out = append(out, picture)
		}
	}
	return out, nil
}

// Fetch reads one picture's larger rendition: the picture itself where it fits the client's
// limit, otherwise the library's own full-size rendition of it.
func (o OpenLibrary) Fetch(ctx context.Context, id string) (coverimages.Image, error) {
	if !pictureID.MatchString(id) {
		return coverimages.Image{}, shared.ErrNotFound.WithDetail("pages.cover_image_not_found")
	}
	var detail struct {
		Creator string `json:"creator"`
		URL     string `json:"url"`
	}
	if err := o.getJSON(ctx, o.Base+"/images/"+id+"/", &detail); err != nil {
		return coverimages.Image{}, err
	}
	body, kind, err := o.getImage(ctx, detail.URL)
	if err != nil {
		if body, kind, err = o.getImage(ctx, o.Base+"/images/"+id+"/thumb/?full_size=true"); err != nil {
			return coverimages.Image{}, err
		}
	}
	return coverimages.Image{Content: body, ContentType: kind, Creator: creatorOf(detail.Creator)}, nil
}

func (o OpenLibrary) getJSON(ctx context.Context, address string, into any) error {
	response, err := o.get(ctx, address, "application/json")
	if err != nil {
		return err
	}
	if response.Status == 404 {
		return shared.ErrNotFound.WithDetail("pages.cover_image_not_found")
	}
	if response.Status != 200 {
		return shared.ErrUnavailable.WithDetail("pages.cover_images_unavailable")
	}
	if err := json.Unmarshal(response.Body, into); err != nil {
		return shared.ErrUnavailable.WithDetail("pages.cover_images_unavailable").WithCause(err)
	}
	return nil
}

// getImage reads a picture, accepting only the three kinds a cover takes.
func (o OpenLibrary) getImage(ctx context.Context, address string) ([]byte, string, error) {
	if address == "" {
		return nil, "", shared.ErrNotFound.WithDetail("pages.cover_image_not_found")
	}
	response, err := o.get(ctx, address, "image/*")
	if err != nil {
		return nil, "", err
	}
	kind := ""
	if values := response.Header["Content-Type"]; len(values) > 0 {
		kind = strings.ToLower(strings.TrimSpace(strings.Split(values[0], ";")[0]))
	}
	if response.Status != 200 || (kind != "image/jpeg" && kind != "image/png" && kind != "image/webp") || len(response.Body) == 0 {
		return nil, "", shared.ErrUnavailable.WithDetail("pages.cover_images_unavailable")
	}
	return response.Body, kind, nil
}

func (o OpenLibrary) get(ctx context.Context, address, accept string) (httpclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	response, err := o.Client.Do(ctx, httpclient.Request{
		Method: "GET", URL: address, TargetClass: targetClass,
		Header: map[string][]string{"Accept": {accept}},
	})
	if err != nil {
		// Not wrapped: the guard's error names the address, and a search's address holds its query (rule 10).
		return httpclient.Response{}, shared.ErrUnavailable.WithDetail("pages.cover_images_unavailable")
	}
	return response, nil
}

// creatorOf keeps a maker's name to one line of the cover's credit length.
func creatorOf(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if runes := []rune(name); len(runes) > 120 {
		name = string(runes[:120])
	}
	return name
}
