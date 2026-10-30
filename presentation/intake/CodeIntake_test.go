// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/forge"
)

// UC-INT-13 check 2: each forge's push and pull request is read with what names a key.
func TestParseCodeReadsEachForgesPushAndPullRequest(t *testing.T) {
	githubPush := `{"ref":"refs/heads/feature/WEB-42-login","after":"abc","repository":{"full_name":"team/web",
		"html_url":"https://github.example/team/web"},"pusher":{"name":"ada"},
		"commits":[{"id":"abc","message":"WEB-42 the form\n\nmore","url":"https://github.example/c/abc",
		"timestamp":"2026-10-30T09:00:00+01:00","author":{"name":"Ada","username":"ada"}}]}`
	sigs, err := ParseCode(forge.GitHub, "push", []byte(githubPush))
	if err != nil || len(sigs) != 2 {
		t.Fatalf("push = %+v, %v", sigs, err)
	}
	if sigs[0].Kind != forge.Commit || sigs[0].Ref != "abc" || sigs[0].At == nil || sigs[0].Author != "ada" {
		t.Fatalf("commit = %+v", sigs[0])
	}
	if b := sigs[1]; b.Kind != forge.Branch || b.Ref != "feature/WEB-42-login" || b.State != forge.Open ||
		b.URL != "https://github.example/team/web/tree/feature/WEB-42-login" {
		t.Fatalf("branch = %+v", b)
	}
	deleted, _ := ParseCode(forge.Forgejo, "push", []byte(`{"ref":"refs/heads/WEB-42","after":"0000000000000000000000000000000000000000","repository":{"full_name":"r"}}`))
	if len(deleted) != 1 || deleted[0].State != forge.Closed {
		t.Fatalf("deleted branch = %+v", deleted)
	}
	pull := `{"action":"closed","pull_request":{"number":7,"title":"Login","state":"closed","merged":true,
		"html_url":"https://github.example/team/web/pull/7","head":{"ref":"WEB-42-login"},"user":{"login":"ada"},
		"updated_at":"2026-10-30T10:00:00Z"},"repository":{"full_name":"team/web"}}`
	prs, err := ParseCode(forge.GitHub, "pull_request", []byte(pull))
	if err != nil || len(prs) != 1 || prs[0].State != forge.Merged || prs[0].Ref != "7" || len(forge.KeysIn(prs[0].Texts...)) != 1 {
		t.Fatalf("pull request = %+v, %v", prs, err)
	}
	mr := `{"user":{"username":"bo"},"project":{"path_with_namespace":"team/web"},"object_attributes":{"iid":3,
		"title":"Fix","description":"Closes WEB-42","url":"https://gitlab.example/team/web/-/merge_requests/3",
		"state":"opened","source_branch":"fix","updated_at":"2026-10-30 10:00:00 UTC"}}`
	mrs, err := ParseCode(forge.GitLab, "Merge Request Hook", []byte(mr))
	if err != nil || len(mrs) != 1 || mrs[0].State != forge.Open || mrs[0].At == nil || mrs[0].Author != "bo" {
		t.Fatalf("merge request = %+v, %v", mrs, err)
	}
	lab := `{"ref":"refs/heads/main","after":"def","user_username":"bo","project":{"path_with_namespace":"team/web",
		"web_url":"https://gitlab.example/team/web"},"commits":[{"id":"def","message":"WEB-42","url":"u","timestamp":"2026-10-30T09:00:00Z","author":{"name":"Bo"}}]}`
	pushed, err := ParseCode(forge.GitLab, "Push Hook", []byte(lab))
	if err != nil || len(pushed) != 2 || pushed[1].URL != "https://gitlab.example/team/web/-/tree/main" || pushed[0].Author != "Bo" {
		t.Fatalf("gitlab push = %+v, %v", pushed, err)
	}
	if none, err := ParseCode(forge.GitHub, "ping", []byte(`{}`)); err != nil || len(none) != 0 {
		t.Fatalf("a ping = %+v, %v", none, err)
	}
	if _, err := ParseCode(forge.GitHub, "pull_request", []byte(`not json`)); err == nil {
		t.Fatal("a broken body was read")
	}
}
