// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

func catalogue(t *testing.T) i18n.Catalogue {
	t.Helper()
	loaded, err := i18n.LoadEnglish()
	if err != nil {
		t.Fatalf("loading the catalogue: %v", err)
	}
	return loaded
}

// installation stands in for a Hubtask installation. It records the last request, so that a test
// can assert on what the client sent as well as on what it made of the answer.
type installation struct {
	server  *httptest.Server
	request *http.Request
	body    string
}

func serve(t *testing.T, handler http.HandlerFunc) *installation {
	t.Helper()
	stub := &installation{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.request = r.Clone(r.Context())
		if r.Body != nil {
			raw, _ := io.ReadAll(r.Body)
			stub.body = string(raw)
		}
		handler(w, r)
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func clientFor(t *testing.T, stub *installation) *Client {
	t.Helper()
	client, err := NewClient(
		Profile{BaseURL: stub.server.URL, Token: secret.New("hbt_pat_token")},
		catalogue(t), 5*time.Second)
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}
	return client
}

// problemJSON writes an RFC 9457 document the way presentation/rest does.
func problemJSON(w http.ResponseWriter, status int, document map[string]any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(document)
}

func TestTheCredentialTravelsInTheHeaderAndThePathCarriesTheAPIVersion(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})

	var payload struct{ Data []any }
	if err := clientFor(t, stub).Get(context.Background(), "/containers", nil, &payload); err != nil {
		t.Fatalf("calling: %v", err)
	}

	if got := stub.request.Header.Get("Authorization"); got != "Bearer hbt_pat_token" {
		t.Errorf("Authorization %q", got)
	}
	if got := stub.request.URL.Path; got != APIPath+"/containers" {
		t.Errorf("path %q, want %q", got, APIPath+"/containers")
	}
	// The token belongs in the header and nowhere else: a URL travels into logs and into the
	// Referer of whatever the target renders.
	if strings.Contains(stub.request.URL.String(), "hbt_pat") {
		t.Error("the token reached the URL")
	}
}

func TestARefusalIsRenderedFromTheCatalogue(t *testing.T) {
	for _, tc := range []struct {
		name     string
		document map[string]any
		status   int
		want     string
	}{
		{
			name:     "the detail code wins where the catalogue knows it",
			status:   http.StatusNotFound,
			document: map[string]any{"status": 404, "code": "not_found", "detail_code": "containers.parent_not_found"},
			want:     "containers.parent_not_found",
		},
		{
			name:     "the contract code carries an answer with no detail code",
			status:   http.StatusNotFound,
			document: map[string]any{"status": 404, "code": "not_found"},
			want:     "errors.not_found",
		},
		{
			name:     "a detail code the catalogue has never heard of falls back to the contract code",
			status:   http.StatusConflict,
			document: map[string]any{"status": 409, "code": "conflict", "detail_code": "items.invented_for_this_test"},
			want:     "errors.conflict",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				problemJSON(w, tc.status, tc.document)
			})

			err := clientFor(t, stub).Get(context.Background(), "/containers", nil, nil)
			if err == nil {
				t.Fatal("a refusal came back as a success")
			}

			expected, known := catalogue(t).Message(tc.want, nil)
			if !known {
				t.Fatalf("the test expects %s to be in the catalogue", tc.want)
			}
			if err.Error() != expected {
				t.Errorf("message %q, want %q", err.Error(), expected)
			}
			// Whatever else is true of the message, it is not the document.
			if strings.Contains(err.Error(), "{") {
				t.Errorf("the message %q looks like JSON", err.Error())
			}
		})
	}
}

// The parameters are what makes a message a sentence rather than a template.
func TestTheParametersOfAProblemReachTheMessage(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusInternalServerError, map[string]any{
			"status": 500, "code": "internal", "request_id": "01J8ZQ",
		})
	})

	err := clientFor(t, stub).Get(context.Background(), "/containers", nil, nil)
	if err == nil {
		t.Fatal("a 500 came back as a success")
	}
	if !strings.Contains(err.Error(), "01J8ZQ") {
		t.Errorf("the message %q does not quote the request ID the user is asked to reference", err.Error())
	}
}

func TestFieldErrorsAreRenderedUnderTheMessage(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"status": 422, "code": "validation_failed",
			"field_errors": []map[string]any{
				{"path": "/name", "code": "shared.required"},
			},
		})
	})

	err := clientFor(t, stub).Get(context.Background(), "/containers", nil, nil)
	if err == nil {
		t.Fatal("a validation failure came back as a success")
	}
	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines, want the message and one field: %q", len(lines), err.Error())
	}
	if !strings.Contains(lines[1], "name:") {
		t.Errorf("the field line %q does not name the field", lines[1])
	}
}

// The server puts the parameters on the problem and the code on the field, so a field error
// rendered from its own parameters alone would print its placeholders at a user. This is the case
// the end-to-end session caught: "A {item_type} cannot sit in a {parent_type}."
func TestAFieldErrorBorrowsTheProblemsParameters(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"status": 422, "code": "capability_not_supported",
			"detail_code": "items.parent_type_invalid",
			"params":      map[string]any{"item_type": "ACTIVITY", "parent_type": "TASK"},
			"field_errors": []map[string]any{
				{"path": "/parent_id", "code": "items.parent_item_required"},
			},
		})
	})

	err := clientFor(t, stub).Get(context.Background(), "/items", nil, nil)
	if err == nil {
		t.Fatal("a validation failure came back as a success")
	}
	if strings.Contains(err.Error(), "{") {
		t.Errorf("a placeholder reached the message: %q", err.Error())
	}
	// The field line is rendered from the problem's parameters, which is where the server puts
	// them - so it names the type rather than printing the placeholder for it.
	if !strings.Contains(err.Error(), "parent_id: ") || !strings.Contains(err.Error(), "ACTIVITY") {
		t.Errorf("the field line is missing or empty: %q", err.Error())
	}
}

// Two identical sentences read as a bug in the client rather than as detail: the field line adds
// only the field's name, and the sentence above it has already named the problem.
func TestAFieldErrorThatRepeatsTheMessageIsNotPrintedTwice(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusNotFound, map[string]any{
			"status": 404, "code": "not_found", "detail_code": "items.collection_not_found",
			"field_errors": []map[string]any{
				{"path": "/collection_id", "code": "items.collection_not_found"},
			},
		})
	})

	err := clientFor(t, stub).Get(context.Background(), "/items", nil, nil)
	if err == nil {
		t.Fatal("a refusal came back as a success")
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("the message was printed twice: %q", err.Error())
	}
}

// A proxy, a captive portal or a load balancer answers with HTML. Echoing that at a user would
// be printing somebody else's page into their terminal.
func TestAnAnswerThatIsNotAProblemDocumentIsReportedByItsStatus(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html><body>Gateway problem</body></html>"))
	})

	err := clientFor(t, stub).Get(context.Background(), "/containers", nil, nil)
	if err == nil {
		t.Fatal("a 502 came back as a success")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("the message %q does not name the status", err.Error())
	}
	if strings.Contains(err.Error(), "Gateway problem") {
		t.Errorf("the body reached the message: %q", err.Error())
	}
}

func TestAnEmptyAnswerIsASuccess(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := clientFor(t, stub).Delete(context.Background(), "/items/x", `W/"3"`); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if got := stub.request.Header.Get("If-Match"); got != `W/"3"` {
		t.Errorf("If-Match %q", got)
	}
}

func TestABodyIsSentAsJSON(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"Hub"}`))
	})

	var created struct{ Name string }
	if err := clientFor(t, stub).Post(context.Background(), "/containers",
		map[string]string{"name": "Hub"}, &created); err != nil {
		t.Fatalf("creating: %v", err)
	}
	if stub.body != `{"name":"Hub"}` {
		t.Errorf("body %q", stub.body)
	}
	if got := stub.request.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type %q", got)
	}
	if created.Name != "Hub" {
		t.Errorf("the answer was not decoded: %+v", created)
	}
}

func TestAClientNeedsAnAddressAndACredential(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile Profile
		want    string
	}{
		{"no address", Profile{Token: secret.New("t")}, "no installation"},
		{"no credential", Profile{BaseURL: "https://hub.example.com"}, "not signed in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(tc.profile, catalogue(t), time.Second)
			if err == nil {
				t.Fatal("a client was built without what it needs")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("message %q, want it to contain %q", err.Error(), tc.want)
			}
		})
	}
}

// The guard's own refusals are message codes too, and they must not read as though they came
// from a different program than the server's.
func TestAnAddressTheGuardRefusesIsRenderedFromTheCatalogue(t *testing.T) {
	client, err := NewClient(
		Profile{BaseURL: "https://hub.invalid", Token: secret.New("t")}, catalogue(t), 2*time.Second)
	if err != nil {
		t.Fatalf("building the client: %v", err)
	}

	err = client.Get(context.Background(), "/containers", nil, nil)
	if err == nil {
		t.Fatal("a call to a name that does not resolve succeeded")
	}
	if strings.Contains(err.Error(), "dependency.") || strings.Contains(err.Error(), "errors.") {
		t.Errorf("the message %q is a code rather than a sentence", err.Error())
	}
}
