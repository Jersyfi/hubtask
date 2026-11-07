// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	desk "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type publicForm struct {
	opened, submitted int
	sent              servicedesk.PublicSubmission
}

func (p *publicForm) Open(context.Context, integration.FormLinkToken, string) (servicedesk.PublicFormView, error) {
	p.opened++
	return servicedesk.PublicFormView{
		Catalogue: []desk.CatalogueEntry{{Key: "feedback", Name: "Feedback"}},
		Challenge: desk.Challenge{Value: "1.a.b", Bits: desk.PublicFormWorkBits, ExpiresAt: time.Date(2026, 10, 31, 9, 10, 0, 0, time.UTC)},
	}, nil
}

func (p *publicForm) Submit(_ context.Context, _ integration.FormLinkToken, in servicedesk.PublicSubmission) error {
	p.submitted++
	p.sent = in
	return nil
}

// UC-SVC-13 checks 1 and 5: the public form answers its catalogue and a challenge without a
// bearer, takes a submission with 202 and nothing else, and a malformed token is not even asked
// about.
func TestAPublicFormIsServedWithoutABearer(t *testing.T) {
	form := &publicForm{}
	controller := NewRestController()
	controller.Feeds = &FeedDoor{Forms: form}
	token, err := integration.NewFormLinkToken(shared.MustParseID("0192f000-0000-7000-8000-00000000000a"), bytes.Repeat([]byte{3}, integration.FormLinkTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	serve := func(method, presented, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), method, APIBasePath+"/public/forms/"+presented, strings.NewReader(body))
		controller.Routes().ServeHTTP(recorder, request)
		return recorder
	}
	opened := serve(http.MethodGet, token.Secret(), "")
	if opened.Code != http.StatusOK || opened.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status %d: %s", opened.Code, opened.Body)
	}
	var view struct {
		Data      []struct{ Key string } `json:"data"`
		Challenge struct {
			Value string `json:"value"`
			Bits  int    `json:"bits"`
		} `json:"challenge"`
	}
	if err := json.Unmarshal(opened.Body.Bytes(), &view); err != nil || len(view.Data) != 1 || view.Challenge.Bits != desk.PublicFormWorkBits {
		t.Fatalf("view %s (%v)", opened.Body, err)
	}
	sent := serve(http.MethodPost, token.Secret(), `{"entry_type":"feedback","title":"Lob","requester_email":"kaya@example.org","challenge":"1.a.b","solution":"42"}`)
	if sent.Code != http.StatusAccepted || sent.Body.Len() != 0 || form.sent.RequesterEmail != "kaya@example.org" || form.sent.Solution != "42" {
		t.Fatalf("submitted %d %q %+v", sent.Code, sent.Body, form.sent)
	}
	if unknown := serve(http.MethodPost, token.Secret(), `{"entry_type":"feedback","title":"x","challenge":"c","solution":"1","assignee_id":"x"}`); unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown member answered %d", unknown.Code)
	}
	if serve(http.MethodGet, "hbt_form_nonsense", "").Code != http.StatusNotFound ||
		serve(http.MethodPost, "hbt_form_nonsense", "{}").Code != http.StatusNotFound || form.opened != 1 || form.submitted != 1 {
		t.Fatalf("a malformed token was asked about (%d, %d)", form.opened, form.submitted)
	}
}

// UC-SVC-13 check 5: a public form's submissions count per client address, and only them.
func TestAPublicFormIsCountedPerAddress(t *testing.T) {
	bucket := PublicFormBucket(5, 2)
	post := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/public/forms/hbt_form_x", nil)
	post.RemoteAddr = "203.0.113.7:4711"
	if got := bucket(post); got.Key != "203.0.113.7" || got.Limit != 5 || got.Burst != 2 {
		t.Fatalf("bucket %+v", got)
	}
	for _, other := range []*http.Request{
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/public/forms/hbt_form_x", nil),
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/public/pages/hbt_page_x", nil),
	} {
		if got := bucket(other); got.Key != "" {
			t.Errorf("%s %s counted: %+v", other.Method, other.URL.Path, got)
		}
	}
}

// UC-SVC-13 check 5: an address rotated inside one network does not buy a fresh budget - the
// network's bucket (/24, /64) refuses with 429 once its own budget is spent.
func TestAPublicFormIsCountedPerNetwork(t *testing.T) {
	bucket := PublicFormNetworkBucket(5, 2)
	for address, want := range map[string]string{
		"203.0.113.7:4711":              "203.0.113.0/24",
		"[2001:db8:1:2:aaaa::1]:4711":   "2001:db8:1:2::/64",
		"[2001:db8:1:2:ffff:1::9]:4711": "2001:db8:1:2::/64",
	} {
		post := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/public/forms/hbt_form_x", nil)
		post.RemoteAddr = address
		if got := bucket(post); got.Key != want || got.Limit != 5*publicNetworkShare || got.Burst != 2*publicNetworkShare {
			t.Errorf("%s: bucket %+v, want key %s", address, got, want)
		}
	}
	if got := bucket(httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/public/forms/hbt_form_x", nil)); got.Key != "" {
		t.Errorf("a read counted: %+v", got)
	}

	limited := Limited{
		Limiter: NewRateLimiter(), Level: "public_network", Bucket: bucket,
		Next: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }),
	}
	refused := 0
	for host := range 64 {
		post := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/public/forms/hbt_form_x", nil)
		post.RemoteAddr = fmt.Sprintf("[2001:db8:1:2::%x]:4711", host+1)
		answer := httptest.NewRecorder()
		limited.ServeHTTP(answer, post)
		if answer.Code == http.StatusTooManyRequests {
			refused++
		}
	}
	if refused < 64-2*publicNetworkShare {
		t.Fatalf("64 addresses of one /64 were refused %d times", refused)
	}
}
