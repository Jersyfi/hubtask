// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxSoftwareAliases bounds a workspace's alias table.
const MaxSoftwareAliases = 2000

// SoftwareAlias maps what installations report to one publisher and product (research A3): an
// alias matches on the normalised publisher and product; an empty match publisher matches any.
type SoftwareAlias struct {
	ID             shared.ID
	TenantID       shared.ID
	MatchPublisher string
	MatchProduct   string
	Publisher      string
	Product        string
	UpdatedBy      shared.ID
	UpdatedAt      time.Time
	Version        int
}

var (
	spaces         = regexp.MustCompile(`\s+`)
	trailingVer    = regexp.MustCompile(`\s+(v(ersion)?\s*)?\d+([._-]\d+)*[a-z]?$`)
	trailingArch   = regexp.MustCompile(`\s+\(?(x64|x86|64-bit|32-bit|amd64|arm64)\)?$`)
	legalSuffixes  = []string{", inc.", " inc.", " inc", " corporation", " corp.", " corp", " gmbh", " ltd.", " ltd", " llc", " ag", " se", " b.v.", " s.a."}
	nameCharacters = regexp.MustCompile(`[^\p{L}\p{N} .+#_-]`)
)

// NormalPublisher folds a publisher's spelling: lower case, single spaces, without its legal form
// ("Microsoft Corporation" and "microsoft" are one).
func NormalPublisher(p string) string {
	p = strings.ToLower(strings.TrimSpace(spaces.ReplaceAllString(p, " ")))
	for changed := true; changed; {
		changed = false
		for _, s := range legalSuffixes {
			if strings.HasSuffix(p, s) {
				p, changed = strings.TrimSpace(strings.TrimSuffix(p, s)), true
			}
		}
	}
	return strings.TrimRight(p, ",. ")
}

// NormalProduct folds a product's spelling: lower case, single spaces, without a trailing version
// or architecture ("Firefox 128.0 (x64)" is "firefox").
func NormalProduct(p string) string {
	p = strings.ToLower(strings.TrimSpace(spaces.ReplaceAllString(p, " ")))
	p = nameCharacters.ReplaceAllString(p, "")
	for i := 0; i < 3; i++ {
		p = strings.TrimSpace(trailingArch.ReplaceAllString(p, ""))
		p = strings.TrimSpace(trailingVer.ReplaceAllString(p, ""))
	}
	return p
}

// NewSoftwareAlias checks an alias: both match parts are stored normalised, the product is
// required, and the names it maps to are what the inventory shows.
func NewSoftwareAlias(id, tenant, by shared.ID, matchPublisher, matchProduct, publisher, product string, now time.Time) (SoftwareAlias, error) {
	mp, mq := NormalPublisher(matchPublisher), NormalProduct(matchProduct)
	if mq == "" || len(mq) > 200 || len(mp) > 200 {
		return SoftwareAlias{}, invalid("/match_product", "cmdb.alias_match_invalid")
	}
	publisher, product = strings.TrimSpace(publisher), strings.TrimSpace(product)
	if product == "" || len([]rune(product)) > MaxNameLength || len([]rune(publisher)) > MaxNameLength ||
		strings.ContainsAny(product+publisher, "\n\r") {
		return SoftwareAlias{}, invalid("/product", "cmdb.alias_target_invalid")
	}
	return SoftwareAlias{
		ID: id, TenantID: tenant, MatchPublisher: mp, MatchProduct: mq, Publisher: publisher, Product: product,
		UpdatedBy: by, UpdatedAt: now, Version: 1,
	}, nil
}

// Product is a normalised software product of a hub with its versions and how often each is
// installed.
type Product struct {
	Publisher string
	Product   string
	// Aliased says an alias named it, not the folding alone.
	Aliased  bool
	Versions []ProductVersion
	CIs      []shared.ID
}

// ProductVersion is one version of a product and its installations.
type ProductVersion struct {
	Version       string
	CIs           int
	Installations int
}

// Normalise groups a hub's software CIs by product: an alias matching the folded publisher and
// product wins - one with its publisher before one for any -, the folded names otherwise. The
// installations of a CI are counted by the caller from its `installed_on` relations.
func Normalise(cis []CI, classes Classes, aliases []SoftwareAlias, installations map[shared.ID]int) []Product {
	byMatch := map[[2]string]SoftwareAlias{}
	for _, a := range aliases {
		byMatch[[2]string{a.MatchPublisher, a.MatchProduct}] = a
	}
	type key struct{ publisher, product string }
	groups := map[key]*Product{}
	var order []key
	for _, ci := range cis {
		if ci.Status == StatusMerged || !classes.IsA(ci.Class, "software") {
			continue
		}
		rawPublisher, _ := ci.Attributes["publisher"].(string)
		version, _ := ci.Attributes["version"].(string)
		pub, prod := NormalPublisher(rawPublisher), NormalProduct(ci.Name)
		k, aliased := key{pub, prod}, false
		if a, ok := byMatch[[2]string{pub, prod}]; ok {
			k, aliased = key{a.Publisher, a.Product}, true
		} else if a, ok := byMatch[[2]string{"", prod}]; ok {
			k, aliased = key{a.Publisher, a.Product}, true
		}
		p, ok := groups[k]
		if !ok {
			p = &Product{Publisher: k.publisher, Product: k.product}
			groups[k] = p
			order = append(order, k)
		}
		p.Aliased = p.Aliased || aliased
		p.CIs = append(p.CIs, ci.ID)
		version = strings.TrimSpace(version)
		at := slices.IndexFunc(p.Versions, func(v ProductVersion) bool { return v.Version == version })
		if at < 0 {
			p.Versions = append(p.Versions, ProductVersion{Version: version})
			at = len(p.Versions) - 1
		}
		p.Versions[at].CIs++
		p.Versions[at].Installations += installations[ci.ID]
	}
	out := make([]Product, 0, len(order))
	for _, k := range order {
		p := *groups[k]
		slices.SortFunc(p.Versions, func(a, b ProductVersion) int { return strings.Compare(b.Version, a.Version) })
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b Product) int {
		if c := strings.Compare(a.Publisher, b.Publisher); c != 0 {
			return c
		}
		return strings.Compare(a.Product, b.Product)
	})
	return out
}
