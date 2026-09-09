// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"fmt"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Scheme is the URI scheme this server addresses its content under (ai-first.md §1.1).
const Scheme = "hubtask://"

// ResourceKind is one shape of addressable content: a URI, the catalogue read behind it, and the
// input field the identifier goes into (ADR-0051 decision 1).
//
// Everything about a resource is here, and nothing about it is anywhere else. `resources/read`
// parses a URI into one of these and calls `Catalogue.Invoke`; the permission question, the tenant
// and the audit entry are the application layer's, exactly as they are for a tool call.
type ResourceKind struct {
	// Segment is what follows the scheme: `items`, `containers`, `views`.
	Segment string
	// UseCase is the catalogue name of the read.
	UseCase string
	// Field is the input the identifier is bound to.
	Field string
	// Title is what a client shows for the *kind* in `resources/templates/list`. Protocol
	// documentation like a tool's description, not display text (ADR-0011): what a person sees in
	// their own language is rendered by their own client from their own catalogue.
	Title string
	// Description says what the content is and what reading it costs.
	Description string
	// NameField is the key of the read's own output that names one instance. Most resources here
	// are named by `name` and an entry by its `title`; the field is in the table rather than
	// assumed, so that a kind whose read calls it something else needs no second code path.
	NameField string
	// ListUseCase is the catalogue read that enumerates this kind, or empty where the kind cannot
	// be enumerated. Entries are the empty one, and deliberately so (ADR-0051 decision 3): no
	// unanchored item list exists in this product, and one built for `resources/list` would be a
	// tenant asking for everything.
	ListUseCase string
	// Paged says whether that list answers a page with a cursor rather than a bare list.
	//
	// Exactly one kind may be paged, and `list` relies on it: the MCP cursor is that kind's own
	// cursor passed through, and two paged kinds would need a composite cursor this package would
	// have to invent and sign itself. A test holds the invariant, so a second one is caught here
	// rather than by an agent walking a page twice.
	Paged bool
}

// resourceKinds is the table, chosen rather than derived (ADR-0051 decision 2).
//
// A rule - "every Get* use case is a resource" - would publish `GetRecurrence` and `GetTemplate` as
// content, and they are not: they are attributes of something else. Three entries somebody chose
// are a smaller thing to keep right than a rule that is wrong for two of the cases it covers, and
// the cost is named where it falls: **a fourth kind is a change to this file**, which is the
// opposite of the tools half of this package and the honest trade for it.
var resourceKinds = []ResourceKind{
	{
		Segment: "items", UseCase: "GetWorkItem", Field: "item_id", NameField: "title",
		Title: "Work item",
		Description: "One task, work package or activity, with its fields, its labels and its " +
			"position in the tree. The same projection GET /items/{id} answers, and the same " +
			"permission decides it.",
	},
	{
		Segment: "containers", UseCase: "GetContainer", Field: "container_id", NameField: "name",
		ListUseCase: "ListContainers", Paged: true,
		Title: "Container",
		Description: "One hub or collection, with its policies and its lifecycle stamps. Reading " +
			"it says nothing about what is inside: the entries are their own resources.",
	},
	{
		Segment: "views", UseCase: "GetSavedView", Field: "view_id", NameField: "name",
		ListUseCase: "ListSavedViews",
		Title:       "Saved view",
		Description: "One saved query: its filter, its grouping, its visible fields and how it is " +
			"shared. Reading the view does not run it - `query_items` does that, with the view's " +
			"own query as the argument.",
	},
}

// MimeType is what every resource in this server answers as. One type for all of them, because
// every one of them is a use case result rendered the way `tools/call` renders one - a client that
// can read a tool result can read a resource.
const MimeType = "application/json"

// ResourceTemplate is one entry of `resources/templates/list`: the URI shape of content a client
// cannot enumerate.
//
// It is what makes decision 3 of ADR-0051 workable. `resources/list` does not enumerate entries -
// there is no unanchored item list in this product and there should not be one - so a client learns
// `hubtask://items/{id}` from here and constructs the URI itself, from an identifier it got out of
// `search_items`, `query_items` or a container it has read.
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// Resource is one entry of `resources/list`.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType"`
}

// ResourceTemplates renders the table as URI templates, in the table's own order.
func ResourceTemplates() []ResourceTemplate {
	templates := make([]ResourceTemplate, 0, len(resourceKinds))
	for _, kind := range resourceKinds {
		templates = append(templates, ResourceTemplate{
			URITemplate: Scheme + kind.Segment + "/{id}",
			Name:        kind.Segment,
			Title:       kind.Title,
			Description: kind.Description,
			MimeType:    MimeType,
		})
	}
	return templates
}

// URIOf builds the address of one instance.
func URIOf(segment string, id shared.ID) string {
	return Scheme + segment + "/" + id.String()
}

// ErrURI is what a URI this server cannot address answers with.
//
// A protocol failure rather than a refusal, and the distinction is the one MCP asks for: an unknown
// scheme or an unaddressable segment is the client having called wrongly, while a resource the
// actor may not read is the server saying no - and only the second is worth reporting to the person
// the agent works for. Which is why this error never travels: `resources/read` turns it into a
// JSON-RPC error, and a refused read into a result.
type ErrURI struct{ Reason string }

func (e ErrURI) Error() string { return e.Reason }

// ParseResourceURI resolves one URI into the read behind it and the identifier to run it with.
//
// The identifier is parsed here, and that is deliberate: an identifier that is not one is a URI
// this server cannot address, which is a protocol error - while an identifier that is well formed
// and names nothing the actor may see is the use case's answer, and has to stay one. The difference
// matters because the second must never confirm existence (ADR-0051 decision 4).
func ParseResourceURI(uri string) (ResourceKind, shared.ID, error) {
	rest, found := strings.CutPrefix(uri, Scheme)
	if !found {
		return ResourceKind{}, "", ErrURI{Reason: "unknown URI scheme"}
	}

	segment, identifier, found := strings.Cut(rest, "/")
	if !found || identifier == "" || strings.Contains(identifier, "/") {
		return ResourceKind{}, "", ErrURI{Reason: "unknown URI shape"}
	}

	for _, kind := range resourceKinds {
		if kind.Segment != segment {
			continue
		}
		id, err := shared.ParseID(identifier)
		if err != nil {
			return ResourceKind{}, "", ErrURI{Reason: "the URI does not name an identifier"}
		}
		return kind, id, nil
	}
	return ResourceKind{}, "", ErrURI{Reason: "unknown resource kind"}
}

// resourceOf renders one row of a catalogue list as a resource entry.
//
// The row is a use case output, so the identifier and the name are read out of it by key rather
// than by type: this package never reconstructs a domain object, which is what keeps it an adapter.
func resourceOf(kind ResourceKind, row usecase.Output) (Resource, bool) {
	id, ok := row["id"].(string)
	if !ok || id == "" {
		return Resource{}, false
	}
	name, _ := row[kind.NameField].(string)
	if name == "" {
		// Every one of these reads answers a name, and an entry without one would be a resource a
		// client cannot show in a list. Naming it by its kind is better than dropping it: the URI
		// is still readable, which is what the entry is for.
		name = kind.Segment
	}
	return Resource{
		URI:      Scheme + kind.Segment + "/" + id,
		Name:     name,
		Title:    kind.Title,
		MimeType: MimeType,
	}, true
}

// ResourceKinds answers the table, for the server and for the tests that hold its invariants.
func ResourceKinds() []ResourceKind { return resourceKinds }

// String is what a log or a test prints for one kind.
func (k ResourceKind) String() string {
	return fmt.Sprintf("%s -> %s(%s)", Scheme+k.Segment+"/{id}", k.UseCase, k.Field)
}
