// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"context"
	"strconv"
	"strings"
	"time"

	forgeservice "github.com/Jersyfi/hubtask/core/application/service/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CodeDeliverer is the application's side of a forge hook.
type CodeDeliverer interface {
	Deliver(ctx context.Context, d forgeservice.Delivery, parse forgeservice.Parser) (forgeservice.Receipt, error)
}

// CodeIntake translates a forge's webhook - GitHub's, GitLab's, Forgejo's or Gitea's - into
// signals (UC-INT-13). Which entry a key names, and whether the delivery is trusted, is decided
// inwards; this only reads the forge's format.
type CodeIntake struct {
	Deliveries CodeDeliverer
}

// Deliver hands the delivery on with the forges' parser.
func (i CodeIntake) Deliver(ctx context.Context, d forgeservice.Delivery) (forgeservice.Receipt, error) {
	return i.Deliveries.Deliver(ctx, d, ParseCode)
}

func codeMalformed() error { return shared.ErrMalformedRequest.WithDetail("forge.delivery_malformed") }

// ParseCode reads a delivery by its forge and event. An event the intake does not read - a ping, a
// tag, an issue - answers no signal, so that a forge's test delivery succeeds.
func ParseCode(kind forge.Kind, event string, body []byte) ([]forge.Signal, error) {
	event = strings.ToLower(strings.TrimSpace(event))
	switch kind {
	case forge.GitHub, forge.Forgejo:
		switch event {
		case "push":
			return hubPush(body)
		case "pull_request":
			return hubPull(body)
		}
		return nil, nil
	case forge.GitLab:
		switch event {
		case "push hook":
			return labPush(body)
		case "merge request hook":
			return labMerge(body)
		}
		return nil, nil
	}
	return nil, codeMalformed()
}

func branchOf(ref string) (string, bool) {
	return strings.CutPrefix(ref, "refs/heads/")
}

// zeroCommit is what a push deleting its branch names as the commit after it.
func zeroCommit(sha string) bool { return sha != "" && strings.Trim(sha, "0") == "" }

type hubCommit struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	URL       string `json:"url"`
	Timestamp string `json:"timestamp"`
	Author    struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"author"`
}

func commits(repo string, list []hubCommit) []forge.Signal {
	out := make([]forge.Signal, 0, len(list))
	for _, c := range list {
		author := c.Author.Username
		if author == "" {
			author = c.Author.Name
		}
		out = append(out, forge.Signal{
			Kind: forge.Commit, Repository: repo, Ref: c.ID, Title: c.Message, URL: c.URL, Author: author,
			State: forge.Pushed, At: when(c.Timestamp), Texts: []string{c.Message},
		})
	}
	return out
}

// hubPush reads GitHub's push, which Forgejo and Gitea send in the same shape: the branch, and each
// commit by its message.
func hubPush(body []byte) ([]forge.Signal, error) {
	var doc struct {
		Ref        string      `json:"ref"`
		After      string      `json:"after"`
		Deleted    bool        `json:"deleted"`
		Commits    []hubCommit `json:"commits"`
		Repository struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		Pusher struct {
			Name     string `json:"name"`
			Login    string `json:"login"`
			Username string `json:"username"`
		} `json:"pusher"`
	}
	if err := decodeInto(body, &doc); err != nil {
		return nil, codeMalformed()
	}
	repo := doc.Repository.FullName
	out := commits(repo, doc.Commits)
	if branch, ok := branchOf(doc.Ref); ok {
		state := forge.Open
		if doc.Deleted || zeroCommit(doc.After) {
			state = forge.Closed
		}
		url := ""
		if doc.Repository.HTMLURL != "" {
			url = strings.TrimSuffix(doc.Repository.HTMLURL, "/") + "/tree/" + branch
		}
		pusher := firstNonEmpty(doc.Pusher.Login, doc.Pusher.Username, doc.Pusher.Name)
		out = append(out, forge.Signal{
			Kind: forge.Branch, Repository: repo, Ref: branch, Title: branch, URL: url, Author: pusher,
			State: state, Texts: []string{branch},
		})
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// hubPull reads GitHub's pull_request, and Forgejo's and Gitea's in the same shape.
func hubPull(body []byte) ([]forge.Signal, error) {
	var doc struct {
		PullRequest struct {
			Number    int    `json:"number"`
			Title     string `json:"title"`
			Body      string `json:"body"`
			HTMLURL   string `json:"html_url"`
			State     string `json:"state"`
			Merged    bool   `json:"merged"`
			UpdatedAt string `json:"updated_at"`
			Head      struct {
				Ref string `json:"ref"`
			} `json:"head"`
			User struct {
				Login    string `json:"login"`
				Username string `json:"username"`
			} `json:"user"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := decodeInto(body, &doc); err != nil || doc.PullRequest.Number == 0 {
		return nil, codeMalformed()
	}
	pr := doc.PullRequest
	state := forge.Open
	switch {
	case pr.Merged:
		state = forge.Merged
	case strings.EqualFold(pr.State, "closed"):
		state = forge.Closed
	}
	return []forge.Signal{{
		Kind: forge.PullRequest, Repository: doc.Repository.FullName, Ref: strconv.Itoa(pr.Number), Title: pr.Title,
		URL: pr.HTMLURL, Author: firstNonEmpty(pr.User.Login, pr.User.Username), State: state, At: when(pr.UpdatedAt),
		Texts: []string{pr.Title, pr.Head.Ref, pr.Body},
	}}, nil
}

// labPush reads GitLab's push hook.
func labPush(body []byte) ([]forge.Signal, error) {
	var doc struct {
		Ref          string      `json:"ref"`
		After        string      `json:"after"`
		UserUsername string      `json:"user_username"`
		Commits      []hubCommit `json:"commits"`
		Project      struct {
			PathWithNamespace string `json:"path_with_namespace"`
			WebURL            string `json:"web_url"`
		} `json:"project"`
	}
	if err := decodeInto(body, &doc); err != nil {
		return nil, codeMalformed()
	}
	repo := doc.Project.PathWithNamespace
	out := commits(repo, doc.Commits)
	if branch, ok := branchOf(doc.Ref); ok {
		state := forge.Open
		if zeroCommit(doc.After) {
			state = forge.Closed
		}
		url := ""
		if doc.Project.WebURL != "" {
			url = strings.TrimSuffix(doc.Project.WebURL, "/") + "/-/tree/" + branch
		}
		out = append(out, forge.Signal{
			Kind: forge.Branch, Repository: repo, Ref: branch, Title: branch, URL: url, Author: doc.UserUsername,
			State: state, Texts: []string{branch},
		})
	}
	return out, nil
}

// labMerge reads GitLab's merge request hook.
func labMerge(body []byte) ([]forge.Signal, error) {
	var doc struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Project struct {
			PathWithNamespace string `json:"path_with_namespace"`
		} `json:"project"`
		Attrs struct {
			IID          int    `json:"iid"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			URL          string `json:"url"`
			State        string `json:"state"`
			SourceBranch string `json:"source_branch"`
			UpdatedAt    string `json:"updated_at"`
		} `json:"object_attributes"`
	}
	if err := decodeInto(body, &doc); err != nil || doc.Attrs.IID == 0 {
		return nil, codeMalformed()
	}
	a := doc.Attrs
	state := forge.Open
	switch strings.ToLower(a.State) {
	case "merged":
		state = forge.Merged
	case "closed":
		state = forge.Closed
	}
	return []forge.Signal{{
		Kind: forge.PullRequest, Repository: doc.Project.PathWithNamespace, Ref: strconv.Itoa(a.IID), Title: a.Title,
		URL: a.URL, Author: doc.User.Username, State: state, At: labTime(a.UpdatedAt),
		Texts: []string{a.Title, a.SourceBranch, a.Description},
	}}, nil
}

// labTime reads GitLab's moments, RFC 3339 in newer versions and "2013-12-03 17:23:34 UTC" in
// older ones.
func labTime(v string) *time.Time {
	if t := when(v); t != nil {
		return t
	}
	t, err := time.Parse("2006-01-02 15:04:05 MST", v)
	if err != nil || t.Year() < 2000 {
		return nil
	}
	return &t
}
