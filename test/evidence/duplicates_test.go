// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build evidence

// Package evidence holds the runs that cannot happen in a pipeline: measurements against a model
// somebody has to have running, recorded once into docs/evidence/ (its README says why). Nothing
// here is a gate. A test in this package skips, loudly, when what it measures is not reachable.
package evidence

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// The measurement #532 asks for: where the similarity bands of paraphrases, of entries that merely
// share a subject, and of unrelated entries sit for the embedding models this product stores, so
// that `suggestion.DefaultDuplicateFloor` is a measured number rather than an argued one.
//
// What it drives is the product's own Ollama adapter, over the product's own guarded client, on
// the text shape the product embeds (`work.embeddingText`: the title, or the title and the notes
// with a blank line between). What it computes is cosine similarity in exact arithmetic - the
// number `1 - (a <=> b)` in EmbeddingRepository.go is - over every pair in the corpus. What it does
// **not** do is run the product's retrieval: no LIMIT, no subtree exclusion, no permission
// narrowing, no HNSW approximation. The document that reads this run says so.
//
// Two halves, decided by the group's name and not by anything measured: the floor is chosen on one
// and reported on the other, so the number is not an optimum read off the data it is then judged
// by.

const (
	endpointVar = "HUBTASK_EVIDENCE_OLLAMA_URL"
	modelsVar   = "HUBTASK_EVIDENCE_MODELS"
	outVar      = "HUBTASK_EVIDENCE_OUT"

	defaultModels = "nomic-embed-text,mxbai-embed-large"
)

type item struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	Role  string `json:"role"`
	Lang  string `json:"lang"`
	Title string `json:"title"`
	Notes string `json:"notes"`
	Twin  string `json:"twin,omitempty"`
}

type corpus struct {
	Items []item `json:"items"`
}

// class is what two items are to each other, derived from the labels and never from a score.
func class(a, b item) string {
	if a.Group != b.Group {
		return "unrelated"
	}
	if a.Role == "namesake" || b.Role == "namesake" {
		return "namesake"
	}
	same := map[string]bool{"anchor": true, "duplicate": true, "duplicate-xl": true, "near-identical": true}
	if same[a.Role] && same[b.Role] || a.Twin == b.ID || b.Twin == a.ID {
		if a.Lang != b.Lang {
			return "duplicate-xl"
		}
		return "duplicate"
	}
	return "subject"
}

// half is the held-out split: tuning or reporting, by the group's name.
func half(group string) string {
	var sum int
	for _, r := range group {
		sum += int(r)
	}
	if sum%2 == 0 {
		return "tuning"
	}
	return "reporting"
}

// embeddingText is the product's shape, restated: work.embeddingText is unexported, and the two
// lines it is are worth copying rather than exporting a function for a measurement.
func embeddingText(i item) string {
	if i.Notes == "" {
		return i.Title
	}
	return i.Title + "\n\n" + i.Notes
}

func TestWhereTheDuplicateBandsSit(t *testing.T) {
	endpoint := os.Getenv(endpointVar)
	if endpoint == "" {
		t.Skipf("%s is not set: this is a measurement against a running model, not a gate", endpointVar)
	}
	// The product's own client. The guard is told to allow a private address, which no default
	// installation does: an operator who points a workspace at localhost gets the same refusal a
	// webhook to 10.0.0.1 gets (ADR-0015, T-07). That is the one configuration this run differs
	// from a production one in, and the document says so.
	cfg := env.OutboundConfig{
		Timeout: 5 * time.Minute, ConnectTimeout: 5 * time.Second,
		MaxResponseBytes: 64 << 20, MaxRedirects: 0, AllowPrivateNetworks: true,
	}
	client := httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))
	if err := reachable(client, endpoint+"/api/version"); err != nil {
		t.Skipf("nothing answers at %s: %v", endpoint, err)
	}
	out := os.Getenv(outVar)
	if out == "" {
		out = t.TempDir()
	}

	var body corpus
	raw, err := os.ReadFile(filepath.Join("corpus", "duplicates.json"))
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("parsing the corpus: %v", err)
	}
	items := body.Items
	t.Logf("%d items, %d groups", len(items), groups(items))

	models := strings.Split(os.Getenv(modelsVar), ",")
	if os.Getenv(modelsVar) == "" {
		models = strings.Split(defaultModels, ",")
	}

	report := map[string]any{
		"ran_at":      time.Now().UTC().Format(time.RFC3339),
		"endpoint":    endpoint,
		"ollama":      versionOf(t, client, endpoint),
		"corpus":      map[string]any{"items": len(items), "groups": groups(items)},
		"embedded_as": "title, or title + blank line + notes (work.embeddingText)",
		"similarity":  "cosine, exact arithmetic over the raw vectors; no pgvector, no HNSW, no LIMIT, no exclusions",
		"percentiles": "nearest-rank",
		"models":      map[string]any{},
	}

	for _, model := range models {
		model = strings.TrimSpace(model)
		provider := ai.Ollama{
			Client: client, Clock: clock.Fixed(time.Now()), BaseURL: endpoint, EmbeddingModel: model,
		}
		texts := make([]string, len(items))
		for i, it := range items {
			texts[i] = embeddingText(it)
		}
		started := time.Now()
		answer, err := provider.Embed(context.Background(), texts)
		if err != nil {
			t.Fatalf("%s: embedding the corpus: %v", model, err)
		}
		if len(answer.Vectors) != len(items) {
			t.Fatalf("%s answered %d vectors for %d items", model, len(answer.Vectors), len(items))
		}
		t.Logf("%s: %d vectors of %d dimensions in %s", model, len(answer.Vectors), answer.Dimensions, time.Since(started).Round(time.Second))

		report["models"].(map[string]any)[model] = measure(t, client, endpoint, model, items, answer.Vectors, answer.Dimensions)
	}

	encoded, _ := json.MarshalIndent(report, "", " ")
	path := filepath.Join(out, "duplicate-threshold.json")
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("writing the report: %v", err)
	}
	t.Logf("report written to %s", path)
}

type scored struct {
	class, half, a, b string
	similarity        float64
	titleOnly         bool
	crossLingual      bool
}

func measure(t *testing.T, client httpport.Port, endpoint, model string, items []item, vectors [][]float32, dimensions int) map[string]any {
	t.Helper()
	// Norms are recorded rather than assumed: cosine hides normalisation, and a future reader
	// comparing against a dot-product implementation will want to know.
	var minNorm, maxNorm = math.Inf(1), 0.0
	for _, v := range vectors {
		n := norm(v)
		minNorm, maxNorm = math.Min(minNorm, n), math.Max(maxNorm, n)
	}

	var pairs []scored
	byID := map[string]int{}
	for i, it := range items {
		byID[it.ID] = i
	}
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			a, b := items[i], items[j]
			pairs = append(pairs, scored{
				class: class(a, b), half: half(a.Group), a: a.ID, b: b.ID,
				similarity:   cosine(vectors[i], vectors[j]),
				titleOnly:    a.Notes == "" && b.Notes == "",
				crossLingual: a.Lang != b.Lang,
			})
		}
	}

	// Per item: its true duplicates' similarities, and the highest similarity to anything that is
	// not one. The floor lives between those two distributions, and nowhere else - a pairwise
	// histogram of classes cannot say what one entry's inbox would show.
	// Same-language duplicates and cross-lingual ones are kept apart from here on. They are two
	// different questions in the issue (its first and its fourth), and folding a translation's
	// similarity into "an item's lowest duplicate" would let the answer to the fourth hide the
	// answer to the first.
	type perItem struct {
		id, half     string
		duplicates   []float64
		translations []float64
		maxNonDup    float64
		maxNonDupID  string
		maxNonDupCls string
	}
	per := map[string]*perItem{}
	for _, it := range items {
		per[it.ID] = &perItem{id: it.ID, half: half(it.Group)}
	}
	for _, p := range pairs {
		for _, side := range [][2]string{{p.a, p.b}, {p.b, p.a}} {
			me := per[side[0]]
			if p.class == "duplicate" {
				me.duplicates = append(me.duplicates, p.similarity)
			} else if p.class == "duplicate-xl" {
				me.translations = append(me.translations, p.similarity)
			} else if p.similarity > me.maxNonDup {
				me.maxNonDup, me.maxNonDupID, me.maxNonDupCls = p.similarity, side[1], p.class
			}
		}
	}

	result := map[string]any{
		"dimensions": dimensions,
		"norm":       map[string]float64{"min": round(minNorm), "max": round(maxNorm)},
		"digest":     digestOf(t, client, endpoint, model),
	}

	// Band summaries: per class, per half, and the strata the document needs.
	bands := map[string]any{}
	for _, c := range []string{"duplicate", "duplicate-xl", "subject", "namesake", "unrelated"} {
		var all, titleOnly, withNotes []float64
		for _, p := range pairs {
			if p.class != c {
				continue
			}
			all = append(all, p.similarity)
			if p.titleOnly {
				titleOnly = append(titleOnly, p.similarity)
			} else {
				withNotes = append(withNotes, p.similarity)
			}
		}
		bands[c] = map[string]any{
			"all": summary(all), "title_only": summary(titleOnly), "with_notes": summary(withNotes),
		}
	}
	result["bands"] = bands

	// The per-item view, by half: the duplicate band's low end against the non-duplicate band's
	// high end, which is the gap a floor has to fit into.
	perHalf := map[string]any{}
	for _, h := range []string{"tuning", "reporting"} {
		var dupLows, xlLows, nonDupHighs []float64
		var offenders []map[string]any
		for _, it := range items {
			me := per[it.ID]
			if me.half != h {
				continue
			}
			if len(me.duplicates) > 0 {
				dupLows = append(dupLows, minOf(me.duplicates))
			}
			if len(me.translations) > 0 {
				xlLows = append(xlLows, minOf(me.translations))
			}
			nonDupHighs = append(nonDupHighs, me.maxNonDup)
		}
		sort.Float64s(nonDupHighs)
		// The highest non-duplicates, named, so the document can say what they are.
		for _, it := range items {
			me := per[it.ID]
			if me.half == h && me.maxNonDup >= percentile(nonDupHighs, 95) {
				offenders = append(offenders, map[string]any{
					"item": me.id, "nearest": me.maxNonDupID, "class": me.maxNonDupCls,
					"similarity": round(me.maxNonDup),
				})
			}
		}
		sort.Slice(offenders, func(i, j int) bool {
			return offenders[i]["similarity"].(float64) > offenders[j]["similarity"].(float64)
		})
		perHalf[h] = map[string]any{
			"items":                       len(nonDupHighs),
			"lowest_duplicate_per_item":   summary(dupLows),
			"lowest_translation_per_item": summary(xlLows),
			"highest_non_duplicate":       summary(nonDupHighs),
			"highest_non_duplicates":      offenders,
		}
	}
	result["per_item"] = perHalf

	// The curve: for each candidate floor, on each half, how many of the items with a duplicate
	// would have it proposed, and how many items would have something proposed that is not one.
	// Counts of items rather than a precision - a precision over a corpus whose prevalence is
	// nothing like a workspace's would mislead by orders of magnitude.
	var curve []map[string]any
	for floor := 0.60; floor <= 0.985; floor += 0.025 {
		row := map[string]any{"floor": round(floor)}
		for _, h := range []string{"tuning", "reporting"} {
			withDup, found, withXl, foundXl, falseOnes, total := 0, 0, 0, 0, 0, 0
			for _, it := range items {
				me := per[it.ID]
				if me.half != h {
					continue
				}
				total++
				if len(me.duplicates) > 0 {
					withDup++
					if minOf(me.duplicates) >= floor {
						found++
					}
				}
				if len(me.translations) > 0 {
					withXl++
					if minOf(me.translations) >= floor {
						foundXl++
					}
				}
				if me.maxNonDup >= floor {
					falseOnes++
				}
			}
			row[h] = map[string]any{
				"items": total, "with_a_duplicate": withDup, "all_duplicates_found": found,
				"with_a_translation": withXl, "all_translations_found": foundXl,
				"a_non_duplicate_proposed": falseOnes,
			}
		}
		curve = append(curve, row)
	}
	result["curve"] = curve

	// The rule for choosing, stated before the numbers were seen: the midpoint between the 5th
	// percentile of an item's lowest true duplicate and the 95th percentile of its highest
	// non-duplicate, on the tuning half. Reported on the other half by the curve above.
	var dupLows, nonDupHighs []float64
	for _, it := range items {
		me := per[it.ID]
		if me.half != "tuning" {
			continue
		}
		if len(me.duplicates) > 0 {
			dupLows = append(dupLows, minOf(me.duplicates))
		}
		nonDupHighs = append(nonDupHighs, me.maxNonDup)
	}
	low, high := percentile(dupLows, 5), percentile(nonDupHighs, 95)
	result["chosen_on_tuning_half"] = map[string]any{
		"duplicate_p5": round(low), "non_duplicate_p95": round(high),
		"midpoint": round((low + high) / 2), "bands_overlap": high > low,
	}
	return result
}

func summary(values []float64) map[string]any {
	if len(values) == 0 {
		return map[string]any{"n": 0}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return map[string]any{
		"n": len(sorted), "min": round(sorted[0]), "p5": round(percentile(sorted, 5)),
		"median": round(percentile(sorted, 50)), "p95": round(percentile(sorted, 95)),
		"max": round(sorted[len(sorted)-1]),
	}
}

// percentile is nearest-rank over an ascending slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	rank := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if rank < 0 {
		rank = 0
	}
	if rank >= len(sorted) {
		rank = len(sorted) - 1
	}
	return sorted[rank]
}

func minOf(values []float64) float64 {
	m := values[0]
	for _, v := range values[1:] {
		m = math.Min(m, v)
	}
	return m
}

func round(f float64) float64 { return math.Round(f*10000) / 10000 }

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func norm(v []float32) float64 {
	var n float64
	for _, x := range v {
		n += float64(x) * float64(x)
	}
	return math.Sqrt(n)
}

func groups(items []item) int {
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Group] = true
	}
	return len(seen)
}

// versionOf and digestOf read what the document has to record for the run to mean anything in six
// months: a tag like `latest` moves, and a digest does not.
func versionOf(t *testing.T, client httpport.Port, endpoint string) string {
	t.Helper()
	var v struct {
		Version string `json:"version"`
	}
	fetch(t, client, endpoint+"/api/version", &v)
	return v.Version
}

func digestOf(t *testing.T, client httpport.Port, endpoint, model string) map[string]any {
	t.Helper()
	var tags struct {
		Models []struct {
			Name    string `json:"name"`
			Digest  string `json:"digest"`
			Details struct {
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
				Family            string `json:"family"`
			} `json:"details"`
		} `json:"models"`
	}
	fetch(t, client, endpoint+"/api/tags", &tags)
	for _, m := range tags.Models {
		if m.Name == model || m.Name == model+":latest" {
			return map[string]any{
				"tag": m.Name, "digest": m.Digest, "parameters": m.Details.ParameterSize,
				"quantisation": m.Details.QuantizationLevel, "family": m.Details.Family,
			}
		}
	}
	t.Fatalf("%s is not among the models %s serves", model, endpoint)
	return nil
}

// reachable and fetch read Ollama's own metadata endpoints, through the same guarded client the
// adapter uses (rule 6): what the run records about the model came over the product's own path.
func reachable(client httpport.Port, url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Do(ctx, httpport.Request{Method: http.MethodGet, URL: url, TargetClass: "ai"})
	return err
}

func fetch(t *testing.T, client httpport.Port, url string, into any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := client.Do(ctx, httpport.Request{Method: http.MethodGet, URL: url, TargetClass: "ai"})
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	if err := json.Unmarshal(response.Body, into); err != nil {
		t.Fatalf("decoding %s: %v", url, err)
	}
}
