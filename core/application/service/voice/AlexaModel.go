// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"sort"
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

	// InvocationName is what follows "Alexa, sag" / "Alexa, ask".
	InvocationName = "hubtask"

	listType  = "HUBTASK_LIST"
	thingType = "HUBTASK_THING"
)

// AlexaModel is the interaction model for a language: the intents, their sentences and the two
// slot types - the workspace's places by every name they are spoken by, and things as people
// say them.
//
// A thing slot is a custom type rather than free text so that it can stand beside the list in one
// sentence ("{Thing} auf die {List}"); Alexa hands over what was said whether or not it is among
// the samples, and the samples only teach it how things sound. A sentence with no list in it is
// the free-text intent, read whole by Hubtask - which is why a new list works before the model is
// pasted again.
func AlexaModel(language domain.Language, known []domain.Known) map[string]any {
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
		"invocationName": InvocationName,
		"intents":        intents,
		"types": []any{
			map[string]any{"name": listType, "values": listValues(known, language)},
			map[string]any{"name": thingType, "values": plainValues(words.things)},
		},
	}}}
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

// listValues are the places by every name: one value per place, its other names as synonyms.
func listValues(known []domain.Known, language domain.Language) []any {
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
	values := make([]any, 0, len(order)+1)
	if len(order) == 0 {
		// A slot type needs a value; a workspace with no list yet still gets a model that loads.
		fallback := map[domain.Language]string{domain.German: "Einkauf", domain.English: "Shopping"}[language]
		values = append(values, map[string]any{"name": map[string]any{"value": fallback}})
	}
	for _, place := range order {
		current := byPlace[place]
		synonyms := make([]string, 0, len(current.synonyms))
		for each := range current.synonyms {
			synonyms = append(synonyms, each)
		}
		sort.Strings(synonyms)
		values = append(values, map[string]any{"name": map[string]any{"value": current.value, "synonyms": toAny(synonyms)}})
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
	add, addSentence, read, complete, today, things []string
}

var modelWords = map[domain.Language]languageWords{
	domain.German: {
		add: []string{
			"{Thing} auf die {List}", "{Thing} auf {List}", "{Thing} auf die {List} setzen",
			"setz {Thing} auf die {List}", "setze {Thing} auf die {List}", "schreib {Thing} auf die {List}",
			"pack {Thing} auf die {List}", "füge {Thing} zur {List} hinzu", "{Thing} zur {List}",
			"{Thing} in {List}", "{Thing} in die {List}", "trag {Thing} in {List} ein",
		},
		addSentence: []string{
			"setz {Sentence}", "setze {Sentence}", "schreib {Sentence}", "schreibe {Sentence}",
			"notier {Sentence}", "notiere {Sentence}", "pack {Sentence}", "merk dir {Sentence}",
			"füge {Sentence} hinzu", "trag {Sentence} ein", "wir brauchen {Sentence}", "ich brauche {Sentence}",
		},
		read: []string{
			"was steht auf der {List}", "was steht auf {List}", "lies die {List} vor", "lies {List} vor",
			"was ist auf der {List}", "was fehlt auf der {List}", "was steht auf der liste", "lies die liste vor",
		},
		complete: []string{
			"{Thing} ist erledigt", "{Thing} erledigt", "hak {Thing} ab", "hake {Thing} ab",
			"streich {Thing}", "streiche {Thing}", "{Thing} von der {List} ist erledigt",
			"streich {Thing} von der {List}", "hak {Thing} auf der {List} ab", "{Thing} haben wir",
		},
		today: []string{
			"was ist heute fällig", "was steht heute an", "was muss ich heute machen", "was habe ich heute zu tun",
			"was ist überfällig", "was liegt heute an",
		},
		things: []string{
			"Butter", "Milch", "Eier", "Brot", "Brötchen", "Käse", "Joghurt", "Quark", "Sahne", "Reis",
			"Nudeln", "Kartoffeln", "Zwiebeln", "Knoblauch", "Tomaten", "Gurke", "Paprika", "Salat",
			"Äpfel", "Bananen", "Zitronen", "Orangen", "Mehl", "Zucker", "Salz", "Pfeffer", "Öl",
			"Olivenöl", "Essig", "Kaffee", "Tee", "Wasser", "Saft", "Bier", "Wein", "Hackfleisch",
			"Hähnchen", "Wurst", "Schinken", "Lachs", "Toilettenpapier", "Küchenrolle", "Spülmittel",
			"Waschmittel", "Zahnpasta", "Shampoo", "Müllbeutel", "Batterien", "Glühbirne", "Katzenfutter",
			"Hundefutter", "Haferflocken", "Müsli", "Konfitüre", "Honig", "Schokolade", "Chips", "Kekse",
			"Tiefkühlpizza", "Erbsen", "Mais", "Bohnen", "Linsen", "Thunfisch", "Ketchup", "Senf",
			"Mayonnaise", "Basilikum", "Petersilie", "Hefe", "Backpulver", "Vanillezucker", "Frischkäse",
			"Mozzarella", "Parmesan", "Avocado", "Karotten", "Brokkoli", "Zucchini", "Champignons",
			"Rechnung bezahlen", "Müll rausbringen", "Arzttermin vereinbaren", "Paket abholen",
			"Auto waschen", "Blumen gießen", "Steuererklärung", "Geschenk kaufen",
		},
	},
	domain.English: {
		add: []string{
			"{Thing} to the {List}", "{Thing} to my {List}", "{Thing} to {List}", "{Thing} on the {List}",
			"add {Thing} to the {List}", "add {Thing} to my {List}", "put {Thing} on the {List}",
			"put {Thing} on my {List}", "{Thing} onto the {List}", "write {Thing} on the {List}",
		},
		addSentence: []string{
			"add {Sentence}", "put {Sentence}", "write {Sentence}", "note {Sentence}", "remember {Sentence}",
			"we need {Sentence}", "i need {Sentence}",
		},
		read: []string{
			"what is on the {List}", "what is on my {List}", "what's on the {List}", "read the {List}",
			"read my {List}", "what is on the list", "read the list",
		},
		complete: []string{
			"{Thing} is done", "check off {Thing}", "tick off {Thing}", "cross off {Thing}",
			"remove {Thing} from the {List}", "check off {Thing} on the {List}", "we have {Thing}",
		},
		today: []string{
			"what is due today", "what's due today", "what do i have to do today", "what is overdue",
			"what is on for today",
		},
		things: []string{
			"butter", "milk", "eggs", "bread", "cheese", "yogurt", "cream", "rice", "pasta", "potatoes",
			"onions", "garlic", "tomatoes", "cucumber", "peppers", "lettuce", "apples", "bananas", "lemons",
			"oranges", "flour", "sugar", "salt", "pepper", "oil", "olive oil", "vinegar", "coffee", "tea",
			"water", "juice", "beer", "wine", "minced meat", "chicken", "sausages", "ham", "salmon",
			"toilet paper", "kitchen roll", "dish soap", "laundry detergent", "toothpaste", "shampoo",
			"bin bags", "batteries", "light bulb", "cat food", "dog food", "oats", "cereal", "jam", "honey",
			"chocolate", "crisps", "biscuits", "frozen pizza", "peas", "corn", "beans", "lentils", "tuna",
			"ketchup", "mustard", "mayonnaise", "basil", "parsley", "yeast", "baking powder", "cream cheese",
			"mozzarella", "parmesan", "avocado", "carrots", "broccoli", "zucchini", "mushrooms",
			"pay the bill", "take out the trash", "book a doctor's appointment", "pick up the parcel",
			"wash the car", "water the plants", "tax return", "buy a present",
		},
	},
}
