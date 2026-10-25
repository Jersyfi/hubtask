// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"sort"
	"strconv"
	"strings"

	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
)

// The intents the Alexa adapter answers. Stable: a model pasted into the console names them.
const (
	IntentAdd         = "AddIntent"
	IntentAddSentence = "AddSentenceIntent"
	IntentRead        = "ReadIntent"
	IntentComplete    = "CompleteIntent"
	IntentToday       = "TodayIntent"

	SlotThing    = "Thing"
	SlotList     = "List"
	SlotSentence = "Sentence"

	listType  = "HUBTASK_LIST"
	thingType = "HUBTASK_THING"
)

// AlexaModel is the interaction model for a language: the intents, their sentences and the two
// slot types. It is the same for every workspace and never names one's lists: a custom slot is not
// an enumeration - Alexa hands over whatever name was said - and Hubtask resolves the name itself,
// so a list made or renamed tomorrow works without the model being pasted again. The samples only
// teach Alexa how a list name and a thing sound; the workspace's own names reach Alexa at run time,
// with every answer (DynamicEntities).
func AlexaModel(language domain.Language, invocation string) map[string]any {
	words := modelWords[language]
	intents := []any{
		intent(IntentAdd, words.add, slot(SlotThing, thingType), slot(SlotList, listType)),
		intent(IntentAddSentence, words.addSentence, slot(SlotSentence, "AMAZON.SearchQuery")),
		intent(IntentRead, words.read, slot(SlotList, listType)),
		intent(IntentComplete, words.complete, slot(SlotThing, thingType), slot(SlotList, listType)),
		intent(IntentToday, words.today),
		map[string]any{"name": "AMAZON.HelpIntent", "samples": []any{}},
		map[string]any{"name": "AMAZON.StopIntent", "samples": []any{}},
		map[string]any{"name": "AMAZON.CancelIntent", "samples": []any{}},
		map[string]any{"name": "AMAZON.FallbackIntent", "samples": []any{}},
		map[string]any{"name": "AMAZON.NavigateHomeIntent", "samples": []any{}},
	}
	return map[string]any{"interactionModel": map[string]any{"languageModel": map[string]any{
		"invocationName": invocationOr(invocation, language),
		"intents":        intents,
		"types": []any{
			map[string]any{"name": listType, "values": plainValues(words.lists)},
			map[string]any{"name": thingType, "values": plainValues(words.things)},
		},
	}}}
}

// MaxDynamicPlaces is how many of a workspace's names one answer hands Alexa: its limit for a
// session's dynamic entities is a hundred values.
const MaxDynamicPlaces = 100

// DynamicEntities is the directive that teaches Alexa the workspace's own list names for the rest
// of the session: every collection and category by every name, the list word included. Nil for a
// workspace without one.
func DynamicEntities(known []domain.Known, language domain.Language) map[string]any {
	values := placeValues(known, language)
	if len(values) == 0 {
		return nil
	}
	if len(values) > MaxDynamicPlaces {
		values = values[:MaxDynamicPlaces]
	}
	return map[string]any{
		"type":           "Dialog.UpdateDynamicEntities",
		"updateBehavior": "REPLACE",
		"types":          []any{map[string]any{"name": listType, "values": values}},
	}
}

func intent(name string, samples []string, slots ...map[string]any) map[string]any {
	out := map[string]any{"name": name, "samples": toAny(samples)}
	if len(slots) > 0 {
		list := make([]any, 0, len(slots))
		for _, each := range slots {
			list = append(list, each)
		}
		out["slots"] = list
	}
	return out
}

func slot(name, kind string) map[string]any { return map[string]any{"name": name, "type": kind} }

// placeValues are the places by every name: one value per place, its other names as synonyms.
func placeValues(known []domain.Known, language domain.Language) []any {
	type entry struct {
		value    string
		synonyms map[string]bool
	}
	byPlace := map[domain.Place]*entry{}
	var order []domain.Place
	for _, each := range known {
		current, seen := byPlace[each.Place]
		if !seen {
			current = &entry{value: each.Name, synonyms: map[string]bool{}}
			byPlace[each.Place] = current
			order = append(order, each.Place)
		} else if !strings.EqualFold(each.Name, current.value) {
			current.synonyms[each.Name] = true
		}
		// "Einkauf" is also said "Einkaufsliste"; Alexa learns both.
		if language == domain.German && !strings.HasSuffix(strings.ToLower(each.Name), "liste") {
			current.synonyms[each.Name+"liste"] = true
		}
		if language == domain.English && !strings.HasSuffix(strings.ToLower(each.Name), "list") {
			current.synonyms[each.Name+" list"] = true
		}
	}
	values := make([]any, 0, len(order))
	for index, place := range order {
		current := byPlace[place]
		synonyms := make([]string, 0, len(current.synonyms))
		for each := range current.synonyms {
			synonyms = append(synonyms, each)
		}
		sort.Strings(synonyms)
		values = append(values, map[string]any{
			"id":   "place-" + strconv.Itoa(index),
			"name": map[string]any{"value": current.value, "synonyms": toAny(synonyms)},
		})
	}
	return values
}

func plainValues(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, map[string]any{"name": map[string]any{"value": value}})
	}
	return out
}

func toAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

type languageWords struct {
	add, addSentence, read, complete, today, things, lists []string
}

// The sentences are for every kind of place, not for a shopping list: a store one buys *at* ("bei
// Metro"), a project one works *for*, a list things go *on*, a folder things go *in*. Each
// preposition is said with and without its article, before and after the verb, and the verb at
// the end ("… kaufen", "… eintragen") as well as at the start - which is how people talk to a
// kitchen speaker. The thing samples mix groceries, chores and work, because they teach Alexa how
// a thing sounds, and a model taught only food hears "Angebot schreiben" as noise.
var modelWords = map[domain.Language]languageWords{
	domain.German: {
		add: expand(
			[]string{"{Thing} %s {List}"},
			[]string{
				"setz {Thing} %s {List}", "setze {Thing} %s {List}", "schreib {Thing} %s {List}",
				"schreibe {Thing} %s {List}", "pack {Thing} %s {List}", "notier {Thing} %s {List}",
				"notiere {Thing} %s {List}", "trag {Thing} %s {List} ein", "trage {Thing} %s {List} ein",
				"nimm {Thing} %s {List} auf", "leg {Thing} %s {List} an", "{Thing} %s {List} eintragen",
				"{Thing} %s {List} notieren", "{Thing} %s {List} setzen",
			},
			[]string{"auf", "auf die", "in", "in die", "bei", "für", "zu", "zur", "zum"},
		).with(
			"{Thing} bei {List} kaufen", "{Thing} bei {List} besorgen", "{Thing} bei {List} holen",
			"bei {List} brauchen wir {Thing}", "für {List} brauche ich {Thing}", "füg {Thing} zu {List} hinzu",
			"füge {Thing} zu {List} hinzu",
			"neue aufgabe {Thing} in {List}", "neue aufgabe {Thing} für {List}", "neuer eintrag {Thing} in {List}",
			"neuer eintrag {Thing} bei {List}", "erinnere mich an {Thing} bei {List}",
		),
		addSentence: []string{
			"setz {Sentence}", "setze {Sentence}", "schreib {Sentence}", "schreibe {Sentence}",
			"notier {Sentence}", "notiere {Sentence}", "merk dir {Sentence}", "füge {Sentence} hinzu",
			"trag {Sentence} ein", "neue aufgabe {Sentence}", "neuer eintrag {Sentence}",
			"wir brauchen {Sentence}", "ich brauche {Sentence}", "ich muss {Sentence}",
		},
		read: expand(
			[]string{
				"was steht %s {List}", "was ist %s {List}", "was ist noch offen %s {List}",
				"was gibt es %s {List}",
			},
			nil,
			[]string{"auf", "auf der", "in", "in der", "bei", "für"},
		).with(
			"lies {List} vor", "lies die {List} vor", "lies mir {List} vor", "zeig {List}",
			"was brauchen wir bei {List}", "was muss ich bei {List} kaufen", "was ist für {List} zu tun",
			"was steht auf der liste", "lies die liste vor",
		),
		complete: expand(
			[]string{
				"{Thing} %s {List} ist erledigt", "{Thing} %s {List} erledigt", "hak {Thing} %s {List} ab",
				"hake {Thing} %s {List} ab",
			},
			nil,
			[]string{"auf", "auf der", "in", "in der", "bei", "für"},
		).with(
			"{Thing} ist erledigt", "{Thing} erledigt", "{Thing} ist fertig", "{Thing} ist gekauft",
			"{Thing} habe ich", "{Thing} haben wir", "hak {Thing} ab", "hake {Thing} ab",
			"{Thing} abhaken", "streich {Thing}", "streiche {Thing}", "streich {Thing} von {List}",
			"streiche {Thing} von {List}", "{Thing} von {List} ist erledigt",
		),
		today: []string{
			"was ist heute fällig", "was steht heute an", "was muss ich heute machen", "was habe ich heute zu tun",
			"was ist überfällig", "was liegt heute an",
		},
		lists: []string{
			"Einkauf", "Einkaufsliste", "Metro", "Aldi", "Rewe", "Edeka", "Drogerie", "Baumarkt", "Haushalt",
			"Büro", "Arbeit", "Projekt", "Website", "Akquise", "Garten", "Urlaub", "Geschenke", "Familie",
			"Kinder", "Auto", "Haus", "Renovierung", "Steuern", "Termine", "Ideen", "Lesen", "Filme",
		},
		things: []string{
			"Milch", "Butter", "Eier", "Brot", "Käse", "Reis", "Nudeln", "Kartoffeln", "Zwiebeln",
			"Tomaten", "Äpfel", "Bananen", "Kaffee", "Mineralwasser", "Gemüsebrühe", "Olivenöl",
			"Mehl", "Zucker", "Hähnchenbrust", "Toilettenpapier", "Spülmittel", "Waschmittel",
			"Müllbeutel", "Batterien", "Glühbirne", "Katzenfutter", "Druckerpatronen", "Kopierpapier",
			"Schrauben", "Holzleim", "Blumenerde", "Rasendünger",
			"Rechnung bezahlen", "Müll rausbringen", "Arzttermin vereinbaren", "Paket abholen",
			"Auto waschen", "Reifen wechseln", "Blumen gießen", "Steuererklärung machen",
			"Geschenk kaufen", "Wohnung putzen", "Fenster putzen", "Kinder abholen", "Oma anrufen",
			"Kündigung schreiben", "Versicherung prüfen", "Urlaub buchen", "Handwerker anrufen",
			"Angebot schreiben", "Kunden anrufen", "Rechnung stellen", "Präsentation vorbereiten",
			"Protokoll schicken", "Meeting planen", "Vertrag prüfen", "Logo abstimmen", "Texte freigeben",
			"Bewerbung lesen", "Lieferanten anfragen", "Backup prüfen", "Website aktualisieren",
		},
	},
	domain.English: {
		add: expand(
			[]string{"{Thing} %s {List}"},
			[]string{
				"add {Thing} %s {List}", "put {Thing} %s {List}", "write {Thing} %s {List}",
				"note {Thing} %s {List}", "save {Thing} %s {List}",
			},
			[]string{"to", "to the", "to my", "on", "on the", "on my", "in", "in the", "for", "for the", "at"},
		).with(
			"buy {Thing} at {List}", "get {Thing} at {List}", "pick up {Thing} at {List}",
			"at {List} we need {Thing}", "for {List} i need {Thing}", "new task {Thing} in {List}",
			"new task {Thing} for {List}", "remind me to {Thing} for {List}",
		),
		addSentence: []string{
			"add {Sentence}", "put {Sentence}", "write {Sentence}", "note {Sentence}", "remember {Sentence}",
			"new task {Sentence}", "we need {Sentence}", "i need {Sentence}", "i have to {Sentence}",
		},
		read: expand(
			[]string{"what is %s {List}", "what's %s {List}", "what is still open %s {List}"},
			nil,
			[]string{"on", "on the", "on my", "in", "in the", "for", "at"},
		).with(
			"read {List}", "read the {List}", "read my {List}", "read me {List}", "what do we need at {List}",
			"what is on the list", "read the list",
		),
		complete: expand(
			[]string{"{Thing} %s {List} is done", "check off {Thing} %s {List}", "tick off {Thing} %s {List}"},
			nil,
			[]string{"on", "on the", "in", "in the", "for", "at"},
		).with(
			"{Thing} is done", "{Thing} is finished", "{Thing} is bought", "check off {Thing}",
			"tick off {Thing}", "cross off {Thing}", "remove {Thing} from {List}", "we have {Thing}",
		),
		today: []string{
			"what is due today", "what's due today", "what do i have to do today", "what is overdue",
			"what is on for today",
		},
		lists: []string{
			"shopping", "shopping list", "Costco", "Walmart", "Tesco", "pharmacy", "hardware store",
			"household", "office", "work", "project", "website", "sales", "garden", "holiday", "presents",
			"family", "kids", "car", "house", "renovation", "taxes", "appointments", "ideas", "reading",
		},
		things: []string{
			"milk", "butter", "eggs", "bread", "cheese", "rice", "pasta", "potatoes", "onions", "tomatoes",
			"apples", "bananas", "coffee", "sparkling water", "stock cubes", "olive oil", "flour", "sugar",
			"chicken breast", "toilet paper", "dish soap", "laundry detergent", "bin bags", "batteries",
			"light bulb", "cat food", "printer ink", "copy paper", "screws", "wood glue", "potting soil",
			"pay the bill", "take out the trash", "book a doctor's appointment", "pick up the parcel",
			"wash the car", "change the tyres", "water the plants", "do the tax return", "buy a present",
			"clean the flat", "pick up the kids", "call grandma", "book the holiday", "call the plumber",
			"write the quote", "call the client", "send the invoice", "prepare the presentation",
			"send the minutes", "plan the meeting", "review the contract", "approve the copy",
			"read the application", "ask the suppliers", "check the backup", "update the website",
		},
	},
}

// samples is a list of sample sentences that can be extended by more.
type samples []string

func (s samples) with(more ...string) []string {
	return dedupe(append(append([]string{}, s...), more...))
}

// expand writes every pattern with every preposition: "{Thing} %s {List}" with "bei" is
// "{Thing} bei {List}". Patterns where the verb comes first are written the same way.
func expand(plain, verbs, prepositions []string) samples {
	var out samples
	for _, pattern := range append(append([]string{}, plain...), verbs...) {
		for _, preposition := range prepositions {
			out = append(out, strings.Replace(pattern, "%s", preposition, 1))
		}
	}
	return out
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		// Alexa compares sentences without case; the slot names inside them keep theirs.
		value = strings.Join(strings.Fields(value), " ")
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// invocationOr is the name given, or the language's default.
func invocationOr(invocation string, language domain.Language) string {
	if name := domain.NormalInvocation(invocation); name != "" {
		return name
	}
	return domain.DefaultInvocation(language)
}
