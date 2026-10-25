// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package voice is what a spoken sentence means to Hubtask (ADR-0093): the things named, their
// amounts and the place they go. A voice assistant hands over the words as it heard them; what
// they are is decided here, the same for every assistant, so a new list works the moment it
// exists rather than when a speech model is rebuilt.
package voice

import (
	"strconv"
	"strings"
	"unicode"
)

// Language is the language a sentence was spoken in. Only the two the catalogue speaks.
type Language string

const (
	German  Language = "de"
	English Language = "en"
)

// LanguageOf reads a locale such as "de-DE" or "en-GB"; anything else is English.
func LanguageOf(locale string) Language {
	if strings.HasPrefix(strings.ToLower(locale), "de") {
		return German
	}
	return English
}

// Thing is one entry a sentence asks for: a title and, where one was said, an amount.
type Thing struct {
	Title  string
	Amount string
}

// Addition is "these things, to that place". Target is empty where no place was named: the
// default list takes them.
type Addition struct {
	Things []Thing
	Target string
}

// targetMarkers are the words that put a place after the things, longest first so "auf die"
// wins over "auf". A place is not always a list one puts things *on*: a store is something one
// buys *at* ("Gemüsebrühe bei Metro"), a project something one does things *for*, a folder
// something things go *in* - every one of them is a marker.
var targetMarkers = map[Language][]string{
	German: {
		"auf die", "auf den", "auf das", "auf meine", "auf unsere", "in die", "in den", "in das",
		"in meine", "in unsere", "bei der", "bei dem", "für die", "für den", "für das", "fuer die",
		"fuer den", "fuer das", "zur", "zum", "beim", "bei", "zu", "auf", "in", "für", "fuer",
	},
	English: {
		"onto the", "onto my", "on the", "on my", "on our", "to the", "to my", "to our", "into the",
		"in the", "in my", "for the", "for my", "at the", "onto", "into", "to", "on", "for", "at", "in",
	},
}

// targetTails are the words a sentence may end on after its place, which are how it was said and
// not the place: "Gemüsebrühe bei Metro kaufen", "trag das in Büro ein", "add milk to Costco please".
var targetTails = map[Language]map[string]bool{
	German: {
		"kaufen": true, "besorgen": true, "holen": true, "mitbringen": true, "mitnehmen": true,
		"hinzufügen": true, "hinzufuegen": true, "eintragen": true, "setzen": true, "schreiben": true,
		"notieren": true, "aufnehmen": true, "anlegen": true, "packen": true, "erledigen": true,
		"hinzu": true, "ein": true, "an": true, "dazu": true, "bitte": true, "noch": true,
	},
	English: {"please": true, "too": true, "as well": true, "buy": true, "get": true},
}

// withoutTail takes the words after the place off: "Metro kaufen" is the place "Metro".
func withoutTail(target string, language Language) string {
	words := strings.Fields(target)
	for len(words) > 1 && targetTails[language][strings.ToLower(words[len(words)-1])] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, " ")
}

// listWords are what a person adds to a list's name and what the name is without: "die
// Einkaufsliste" is "Einkauf", "the Metro list" is "Metro".
var listWords = map[Language][]string{
	German:  {"liste", "list"},
	English: {"list", "shopping list"},
}

// joiners split several things said in one breath.
var joiners = map[Language][]string{
	German:  {",", " und ", " sowie ", " plus "},
	English: {",", " and ", " plus ", " & "},
}

// ParseAddition reads "zwei Liter Milch, Eier und Brot auf die Metro-Liste".
//
// The last marker with words after it starts the place: "Brot für Oma auf die Einkaufsliste" is
// bread for grandma, to the shopping list. Whether the words after it name a place is the
// resolver's question; one that does not hands the whole sentence back as things
// (WithoutTarget).
func ParseAddition(sentence string, language Language) Addition {
	text := tidy(sentence)
	padded := " " + text + " "
	lower := strings.ToLower(padded)
	at, length := -1, 0
	for _, marker := range targetMarkers[language] {
		needle := " " + marker + " "
		index := strings.LastIndex(lower, needle)
		if index < 0 || strings.TrimSpace(lower[index+len(needle):]) == "" {
			continue
		}
		if index > at || (index == at && len(needle) > length) {
			at, length = index, len(needle)
		}
	}
	if at <= 0 {
		return Addition{Things: splitThings(text, language)}
	}
	return Addition{
		Things: splitThings(strings.TrimSpace(padded[:at]), language),
		Target: withoutTail(strings.TrimSpace(padded[at+length:]), language),
	}
}

// WithoutTarget is the sentence read as things alone, for a place nobody knows.
func WithoutTarget(sentence string, language Language) Addition {
	return Addition{Things: splitThings(tidy(sentence), language)}
}

// TargetWithout is a spoken place with the words for "list" and the articles taken off: "die
// Metro-Liste" → "metro", "Einkaufsliste" → "einkauf".
func TargetWithout(target string, language Language) string {
	words := strings.Fields(Fold(withoutTail(target, language)))
	for len(words) > 0 && isArticle(words[0], language) {
		words = words[1:]
	}
	name := strings.Join(words, " ")
	for _, suffix := range listWords[language] {
		name = strings.TrimSuffix(name, " "+suffix)
		name = strings.TrimSuffix(name, "-"+suffix)
		if name != suffix && strings.HasSuffix(name, suffix) {
			name = strings.TrimSuffix(name, suffix)
			// The joining s of a German compound: "Einkaufsliste" is the list of "Einkauf".
			if language == German && len(name) > 4 && strings.HasSuffix(name, "s") {
				name = strings.TrimSuffix(name, "s")
			}
		}
	}
	return strings.Trim(name, " -")
}

func isArticle(word string, language Language) bool {
	switch language {
	case German:
		switch word {
		case "die", "der", "das", "den", "dem", "meine", "meiner", "meinen", "unsere", "unserer", "unseren":
			return true
		}
	default:
		switch word {
		case "the", "my", "our":
			return true
		}
	}
	return false
}

// splitThings cuts the things apart and reads each one's amount.
func splitThings(text string, language Language) []Thing {
	parts := []string{text}
	for _, joiner := range joiners[language] {
		var next []string
		for _, part := range parts {
			next = append(next, splitFold(part, joiner)...)
		}
		parts = next
	}
	var things []Thing
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		amount, title := amountOf(part, language)
		if title == "" {
			continue
		}
		things = append(things, Thing{Title: upperFirst(title), Amount: amount})
	}
	return things
}

// splitFold splits on a joiner regardless of case.
func splitFold(text, joiner string) []string {
	lower := strings.ToLower(text)
	var parts []string
	for {
		index := strings.Index(lower, joiner)
		if index < 0 {
			return append(parts, text)
		}
		parts = append(parts, text[:index])
		text, lower = text[index+len(joiner):], lower[index+len(joiner):]
	}
}

var numberWords = map[Language]map[string]string{
	German: {"ein": "1", "eine": "1", "einen": "1", "eins": "1", "zwei": "2", "drei": "3", "vier": "4",
		"fünf": "5", "fuenf": "5", "sechs": "6", "sieben": "7", "acht": "8", "neun": "9", "zehn": "10",
		"elf": "11", "zwölf": "12", "zwoelf": "12", "zwanzig": "20", "halbes": "½", "halbe": "½", "halben": "½"},
	English: {"a": "1", "an": "1", "one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
		"six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10", "eleven": "11", "twelve": "12",
		"twenty": "20", "half": "½"},
}

// units maps what is said to how an amount is written.
var units = map[Language]map[string]string{
	German: {"liter": "l", "l": "l", "milliliter": "ml", "ml": "ml", "kilo": "kg", "kilogramm": "kg",
		"kg": "kg", "gramm": "g", "g": "g", "stück": "Stück", "stueck": "Stück", "packung": "Packung",
		"packungen": "Packungen", "flasche": "Flasche", "flaschen": "Flaschen", "dose": "Dose",
		"dosen": "Dosen", "beutel": "Beutel", "glas": "Glas", "gläser": "Gläser", "bund": "Bund",
		"kiste": "Kiste", "kisten": "Kisten", "becher": "Becher", "tüte": "Tüte", "tüten": "Tüten"},
	English: {"litre": "l", "liter": "l", "litres": "l", "liters": "l", "ml": "ml", "kilo": "kg",
		"kilos": "kg", "kg": "kg", "gram": "g", "grams": "g", "g": "g", "pound": "lb", "pounds": "lb",
		"pack": "pack", "packs": "packs", "bottle": "bottle", "bottles": "bottles", "can": "can",
		"cans": "cans", "bag": "bag", "bags": "bags", "jar": "jar", "jars": "jars", "box": "box",
		"boxes": "boxes", "bunch": "bunch", "dozen": "dozen", "pieces": "pcs", "piece": "pc"},
}

// amountOf reads a leading amount: "zwei Liter Milch" → ("2 l", "Milch"), "3 Äpfel" → ("3",
// "Äpfel"), "a bag of rice" → ("1 bag", "rice"). An article alone is no amount: "eine Gurke" is
// a cucumber, not one.
func amountOf(part string, language Language) (string, string) {
	words := strings.Fields(part)
	if len(words) < 2 {
		return "", part
	}
	number := ""
	first := strings.ToLower(words[0])
	if _, err := strconv.ParseFloat(strings.ReplaceAll(first, ",", "."), 64); err == nil {
		number = first
	} else if spoken, ok := numberWords[language][first]; ok {
		number = spoken
	}
	if number == "" {
		return "", part
	}
	rest := words[1:]
	if unit, ok := units[language][strings.ToLower(rest[0])]; ok && len(rest) > 1 {
		rest = rest[1:]
		if language == English && strings.ToLower(rest[0]) == "of" && len(rest) > 1 {
			rest = rest[1:]
		}
		return number + " " + unit, strings.Join(rest, " ")
	}
	// "eine Gurke", "a lemon": an article, not an amount - and not part of the name either.
	if number == "1" && !isDigits(first) {
		return "", strings.Join(rest, " ")
	}
	return number, strings.Join(rest, " ")
}

func isDigits(word string) bool {
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return word != ""
}

// Fold is how two spoken names are compared: lower case, umlauts and ß spelled out, hyphens and
// runs of space one space, no trailing punctuation.
func Fold(text string) string {
	replacer := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "-", " ", "_", " ")
	folded := replacer.Replace(strings.ToLower(text))
	return strings.Join(strings.FieldsFunc(folded, func(r rune) bool {
		return unicode.IsSpace(r) || r == '.' || r == '!' || r == '?' || r == ':' || r == ';'
	}), " ")
}

// tidy trims what an assistant leaves around a sentence.
func tidy(sentence string) string {
	return strings.Trim(strings.Join(strings.Fields(sentence), " "), " .!?")
}

func upperFirst(text string) string {
	for index, r := range text {
		return string(unicode.ToUpper(r)) + text[index+len(string(r)):]
	}
	return text
}

// fromMarkers put a place after a thing being ticked off: "Butter von der Einkaufsliste".
var fromMarkers = map[Language][]string{
	German:  {"von der", "von den", "von dem", "aus der", "aus dem", "auf der", "auf dem", "von", "aus", "auf"},
	English: {"from the", "from my", "off the", "off my", "on the", "on my", "from", "off", "on"},
}

// Pick is "this thing, on that place" - what is ticked off or asked about.
type Pick struct {
	Title  string
	Target string
}

// ParsePick reads "Butter von der Einkaufsliste" or just "Butter".
func ParsePick(sentence string, language Language) Pick {
	text := tidy(sentence)
	padded := " " + text + " "
	lower := strings.ToLower(padded)
	at, length := -1, 0
	for _, marker := range fromMarkers[language] {
		needle := " " + marker + " "
		index := strings.LastIndex(lower, needle)
		if index < 0 || strings.TrimSpace(lower[index+len(needle):]) == "" {
			continue
		}
		if index > at || (index == at && len(needle) > length) {
			at, length = index, len(needle)
		}
	}
	if at <= 0 {
		return Pick{Title: text}
	}
	return Pick{Title: strings.TrimSpace(padded[:at]), Target: strings.TrimSpace(padded[at+length:])}
}
