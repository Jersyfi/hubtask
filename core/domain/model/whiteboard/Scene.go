// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The bounds of a scene.
const (
	// MaxElements is the first release's hard limit of one board: the target is smooth drawing at
	// 5,000 objects (products.md, M4), and the scene is saved whole in one request, which the
	// installation's body limit bounds (security.md §9).
	MaxElements = 5000
	// MaxTextLength bounds the words of one object - a sticky note, a label, a frame's name.
	MaxTextLength = 2000
	// MaxCoordinate bounds where an object may stand, so that a position stays an exact number in
	// every client and an export's view box stays drawable.
	MaxCoordinate = 1_000_000
	// MaxSize bounds an object's width and height.
	MaxSize = 100_000
)

// Kind is what an object on the board is.
type Kind string

const (
	KindSticky    Kind = "STICKY"
	KindShape     Kind = "SHAPE"
	KindText      Kind = "TEXT"
	KindConnector Kind = "CONNECTOR"
	// KindFrame names an area of the board, for navigation and export; a reference's anchor names
	// one (`hubtask:whiteboard/<id>#<frame>`).
	KindFrame Kind = "FRAME"
	// KindCard draws an object of the suite - an entry - as its card; what it shows is resolved for
	// each reader, never copied onto the board (P-19).
	KindCard Kind = "CARD"
)

var kinds = []Kind{KindSticky, KindShape, KindText, KindConnector, KindFrame, KindCard}

// Kinds are every kind of object.
func Kinds() []string { return names(kinds) }

// Shape is the outline of a SHAPE: the basic set and the flowchart's.
type Shape string

const (
	ShapeRectangle     Shape = "RECTANGLE"
	ShapeRounded       Shape = "ROUNDED"
	ShapeEllipse       Shape = "ELLIPSE"
	ShapeDiamond       Shape = "DIAMOND"
	ShapeParallelogram Shape = "PARALLELOGRAM"
	ShapeTriangle      Shape = "TRIANGLE"
)

var shapes = []Shape{ShapeRectangle, ShapeRounded, ShapeEllipse, ShapeDiamond, ShapeParallelogram, ShapeTriangle}

// Shapes are every outline.
func Shapes() []string { return names(shapes) }

// Route is how a connector runs between its ends.
type Route string

const (
	RouteStraight Route = "STRAIGHT"
	RouteElbow    Route = "ELBOW"
	RouteCurved   Route = "CURVED"
)

var routes = []Route{RouteStraight, RouteElbow, RouteCurved}

// Routes are every route.
func Routes() []string { return names(routes) }

// Head is what a connector's end is drawn with.
type Head string

const (
	HeadNone  Head = "NONE"
	HeadArrow Head = "ARROW"
)

var heads = []Head{HeadNone, HeadArrow}

// Heads are every end.
func Heads() []string { return names(heads) }

// Element is one object on the board.
//
// A connector's ends are the objects it is attached to (From, To) and stays attached to as they
// move; an end attached to nothing is a point: the start at (X, Y), the end at (X+Width,
// Y+Height), which is why a connector's width and height may be negative.
type Element struct {
	ID     string
	Kind   Kind
	X, Y   float64
	Width  float64
	Height float64
	// Text is a sticky note's or a shape's words, a text's, a connector's label or a frame's name.
	Text string
	// Colour is a label token's name (rule 15), empty for the kind's own.
	Colour shared.LabelToken
	Shape  Shape
	From   string
	To     string
	Route  Route
	Start  Head
	End    Head
	// Target is what a card draws.
	Target reference.Reference
	Locked bool
}

// Scene is the board's objects in drawing order, the last on top.
type Scene struct{ Elements []Element }

// Equal reports whether two scenes hold the same objects in the same order.
func (s Scene) Equal(other Scene) bool { return slices.Equal(s.Elements, other.Elements) }

// Frame answers the frame of that identifier.
func (s Scene) Frame(id string) (Element, bool) {
	for _, element := range s.Elements {
		if element.ID == id && element.Kind == KindFrame {
			return element, true
		}
	}
	return Element{}, false
}

// Within answers the objects inside a frame - the frame, whatever lies wholly inside it, and the
// connectors between two of those - in drawing order.
func (s Scene) Within(frame Element) Scene {
	inside := map[string]bool{}
	var out []Element
	for _, element := range s.Elements {
		if element.Kind == KindConnector {
			continue
		}
		if element.ID == frame.ID || contains(frame, element) {
			inside[element.ID] = true
		}
	}
	for _, element := range s.Elements {
		switch {
		case element.Kind == KindConnector:
			if inside[element.From] && inside[element.To] {
				out = append(out, element)
			}
		case inside[element.ID]:
			out = append(out, element)
		}
	}
	return Scene{Elements: out}
}

func contains(frame, element Element) bool {
	left, top, right, bottom := box(frame)
	l, t, r, b := box(element)
	return l >= left && t >= top && r <= right && b <= bottom
}

func box(e Element) (left, top, right, bottom float64) {
	return math.Min(e.X, e.X+e.Width), math.Min(e.Y, e.Y+e.Height), math.Max(e.X, e.X+e.Width), math.Max(e.Y, e.Y+e.Height)
}

// References are what the scene points at: each card's object, and the `hubtask:` links of its
// texts, once each, the strongest drawing winning. A link back to the board itself is left out.
func (s Scene) References(self shared.ID) []reference.Found {
	var found []reference.Found
	index := map[reference.Reference]int{}
	add := func(f reference.Found) {
		if f.Reference.Kind == reference.KindWhiteboard && f.Reference.ID == self {
			return
		}
		if at, seen := index[f.Reference]; seen {
			if f.Strength == reference.Card && found[at].Strength == reference.Mention {
				found[at].Strength = reference.Card
			}
			return
		}
		index[f.Reference] = len(found)
		found = append(found, f)
	}
	for _, element := range s.Elements {
		if element.Kind == KindCard {
			add(reference.Found{Reference: element.Target, Strength: reference.Card})
		}
	}
	for _, element := range s.Elements {
		for _, f := range reference.Extract(element.Text) {
			add(f)
		}
	}
	return found
}

// Cards are the identifiers of the entries the board draws as cards, in drawing order.
func (s Scene) Cards() []shared.ID {
	var out []shared.ID
	for _, element := range s.Elements {
		if element.Kind == KindCard && element.Target.Kind == reference.KindEntry && !slices.Contains(out, element.Target.ID) {
			out = append(out, element.Target.ID)
		}
	}
	return out
}

var elementID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// CheckScene brings a scene to its stored form and judges it: identifiers unique and shaped as a
// reference's anchor, every kind, outline, route and end known, coordinates and sizes in bounds,
// words in normal form C and bounded, colours label tokens, a connector's ends objects of the
// board that are not connectors, a card's target a well-formed reference to an entry.
func CheckScene(scene Scene, form text.Normalizer) (Scene, error) {
	if len(scene.Elements) > MaxElements {
		return Scene{}, invalid("/elements", "whiteboards.too_many_elements").WithParams(map[string]string{"maximum": "5000"})
	}
	out := Scene{Elements: make([]Element, 0, len(scene.Elements))}
	kindOf := make(map[string]Kind, len(scene.Elements))
	for i, element := range scene.Elements {
		path := "/elements/" + strconv.Itoa(i)
		if !elementID.MatchString(element.ID) {
			return Scene{}, invalid(path+"/id", "whiteboards.element_id_malformed")
		}
		if _, seen := kindOf[element.ID]; seen {
			return Scene{}, invalid(path+"/id", "whiteboards.element_id_duplicate")
		}
		kindOf[element.ID] = element.Kind
	}
	for i, element := range scene.Elements {
		checked, err := checkElement(element, kindOf, "/elements/"+strconv.Itoa(i), form)
		if err != nil {
			return Scene{}, err
		}
		out.Elements = append(out.Elements, checked)
	}
	return out, nil
}

func checkElement(e Element, kindOf map[string]Kind, path string, form text.Normalizer) (Element, error) {
	if !slices.Contains(kinds, e.Kind) {
		return Element{}, invalid(path+"/kind", "whiteboards.element_kind_unknown")
	}
	for _, value := range []float64{e.X, e.Y} {
		if math.IsNaN(value) || math.Abs(value) > MaxCoordinate {
			return Element{}, invalid(path, "whiteboards.element_out_of_bounds")
		}
	}
	for _, value := range []float64{e.Width, e.Height} {
		// Only a connector runs backwards: its size is the vector from its start to its end.
		if math.IsNaN(value) || math.Abs(value) > MaxSize || (value < 0 && e.Kind != KindConnector) {
			return Element{}, invalid(path, "whiteboards.element_size_invalid")
		}
	}
	words, err := shared.NFC(e.Text, form)
	if err != nil {
		return Element{}, err
	}
	if utf8.RuneCountInString(words) > MaxTextLength {
		return Element{}, invalid(path+"/text", "whiteboards.element_text_too_long").WithParams(map[string]string{"maximum": "2000"})
	}
	if !utf8.ValidString(words) || strings.ContainsRune(words, 0) {
		return Element{}, invalid(path+"/text", "whiteboards.element_text_malformed")
	}
	e.Text = words
	if e.Colour != "" && !shared.IsLabelToken(string(e.Colour)) {
		return Element{}, invalid(path+"/colour", "whiteboards.element_colour_unknown")
	}
	// Each kind keeps only what it draws, so that a stray member of another kind is not stored.
	switch e.Kind {
	case KindShape:
		if e.Shape == "" {
			e.Shape = ShapeRectangle
		}
		if !slices.Contains(shapes, e.Shape) {
			return Element{}, invalid(path+"/shape", "whiteboards.element_shape_unknown")
		}
	default:
		e.Shape = ""
	}
	if e.Kind == KindConnector {
		if e.Route == "" {
			e.Route = RouteStraight
		}
		if e.Start == "" {
			e.Start = HeadNone
		}
		if e.End == "" {
			e.End = HeadArrow
		}
		if !slices.Contains(routes, e.Route) {
			return Element{}, invalid(path+"/route", "whiteboards.connector_route_unknown")
		}
		if !slices.Contains(heads, e.Start) || !slices.Contains(heads, e.End) {
			return Element{}, invalid(path, "whiteboards.connector_head_unknown")
		}
		for _, end := range []struct{ field, id string }{{"/from", e.From}, {"/to", e.To}} {
			if end.id == "" {
				continue
			}
			kind, ok := kindOf[end.id]
			if !ok || kind == KindConnector || end.id == e.ID {
				return Element{}, invalid(path+end.field, "whiteboards.connector_end_unknown")
			}
		}
	} else {
		e.From, e.To, e.Route, e.Start, e.End = "", "", "", "", ""
	}
	if e.Kind == KindCard {
		if e.Target.Kind != reference.KindEntry {
			return Element{}, invalid(path+"/target", "whiteboards.card_target_unsupported")
		}
		target, err := reference.New(e.Target.Kind, e.Target.ID, "")
		if err != nil {
			return Element{}, invalid(path+"/target", "whiteboards.card_target_malformed")
		}
		e.Target = target
	} else {
		e.Target = reference.Reference{}
	}
	return e, nil
}

func names[T ~string](values []T) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = string(value)
	}
	return out
}
