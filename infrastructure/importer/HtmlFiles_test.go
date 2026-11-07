// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"context"
	"strings"
	"testing"
)

// UC-IMP-05 check 7: a notes app's HTML export becomes the hub's pages - the title from the
// file, the heading repeating it dropped, a link between files a page reference, a picture in the
// ZIP the page's file - and one HTML file on its own becomes one page.
func TestANotesAppsHTMLExportBecomesTheHubsPages(t *testing.T) {
	result, err := HTMLFiles{}.Convert(context.Background(), trackerSource(t, markdownZip(t, map[string]string{
		"Team 0123456789abcdef0123456789abcdef.html": `<html><head><title>Team</title></head><body><article>
			<header><h1 class="page-title">Team</h1></header>
			<p>See <a href="Team%200123456789abcdef0123456789abcdef/On%20call.html">on call</a> and <a href="https://example.org">the web</a>.</p>
			</article></body></html>`,
		"Team 0123456789abcdef0123456789abcdef/On call.html": `<html><body><h2>Rota</h2><ul><li>Anna</li><li>Ben</li></ul>
			<img src="On%20call/rota.png"><img src="https://tracker.example/pixel.gif"></body></html>`,
		"Team 0123456789abcdef0123456789abcdef/On call/rota.png": "\x89PNG\r\n\x1a\npng bytes",
	})))
	if err != nil {
		t.Fatal(err)
	}
	team, call := titled(result.Records["pages"], "Team"), titled(result.Records["pages"], "On call")
	if team == nil || call == nil || call["parent_id"] != team["id"] {
		t.Fatalf("pages: %v", result.Records["pages"])
	}
	body := team["body"].(string)
	if body != "See [on call](hubtask:page/"+call["id"].(string)+") and [the web](https://example.org)." {
		t.Errorf("the team page is %q", body)
	}
	rota := call["body"].(string)
	if !strings.Contains(rota, "## Rota") || !strings.Contains(rota, "- Anna\n- Ben") || !strings.Contains(rota, "![](hubtask:media/") ||
		strings.Contains(rota, "tracker.example") {
		t.Errorf("the on-call page is %q", rota)
	}

	source := trackerSource(t, []byte("<!DOCTYPE html><html><body><p>alone</p></body></html>"))
	source.Name = "Notes"
	single, err := HTMLFiles{}.Convert(context.Background(), source)
	if err != nil || titled(single.Records["pages"], "Notes") == nil {
		t.Errorf("one file: %v %v", single.Records["pages"], err)
	}
}
