// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The resource half of the server (J-11, ADR-0051). What is asked here is that a URI becomes a
// catalogue call and nothing else: no permission is decided in this package, no tenant is resolved
// in it, and a refusal comes back as the application layer wrote it.

const (
	itemURI      = Scheme + "items/0192f000-0000-7000-8000-000000000401"
	containerURI = Scheme + "containers/0192f000-0000-7000-8000-00000000000c"
	viewURI      = Scheme + "views/0192f000-0000-7000-8000-00000000000e"
)

// A resource read is a catalogue read: the URI's segment picks the use case and its identifier
// becomes that use case's input. Everything after that is the application layer's.
func TestAResourceReadIsACatalogueRead(t *testing.T) {
	for _, c := range []struct {
		uri     string
		useCase string
		field   string
		id      string
	}{
		{itemURI, "GetWorkItem", "item_id", "0192f000-0000-7000-8000-000000000401"},
		{containerURI, "GetContainer", "container_id", "0192f000-0000-7000-8000-00000000000c"},
		{viewURI, "GetSavedView", "view_id", "0192f000-0000-7000-8000-00000000000e"},
	} {
		store := &catalogue{out: usecase.Output{"id": c.id, "name": "Whatever"}}
		answer := rpc(t, serverWith(store),
			`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"`+c.uri+`"}}`, true)

		if store.invokedName != c.useCase {
			t.Errorf("%s reached %q, want %q", c.uri, store.invokedName, c.useCase)
		}
		if store.invokedIn[c.field] != c.id {
			t.Errorf("%s bound %v to %s, want the identifier", c.uri, store.invokedIn, c.field)
		}
		if len(store.invokedIn) != 1 {
			t.Errorf("%s sent %v, want the identifier alone", c.uri, store.invokedIn)
		}

		result, _ := answer["result"].(map[string]any)
		contents, _ := result["contents"].([]any)
		if len(contents) != 1 {
			t.Fatalf("%s answered %v", c.uri, result)
		}
		content, _ := contents[0].(map[string]any)
		if content["uri"] != c.uri || content["mimeType"] != MimeType {
			t.Errorf("the content does not name itself: %v", content)
		}
		if text, _ := content["text"].(string); !strings.Contains(text, c.id) {
			t.Errorf("the content does not carry the read: %q", text)
		}
	}
}

// A URI this server cannot address is the client having called wrongly, and is refused before the
// catalogue is reached - which is what makes it a protocol error rather than a refusal.
func TestAUriThisServerCannotAddressIsAProtocolError(t *testing.T) {
	for name, uri := range map[string]string{
		"another scheme":              "https://example.org/items/1",
		"a kind nobody serves":        Scheme + "invoices/0192f000-0000-7000-8000-000000000401",
		"no identifier":               Scheme + "items",
		"an empty identifier":         Scheme + "items/",
		"something that is not an id": Scheme + "items/the-first-one",
		"a path rather than an id":    Scheme + "items/0192f000-0000-7000-8000-000000000401/comments",
	} {
		t.Run(name, func(t *testing.T) {
			store := &catalogue{out: usecase.Output{"id": "x"}}
			answer := rpc(t, serverWith(store),
				`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"`+uri+`"}}`, true)

			failure, _ := answer["error"].(map[string]any)
			if failure["code"] != float64(codeInvalidParams) {
				t.Fatalf("%q answered %v, want an invalid-params error", uri, answer)
			}
			if store.invokedName != "" {
				t.Errorf("%q reached the catalogue as %q", uri, store.invokedName)
			}
		})
	}
}

// A refusal is the application layer's, rendered as a code an agent can act on. It is a JSON-RPC
// error rather than a result because `resources/read`'s result has nowhere to put one - and the
// exact refusal travels in `data`, which is where the difference between the two survives.
func TestARefusedResourceReadCarriesTheProblemDocument(t *testing.T) {
	for name, refusal := range map[string]*shared.Error{
		"a URI naming another workspace's entry": shared.ErrNotFound,
		"an entry the agent may not read":        shared.ErrForbidden,
	} {
		t.Run(name, func(t *testing.T) {
			store := &catalogue{errs: map[string]error{"GetWorkItem": refusal}}
			answer := rpc(t, serverWith(store),
				`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"`+itemURI+`"}}`, true)

			failure, _ := answer["error"].(map[string]any)
			if failure["code"] != float64(codeResourceRefused) {
				t.Fatalf("a refusal answered %v", answer)
			}
			problem, _ := failure["data"].(map[string]any)
			if problem["code"] != refusal.Code {
				t.Errorf("the refusal is reported as %v, want %q", problem, refusal.Code)
			}
			// And no result: a client that read both would render content that does not exist.
			if _, answered := answer["result"]; answered {
				t.Errorf("a refused read answered a result as well: %v", answer)
			}
		})
	}
}

// The templates publish the URI shapes, which is how a client learns to construct one for content
// it cannot enumerate.
func TestTheTemplatesPublishEveryShape(t *testing.T) {
	answer := rpc(t, serverWith(&catalogue{}),
		`{"jsonrpc":"2.0","id":1,"method":"resources/templates/list"}`, true)

	result, _ := answer["result"].(map[string]any)
	templates, _ := result["resourceTemplates"].([]any)
	if len(templates) != len(ResourceKinds()) {
		t.Fatalf("%d templates for %d kinds", len(templates), len(ResourceKinds()))
	}

	shapes := make(map[string]bool, len(templates))
	for _, entry := range templates {
		template, _ := entry.(map[string]any)
		shapes[template["uriTemplate"].(string)] = true
		if template["mimeType"] != MimeType {
			t.Errorf("%v does not name its type", template)
		}
		if description, _ := template["description"].(string); description == "" {
			t.Errorf("%v carries no description for an agent to read", template)
		}
	}
	for _, want := range []string{
		Scheme + "items/{id}", Scheme + "containers/{id}", Scheme + "views/{id}",
	} {
		if !shapes[want] {
			t.Errorf("the templates do not publish %q", want)
		}
	}
}

// The list is the two kinds that can be enumerated. Entries are deliberately not among them: no
// unanchored item list exists in this product, and one built for this method would be a tenant
// asking for everything.
func TestTheResourceListEnumeratesWhatCanBeEnumerated(t *testing.T) {
	store := listCatalogue()

	answer := rpc(t, serverWith(store),
		`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, true)

	result, _ := answer["result"].(map[string]any)
	resources, _ := result["resources"].([]any)
	if len(resources) != 2 {
		t.Fatalf("the list answered %v", result)
	}

	uris := make(map[string]string, len(resources))
	for _, entry := range resources {
		resource, _ := entry.(map[string]any)
		uris[resource["uri"].(string)] = resource["name"].(string)
	}
	if uris[containerURI] != "Home" {
		t.Errorf("the hub is listed as %q", uris[containerURI])
	}
	if uris[viewURI] != "This week" {
		t.Errorf("the view is listed as %q", uris[viewURI])
	}

	for _, call := range store.calls {
		if call.name == "ListWorkItems" || call.name == "SearchItems" {
			t.Errorf("the list tried to enumerate entries through %q", call.name)
		}
	}
}

// The walk: the paged kind's own cursor is the MCP cursor, and the unpaged kinds ride on the first
// page only. Repeating them on every page would make an agent read them once per page.
func TestTheResourceListPagesTheKindThatIsPaged(t *testing.T) {
	store := listCatalogue()

	first := rpc(t, serverWith(store), `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, true)
	result, _ := first["result"].(map[string]any)
	if result["nextCursor"] != "the-next-hub" {
		t.Fatalf("the first page does not continue: %v", result)
	}

	second := listCatalogue()
	second.outs["ListContainers"] = usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}
	answer := rpc(t, serverWith(second),
		`{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{"cursor":"the-next-hub"}}`, true)

	page, _ := answer["result"].(map[string]any)
	if _, more := page["nextCursor"]; more {
		t.Errorf("the walk did not end: %v", page)
	}
	if resources, _ := page["resources"].([]any); len(resources) != 0 {
		t.Errorf("the second page repeated the unpaged kinds: %v", resources)
	}

	var listedContainers bool
	for _, call := range second.calls {
		if call.name == "ListContainers" {
			listedContainers = true
			if call.in["cursor"] != "the-next-hub" {
				t.Errorf("the cursor reached the catalogue as %v", call.in)
			}
		}
		if call.name == "ListSavedViews" {
			t.Error("a continued walk read the unpaged kinds again")
		}
	}
	if !listedContainers {
		t.Error("the continued walk never asked for the next page")
	}
}

// Exactly one kind may be paged: the MCP cursor is that kind's own cursor passed through, and a
// second paged kind would need a composite cursor this package would have to invent and sign.
func TestOnlyOneResourceKindIsPaged(t *testing.T) {
	paged := 0
	for _, kind := range ResourceKinds() {
		if kind.Paged {
			paged++
		}
		if kind.Paged && kind.ListUseCase == "" {
			t.Errorf("%s is paged and has no list", kind)
		}
	}
	if paged != 1 {
		t.Errorf("%d resource kinds are paged, want exactly one - see the field's own comment", paged)
	}
}

// A resource method without an actor is refused rather than run: a read without an actor would run
// without a tenant.
func TestResourceMethodsWithoutAnActorAreRefused(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"` + itemURI + `"}}`,
	} {
		store := &catalogue{out: usecase.Output{"id": "x"}}
		answer := rpc(t, serverWith(store), body, false)

		failure, _ := answer["error"].(map[string]any)
		if failure["code"] != float64(codeInternalError) {
			t.Errorf("an unauthenticated call answered %v", answer)
		}
		if store.invokedName != "" {
			t.Errorf("it reached the catalogue as %q", store.invokedName)
		}
	}
}

// listCatalogue answers one hub and one saved view, with a hub page that continues.
func listCatalogue() *catalogue {
	return &catalogue{outs: map[string]usecase.Output{
		"ListContainers": {
			"data": []usecase.Output{
				{"id": "0192f000-0000-7000-8000-00000000000c", "name": "Home", "type": "HUB"},
			},
			"page": map[string]any{"next_cursor": "the-next-hub", "has_more": true},
		},
		"ListSavedViews": {
			"data": []usecase.Output{
				{"id": "0192f000-0000-7000-8000-00000000000e", "name": "This week"},
			},
		},
	}}
}

// URIOf is what the list builds its addresses with, and what a test that constructs one should use
// rather than a string it wrote out by hand.
func TestUriOfAndParseAreEachOthersInverse(t *testing.T) {
	id := shared.MustParseID("0192f000-0000-7000-8000-000000000401")

	for _, kind := range ResourceKinds() {
		uri := URIOf(kind.Segment, id)
		parsed, back, err := ParseResourceURI(uri)
		if err != nil {
			t.Fatalf("%q does not parse: %v", uri, err)
		}
		if parsed.Segment != kind.Segment || back != id {
			t.Errorf("%q parsed as %s/%s", uri, parsed.Segment, back)
		}
	}
}
