// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The use cases the shipped agents call. Named here for the suggestion package's reason: what a
// shipped agent can do to a workspace is exactly what it can name.
const (
	getWorkItemName        = "GetWorkItem"
	getContainerName       = "GetContainer"
	listCisName            = "ListCis"
	readCiName             = "ReadCi"
	listCiDuplicatesName   = "ListCiDuplicates"
	addCommentName         = "AddComment"
	resolveCiDuplicateName = "ResolveCiDuplicate"
	createPageName         = "CreatePage"

	// runAgentJobName is the ledger's name for the tokens a job spent outside an act.
	runAgentJobName = "RunAgentJob"

	// minCiName keeps a CI whose name is one or two letters from matching every ticket.
	minCiName = 3
)

// Proposer asks the workspace's AI to propose labels for an entry as the agent, recorded as a
// suggestion a person accepts - never applied (the suggestion module's Produce).
type Proposer interface {
	Classify(ctx context.Context, actor appshared.ActorContext, item shared.ID) (Proposal, error)
}

// Proposal is what one question to the AI came to: whether a suggestion was recorded, the outcome
// code, and the tokens it spent.
type Proposal struct {
	Recorded bool
	Code     string
	Tokens   int64
}

// Runner runs one job of a shipped agent (G11). It acts as the agent's own account through the
// catalogue, so every act passes the gate (UC-AGT-02): a read runs, and every write a shipped agent
// attempts is held for a person (Admit), so a job proposes and never applies. The labels the
// triage proposes are a suggestion on the entry.
type Runner struct {
	Desk     Desk
	Proposer Proposer
}

// Run runs a job and ends it; a job that ended already is left as it is.
func (r Runner) Run(ctx context.Context, tenant, jobID shared.ID) (domain.Job, error) {
	d := r.Desk
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenant}
	var job domain.Job
	var a domain.Agent
	var judged error
	err := d.UnitOfWork.WithinReadOnly(ctx, system.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if job, err = d.Store.FindJob(ctx, jobID); err != nil {
			return err
		}
		if a, err = d.Store.FindAgent(ctx, job.AgentID); err != nil {
			return err
		}
		situation, err := d.situation(ctx, a)
		if err != nil {
			return err
		}
		// The job itself is one of the runs counted; the switches, the model and the tokens may
		// have changed since it was started.
		situation.Usage.RunsNow = 0
		judged = a.MayStart(job.TargetType, situation)
		return nil
	})
	if err != nil {
		return domain.Job{}, err
	}
	if job.Status != domain.JobRunning {
		return job, nil
	}
	if judged != nil {
		return r.end(ctx, system, job, domain.JobFailed, shared.AsError(judged).DetailCode, 0, 0)
	}

	actor := Gate{}.narrowed(appshared.ActorContext{
		Kind: appshared.ActorAIAgent, TenantID: tenant, AccountID: a.AccountID, AccountName: a.Name, Scopes: a.Scopes,
	}, a)
	work := jobWork{runner: r, actor: actor, agent: a}
	switch a.Kind {
	case domain.KindTicketTriage:
		err = work.triage(ctx, job.TargetID)
	case domain.KindCISuggestion:
		_, err = work.suggestCis(ctx, job.TargetID)
	case domain.KindDuplicateReview:
		err = work.reviewDuplicates(ctx, job.TargetID)
	case domain.KindKnowledgeDraft:
		err = work.draftKnowledge(ctx, job.TargetID)
	}
	if err != nil {
		return r.end(ctx, system, job, domain.JobFailed, codeOf(err), work.proposals, work.tokens)
	}
	code := work.code
	if code == "" {
		code = OutcomeJobNothing
		if work.proposals > 0 {
			code = OutcomeJobProposed
		}
	}
	return r.end(ctx, system, job, domain.JobDone, code, work.proposals, work.tokens)
}

// end writes a job's end and the tokens it spent outside an act into the ledger, in one
// transaction, so the day's tokens and the job agree.
func (r Runner) end(
	ctx context.Context, system appshared.ActorContext, job domain.Job, status domain.JobStatus, code string, proposals int, tokens int64,
) (domain.Job, error) {
	d := r.Desk
	now := d.Clock.Now()
	ended := job.Ended(status, code, proposals, tokens, now)
	err := d.UnitOfWork.Within(ctx, system.PersistenceScope(), func(ctx context.Context) error {
		if done, err := d.Store.EndJob(ctx, ended); err != nil || !done {
			return err
		}
		if tokens == 0 {
			return nil
		}
		return d.Store.RecordAction(ctx, domain.Action{
			ID: d.IDs.NewID(), AgentID: job.AgentID, UseCase: runAgentJobName, Class: domain.ClassRead,
			Decision: domain.DecisionAllowed, Tokens: tokens, At: now,
		})
	})
	return ended, err
}

func codeOf(err error) string {
	if code := shared.AsError(err).DetailCode; code != "" {
		return code
	}
	return "agents.job_failed"
}

// jobWork is one job's acts and what they came to.
type jobWork struct {
	runner    Runner
	actor     appshared.ActorContext
	agent     domain.Agent
	proposals int
	tokens    int64
	code      string
}

func (w *jobWork) invoke(ctx context.Context, name string, in usecase.Input) (usecase.Output, error) {
	return w.runner.Desk.Catalogue.Invoke(ctx, name, w.actor, in)
}

// propose attempts a write. The gate holds it for a person, which is the proposal; a write the
// agent may not attempt at all - its person's rights, a rule NEVER - is passed over. A stopped
// agent or a spent budget ends the job.
func (w *jobWork) propose(ctx context.Context, name string, in usecase.Input) error {
	if w.proposals >= domain.MaxProposalsPerJob {
		return nil
	}
	_, err := w.invoke(ctx, name, in)
	switch code := shared.AsError(err).DetailCode; {
	case err == nil:
		// Only an agent whose kind no longer says shipped could have it run; the gate would have
		// held it otherwise. Counted, as it is in the ledger.
		w.proposals++
		return nil
	case code == "agents.approval_required":
		w.proposals++
		return nil
	case code == "agents.halted" || code == "agents.suspended" || code == "agents.not_active" ||
		code == "agents.model_changed" || code == "agents.budget_spent":
		return err
	case errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrValidation) || errors.Is(err, shared.ErrConflict):
		return nil
	}
	return err
}

// entry reads a ticket as the agent, and the hub its collection sits in.
func (w *jobWork) entry(ctx context.Context, item shared.ID) (usecase.Output, shared.ID, error) {
	out, err := w.invoke(ctx, getWorkItemName, usecase.Input{"item_id": item.String()})
	if err != nil {
		return nil, "", err
	}
	collection := out.String("collection_id")
	if collection == "" {
		return out, "", nil
	}
	container, err := w.invoke(ctx, getContainerName, usecase.Input{"container_id": collection})
	if err != nil {
		return nil, "", err
	}
	hub, err := shared.ParseID(container.String("parent_id"))
	if err != nil {
		return out, "", nil
	}
	return out, hub, nil
}

// triage classifies a ticket - labels proposed as a suggestion by the workspace's classifier -,
// proposes the configuration items its words name and routes it by its offering (G11).
func (w *jobWork) triage(ctx context.Context, item shared.ID) error {
	if w.runner.Proposer != nil {
		p, err := w.runner.Proposer.Classify(ctx, w.actor, item)
		if err != nil {
			return err
		}
		w.tokens += p.Tokens
		if p.Recorded {
			w.proposals++
		}
	}
	read, err := w.suggestCis(ctx, item)
	if err != nil {
		return err
	}
	return w.route(ctx, item, read)
}

// ticketRead is what suggestCis read of a ticket: the entry, its hub and the CIs of the hub its
// title or notes name or reference.
type ticketRead struct {
	entry usecase.Output
	hub   shared.ID
	named []string
}

// suggestCis proposes, as a comment on the entry, the configuration items of its hub whose names
// its title or notes name and that it does not reference yet - a reference a person keeps by
// approving it (P-19).
func (w *jobWork) suggestCis(ctx context.Context, item shared.ID) (ticketRead, error) {
	entry, hub, err := w.entry(ctx, item)
	if err != nil {
		return ticketRead{}, err
	}
	read := ticketRead{entry: entry, hub: hub}
	if hub.IsZero() {
		return read, nil
	}
	text := entry.String("title") + "\n" + entry.String("notes")
	words := normalised(text)
	var lines []string
	after := ""
	for page := 0; page < 10 && len(lines) < domain.MaxProposalsPerJob; page++ {
		in := usecase.Input{"hub_id": hub.String(), "limit": 500}
		if after != "" {
			in["after"] = after
		}
		out, err := w.invoke(ctx, listCisName, in)
		if err != nil {
			return ticketRead{}, err
		}
		for _, ci := range outputs(out["data"]) {
			id, name := ci.String("id"), strings.TrimSpace(ci.String("name"))
			if id == "" {
				continue
			}
			if strings.Contains(text, "hubtask:ci/"+id) {
				read.named = append(read.named, id)
				continue
			}
			if len([]rune(name)) < minCiName || !containsWords(words, normalised(name)) {
				continue
			}
			read.named = append(read.named, id)
			if len(lines) < domain.MaxProposalsPerJob {
				lines = append(lines, "["+markdownText(name)+"](hubtask:ci/"+id+")")
			}
		}
		next, _ := out["next"].(string)
		if next == "" {
			break
		}
		after = next
	}
	if len(lines) == 0 {
		return read, nil
	}
	return read, w.propose(ctx, addCommentName, usecase.Input{
		"item_id": item.String(), "body": strings.Join(lines, "\n"), "internal": true,
	})
}

// reviewDuplicates proposes for each open duplicate of a hub's inventory which CI stays: the one
// seen most recently by a source, else the one recorded first. A person approves the merge.
func (w *jobWork) reviewDuplicates(ctx context.Context, hub shared.ID) error {
	out, err := w.invoke(ctx, listCiDuplicatesName, usecase.Input{"hub_id": hub.String(), "limit": domain.MaxProposalsPerJob})
	if err != nil {
		return err
	}
	for _, dup := range outputs(out["data"]) {
		ids, _ := dup["ci_ids"].([]string)
		var cis []usecase.Output
		for _, id := range ids {
			ci, err := w.invoke(ctx, readCiName, usecase.Input{"ci_id": id})
			if err != nil {
				continue
			}
			cis = append(cis, ci)
		}
		survivor := Survivor(cis)
		if survivor == "" {
			continue
		}
		if err := w.propose(ctx, resolveCiDuplicateName, usecase.Input{
			"duplicate_id": dup.String("id"), "survivor_id": survivor,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Survivor is the CI of a duplicate that stays: the one a source saw last, else the oldest.
func Survivor(cis []usecase.Output) string {
	if len(cis) < 2 {
		return ""
	}
	sorted := append([]usecase.Output(nil), cis...)
	sort.SliceStable(sorted, func(i, j int) bool {
		si, oki := moment(sorted[i]["last_seen_at"])
		sj, okj := moment(sorted[j]["last_seen_at"])
		if oki != okj {
			return oki
		}
		if oki && !si.Equal(sj) {
			return si.After(sj)
		}
		ci, _ := moment(sorted[i]["created_at"])
		cj, _ := moment(sorted[j]["created_at"])
		return ci.Before(cj)
	})
	return sorted[0].String("id")
}

// draftKnowledge proposes a page in the ticket's hub from a resolved ticket: its title, its
// notes, and a reference back to it. A ticket still open is not drafted from.
func (w *jobWork) draftKnowledge(ctx context.Context, item shared.ID) error {
	entry, hub, err := w.entry(ctx, item)
	if err != nil {
		return err
	}
	if done, _ := entry["is_completed"].(bool); !done {
		w.code = OutcomeJobEntryOpen
		return nil
	}
	if hub.IsZero() {
		return nil
	}
	title := strings.TrimSpace(entry.String("title"))
	body := strings.TrimSpace(entry.String("notes"))
	if body != "" {
		body += "\n\n"
	}
	body += "[" + markdownText(title) + "](hubtask:entry/" + item.String() + ")"
	return w.propose(ctx, createPageName, usecase.Input{"hub_id": hub.String(), "title": title, "body": body})
}

func outputs(v any) []usecase.Output {
	switch rows := v.(type) {
	case []usecase.Output:
		return rows
	case []any:
		out := make([]usecase.Output, 0, len(rows))
		for _, row := range rows {
			switch m := row.(type) {
			case usecase.Output:
				out = append(out, m)
			case map[string]any:
				out = append(out, usecase.Output(m))
			}
		}
		return out
	}
	return nil
}

func moment(v any) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, !t.IsZero()
	case *time.Time:
		if t != nil {
			return *t, true
		}
	}
	return time.Time{}, false
}

// normalised is a text's words in lower case, each between single spaces, so a name is found as
// whole words.
func normalised(s string) string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '.'
	})
	kept := fields[:0]
	for _, f := range fields {
		// A sentence's full stop is not part of the name before it.
		if f = strings.Trim(f, ".-_"); f != "" {
			kept = append(kept, f)
		}
	}
	return " " + strings.Join(kept, " ") + " "
}

func containsWords(text, name string) bool {
	return strings.TrimSpace(name) != "" && strings.Contains(text, name)
}

// markdownText keeps a name from closing its link's brackets.
func markdownText(s string) string {
	return strings.NewReplacer("[", "(", "]", ")", "\n", " ").Replace(s)
}
