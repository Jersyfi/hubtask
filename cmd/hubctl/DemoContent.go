// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// The showcase's content, the one copy of it: scripts/seed-demo.sh runs it with --voices. What
// another person of seed-demo.sh says goes through say, so that it is theirs there; the people
// and groups are used only where the workspace has them by name.

var demoModules = []string{"WORKFLOWS", "PLANNING", "KNOWLEDGE", "WHITEBOARD", "SERVICE_DESK", "ASSETS", "TIME", "GOALS", "AGENTS"}

// entryOptions are the optional fields of an entry.
type entryOptions struct {
	entryType, notes string
	assignee         any
	start, due       string
	fields           map[string]any
	labels           []any
}

func (d *demo) run(ctx context.Context) {
	for _, module := range demoModules {
		d.write(ctx, "module "+module, "PUT", "/tenant/modules/"+module, map[string]any{"switch": "ON"})
	}
	if _, held := d.held["hub.scrum"]; !held {
		// The workspace takes each template first, as the gallery does; a second run finds the
		// hubs marked and leaves the configuration as the first left it.
		for _, key := range []string{"scrum_team", "kanban_team", "classic_project", "itil_service_desk"} {
			d.take(ctx, key, "", false)
		}
	}
	scrum := d.scrum(ctx)
	kanban := d.kanban(ctx)
	classic := d.classic(ctx)
	desk, deskFresh := d.serviceDesk(ctx)
	d.cmdb(ctx, desk, deskFresh)
	mine := d.yours(ctx, scrum, kanban, classic, desk)
	d.wiki(ctx, scrum.hub, desk, classic.hub)
	d.whiteboards(ctx, scrum, kanban)
	d.timeTracking(ctx, scrum, classic, desk, mine)
	d.goals(ctx, scrum, kanban, classic, desk)
}

// take plans a template and applies exactly that plan, as the gallery takes it.
func (d *demo) take(ctx context.Context, key, hub string, samples bool) {
	body := map[string]any{}
	if key == "itil_service_desk" && hub == "" {
		// The desk's groups where the workspace has them by the names seed-demo.sh gives them.
		answers := map[string]any{}
		for answer, group := range map[string]string{"first_group": "Service Desk – 1st Level", "cab_group": "Change Advisory Board"} {
			if id := d.groups[group]; id != "" {
				answers[answer] = id
			}
		}
		if len(answers) > 0 {
			body["answers"] = answers
		}
	}
	if hub != "" {
		body["hub_id"] = hub
	}
	if samples {
		body["samples"] = true
	}
	plan := d.write(ctx, "template "+key, "POST", "/config/templates/"+key+":plan", body)
	if plan == nil {
		return
	}
	if str(plan["plan_hash"]) == "" {
		d.refused = append(d.refused, fmt.Sprintf("template %s: planned with refusals %v", key, plan["refusals"]))
		return
	}
	mode := str(plan["mode"])
	if mode == "" {
		mode = "MERGE"
	}
	apply := map[string]any{"document": plan["document"], "mode": mode, "plan_hash": plan["plan_hash"],
		"template": plan["template"], "acknowledge": true}
	if h := str(plan["hub_id"]); h != "" {
		apply["hub_id"] = h
	}
	if c := str(plan["collection_id"]); c != "" {
		apply["collection_id"] = c
	}
	if d.write(ctx, "template "+key+" apply", "POST", "/config:apply", apply) != nil {
		d.counts["templates"]++
	}
}

func (d *demo) hub(ctx context.Context, key, name, description, icon, color string) (string, bool) {
	return d.object(ctx, "hub."+key, "CONTAINER", "hubs", "/containers", map[string]any{
		"type": "HUB", "name": name, "description": description, "icon": icon, "color_token": color,
	})
}

func (d *demo) collection(ctx context.Context, key, parent, name, description string) string {
	if parent == "" {
		return ""
	}
	id, _ := d.object(ctx, "collection."+key, "CONTAINER", "collections", "/containers", map[string]any{
		"type": "COLLECTION", "parent_id": parent, "name": name, "description": description,
	})
	return id
}

func (d *demo) entry(ctx context.Context, key, collection, title string, o entryOptions) (string, bool) {
	if collection == "" {
		return "", false
	}
	return d.item(ctx, key, map[string]any{"type": "TASK", "title": title, "collection_id": collection}, o)
}

func (d *demo) child(ctx context.Context, key, parent, kind, title string, assignee any) (string, bool) {
	if parent == "" {
		return "", false
	}
	return d.datedChild(ctx, key, parent, kind, title, entryOptions{assignee: assignee})
}

// datedChild is a work package or an activity with its own options, its dates among them.
func (d *demo) datedChild(ctx context.Context, key, parent, kind, title string, o entryOptions) (string, bool) {
	if parent == "" {
		return "", false
	}
	return d.item(ctx, key, map[string]any{"type": kind, "title": title, "parent_id": parent}, o)
}

func (d *demo) item(ctx context.Context, key string, body map[string]any, o entryOptions) (string, bool) {
	labels := make([]any, 0, len(o.labels))
	for _, l := range o.labels {
		if str(l) != "" {
			labels = append(labels, l)
		}
	}
	for field, value := range map[string]any{"entry_type": o.entryType, "notes": o.notes, "assignee_id": o.assignee,
		"start_at": o.start, "due_at": o.due} {
		body[field] = value
	}
	if len(o.fields) > 0 {
		body["custom_fields"] = o.fields
	}
	if len(labels) > 0 {
		body["label_ids"] = labels
	}
	return d.object(ctx, "entry."+key, "ENTRY", "entries", "/items", body)
}

func (d *demo) steps(ctx context.Context, item string, keys ...string) {
	for _, key := range keys {
		if d.write(ctx, "step "+key, "POST", "/items/"+item+":step", map[string]any{"step": key}) == nil {
			return
		}
	}
}

// comment is a speaker's comment on an entry; speaker names the person of seed-demo.sh who says it.
func (d *demo) comment(ctx context.Context, speaker, item, body string, internal bool) {
	if d.say(ctx, speaker, "comment", "POST", "/items/"+item+"/comments", map[string]any{"body": body, "internal": internal}) != nil {
		d.counts["comments"]++
	}
}

func (d *demo) link(ctx context.Context, source, target, kind string) map[string]any {
	if source == "" || target == "" {
		return nil
	}
	out := d.write(ctx, "link "+kind, "POST", "/items/"+source+"/links", map[string]any{"target_id": target, "type": kind})
	if out != nil {
		d.counts["links"]++
	}
	return out
}

func (d *demo) labels(ctx context.Context, container string, spec [][2]string) map[string]any {
	out := map[string]any{}
	for _, l := range spec {
		if made := d.write(ctx, "label "+l[0], "POST", "/containers/"+container+"/labels",
			map[string]any{"name": l[0], "color_token": l[1]}); made != nil {
			out[l[0]] = made["id"]
		}
	}
	return out
}

type demoHub struct {
	hub     string
	entries []string
	coll    map[string]string
}

func (d *demo) scrum(ctx context.Context) demoHub {
	out := demoHub{coll: map[string]string{}}
	h, fresh := d.hub(ctx, "scrum", "Produktentwicklung", "Zwei Scrum-Teams bauen die Kunden-App und die Plattform dahinter.", "rocket", "blue")
	out.hub = h
	if h == "" {
		return out
	}
	if fresh {
		d.take(ctx, "scrum_team", h, false)
		if jonas := d.who("Jonas Weber"); jonas != nil {
			d.write(ctx, "membership Jonas", "POST", "/memberships", map[string]any{"account_id": jonas, "scope_type": "HUB", "scope_id": h, "role": "MEMBER"})
		}
	}
	app := d.collection(ctx, "scrum.app", h, "Team Atlas – Mobile App", "iOS- und Android-App für Endkundinnen und Endkunden.")
	api := d.collection(ctx, "scrum.api", h, "Team Boreal – Platform & API", "Public REST API, billing and the event pipeline.")
	out.coll["app"], out.coll["api"] = app, api
	lab := map[string]any{}
	if app != "" && d.held["entry.scrum.login"] == "" {
		lab = d.labels(ctx, app, [][2]string{{"Kundenwunsch", "violet"}, {"Tech-Schuld", "orange"}, {"Barrierefreiheit", "teal"}, {"Sicherheit", "red"}})
	}
	lukas, amelia, mehmet, sofia, jonas := d.who("Lukas Brandt"), d.who("Amelia Hughes"), d.who("Mehmet Yılmaz"), d.who("Sofia Rossi"), d.who("Jonas Weber")

	var stories []string
	login, loginFresh := d.entry(ctx, "scrum.login", app, "Als Kundin möchte ich mich mit Passkey anmelden, damit ich mir kein Passwort mehr merken muss", entryOptions{
		entryType: "story", assignee: amelia, labels: []any{lab["Kundenwunsch"], lab["Sicherheit"]}, due: d.day(9, 9),
		notes: "## Hintergrund\n\n38 % der Support-Tickets im letzten Quartal betrafen vergessene Passwörter.\n\n" +
			"## Akzeptanzkriterien\n\n- Passkey lässt sich in den Einstellungen hinzufügen\n- Anmeldung ohne Passwort auf iOS 17+ und Android 14+\n" +
			"- Fallback auf E-Mail-Code bleibt erhalten",
	})
	stories = append(stories, login)
	wp, _ := d.child(ctx, "scrum.login.register", login, "WORK_PACKAGE", "Passkey-Registrierung im Profil", amelia)
	for i, title := range []string{"WebAuthn-Challenge vom Server holen", "Systemdialog auf iOS anbinden", "Systemdialog auf Android anbinden",
		"Fehlerfälle: abgebrochen, kein Gerät, Zeitüberschreitung"} {
		d.child(ctx, fmt.Sprintf("scrum.login.register.a%d", i), wp, "ACTIVITY", title, nil)
	}
	wp2, _ := d.child(ctx, "scrum.login.signin", login, "WORK_PACKAGE", "Anmeldung mit Passkey", mehmet)
	for i, title := range []string{"Login-Screen um Passkey-Button erweitern", "Telemetrie: Anteil Passkey-Anmeldungen messen"} {
		d.child(ctx, fmt.Sprintf("scrum.login.signin.a%d", i), wp2, "ACTIVITY", title, nil)
	}
	if loginFresh {
		d.steps(ctx, login, "plan", "start")
		d.comment(ctx, "Mehmet Yılmaz", login, "Android 13 kann Passkeys nur mit Google Play Services ≥ 23.x – sollen wir das als Mindestversion festlegen?", false)
		d.comment(ctx, "Amelia Hughes", login, "Yes, let's require it; the share of Android 13 without current Play Services is under 2 % in our analytics.", false)
	}

	offline, fresh := d.entry(ctx, "scrum.offline", app, "Offline mode: orders placed without a connection are queued and sent once the phone is back online", entryOptions{
		entryType: "story", assignee: sofia, labels: []any{lab["Kundenwunsch"]}, due: d.day(16, 9),
		notes: "Customers in rural areas lose their basket when the train enters a tunnel. Queue the order locally, " +
			"show a clear *pending* state and send it within 30 seconds of reconnecting.",
	})
	stories = append(stories, offline)
	if fresh {
		d.steps(ctx, offline, "plan")
	}
	d.child(ctx, "scrum.offline.queue", offline, "WORK_PACKAGE", "Local queue with retry and idempotency key", nil)
	d.child(ctx, "scrum.offline.badge", offline, "WORK_PACKAGE", "Pending badge in the order list", nil)

	darkmode, fresh := d.entry(ctx, "scrum.darkmode", app, "Dunkelmodus folgt der Systemeinstellung", entryOptions{
		entryType: "story", assignee: jonas, labels: []any{lab["Barrierefreiheit"]}})
	stories = append(stories, darkmode)
	if fresh {
		d.steps(ctx, darkmode, "plan", "start", "review", "finish")
	}
	a11y, fresh := d.entry(ctx, "scrum.a11y", app, "Screenreader liest Preise inklusive Währung vor (VoiceOver und TalkBack)", entryOptions{
		entryType: "story", labels: []any{lab["Barrierefreiheit"]}, assignee: sofia})
	stories = append(stories, a11y)
	if fresh {
		d.steps(ctx, a11y, "plan", "start", "review")
	}
	bug1, fresh := d.entry(ctx, "scrum.pushcrash", app, "App stürzt beim Öffnen eines Push-Links ab, wenn die App vorher beendet war", entryOptions{
		entryType: "bug", assignee: mehmet, labels: []any{lab["Tech-Schuld"]},
		notes: "Reproduzierbar auf Pixel 8, Android 15. Stacktrace im Crash-Reporter: `NullPointerException in DeepLinkRouter.resolve`."})
	if fresh {
		d.steps(ctx, bug1, "plan", "start")
		d.comment(ctx, "Sofia Rossi", bug1, "Seen on iOS too? I could not reproduce it on an iPhone 15 with iOS 18.1.", false)
		d.link(ctx, bug1, login, "blocks")
		d.link(ctx, offline, darkmode, "depends_on")
	}
	bug2, _ := d.entry(ctx, "scrum.keyboard", app, "Checkout button hidden behind the keyboard on small screens", entryOptions{entryType: "bug", assignee: jonas})
	stories = append(stories, bug2)

	platformSpec := []struct {
		title, kind string
		who         any
	}{
		{"Rate limiting per API key with a clear Retry-After header", "story", amelia},
		{"Webhooks: Zustellungen bei 5xx exponentiell wiederholen und nach 24 h aufgeben", "story", lukas},
		{"Billing: invoices as PDF/A-3 with embedded ZUGFeRD XML", "story", mehmet},
		{"Event-Pipeline: Partitionierung nach Mandant statt nach Zeit", "story", amelia},
		{"Pagination cursor breaks when two entries share a timestamp", "bug", lukas},
	}
	platform := make([]string, len(platformSpec))
	anyFresh := false
	for i, p := range platformSpec {
		var made bool
		platform[i], made = d.entry(ctx, fmt.Sprintf("scrum.platform%d", i), api, p.title, entryOptions{entryType: p.kind, assignee: p.who})
		anyFresh = anyFresh || made
	}
	if anyFresh && platform[4] != "" {
		d.steps(ctx, platform[0], "plan", "start", "review")
		d.steps(ctx, platform[1], "plan", "start")
		d.steps(ctx, platform[4], "plan", "start", "review", "finish")
		d.link(ctx, platform[3], platform[1], "blocks")
		d.comment(ctx, "Lukas Brandt", platform[1], "Wir sollten die Wiederholungen im Audit sichtbar machen, sonst fragt der Support jedes Mal nach.", false)
		d.comment(ctx, "Amelia Hughes", platform[0], "Draft of the limits: 600 requests/minute per key, burst 1200. @Lukas please sanity-check before Friday.", false)
	}

	// Sprints: one finished, one running, one planned; estimates in points.
	// The sprints and releases are the hub's, written once with the story they start from.
	if loginFresh {
		points := []int{3, 5, 8, 2, 13, 1, 5, 3, 8}
		for i, story := range append(append([]string{}, stories...), platform...) {
			if story != "" {
				d.write(ctx, "estimate", "PUT", "/items/"+story+"/estimate", map[string]any{"unit": "POINTS", "value": points[i%9]})
			}
		}
		if s1 := d.write(ctx, "sprint 23", "POST", "/collections/"+app+"/sprints", map[string]any{"name": "Sprint 23 – Barrierefreiheit",
			"goal": "Dunkelmodus und Screenreader-Preise ausliefern", "start_on": d.date(-21), "end_on": d.date(-8), "unit": "POINTS"}); s1 != nil {
			id := str(s1["id"])
			d.write(ctx, "sprint 23 entries", "POST", "/sprints/"+id+"/entries", map[string]any{"item_ids": []string{darkmode, a11y}})
			d.write(ctx, "sprint 23 start", "POST", "/sprints/"+id+":start", nil)
			d.write(ctx, "sprint 23 close", "POST", "/sprints/"+id+":close", map[string]any{})
			d.counts["sprints"]++
		}
		if s2 := d.write(ctx, "sprint 24", "POST", "/collections/"+app+"/sprints", map[string]any{"name": "Sprint 24 – Passkeys",
			"goal": "Passwortlose Anmeldung für alle Kundinnen und Kunden", "start_on": d.date(-7), "end_on": d.date(6), "unit": "POINTS"}); s2 != nil {
			id := str(s2["id"])
			d.write(ctx, "sprint 24 entries", "POST", "/sprints/"+id+"/entries", map[string]any{"item_ids": []string{login, bug1, a11y}})
			d.write(ctx, "sprint 24 start", "POST", "/sprints/"+id+":start", nil)
			d.counts["sprints"]++
		}
		if s3 := d.write(ctx, "sprint 25", "POST", "/collections/"+app+"/sprints", map[string]any{"name": "Sprint 25 – Offline",
			"start_on": d.date(7), "end_on": d.date(20), "unit": "POINTS"}); s3 != nil {
			d.write(ctx, "sprint 25 entries", "POST", "/sprints/"+str(s3["id"])+"/entries", map[string]any{"item_ids": []string{offline, bug2}})
			d.counts["sprints"]++
		}
		if rel := d.write(ctx, "release 4.2", "POST", "/hubs/"+h+"/releases", map[string]any{"name": "App 4.2 „Schlüssellos“",
			"description": "Passkeys, dark mode and screen reader prices.", "release_on": d.date(14)}); rel != nil {
			d.write(ctx, "release entries", "POST", "/releases/"+str(rel["id"])+"/entries", map[string]any{"item_ids": []string{login, darkmode, a11y}})
			d.counts["releases"]++
		}
		if rel := d.write(ctx, "release api", "POST", "/hubs/"+h+"/releases", map[string]any{"name": "API v3.8", "release_on": d.date(30)}); rel != nil {
			d.write(ctx, "release api entries", "POST", "/releases/"+str(rel["id"])+"/entries", map[string]any{"item_ids": platform[:3]})
			d.counts["releases"]++
		}
	}
	out.entries = append(stories, platform...)
	return out
}

// yours gives the person the showcase runs as - the one meant to see what exists - work of their
// own, so that Today shows it (user-experience.md §3.3): an entry due today in the running sprint,
// one overdue, one coming up and a desk incident whose promised times run. Each under a mark of its
// own, so a showcase made before this is completed by the next run.
func (d *demo) yours(ctx context.Context, scrum, kanban, classic, desk demoHub) []string {
	if d.self == "" {
		return nil
	}
	story, fresh := d.entry(ctx, "mine.story", scrum.coll["app"], "Passkey-Anmeldung: Wiederherstellung über ein zweites Gerät",
		entryOptions{entryType: "story", assignee: d.self, due: d.day(0, 17)})
	if fresh {
		d.write(ctx, "estimate", "PUT", "/items/"+story+"/estimate", map[string]any{"unit": "POINTS", "value": 3})
		if sprint := d.runningSprint(ctx, scrum.coll["app"]); sprint != "" {
			d.write(ctx, "sprint mine", "POST", "/sprints/"+sprint+"/entries", map[string]any{"item_ids": []string{story}})
		}
	}
	card, _ := d.entry(ctx, "mine.card", kanban.coll["board"], "Pressemitteilung zur Herbstmesse freigeben",
		entryOptions{entryType: "card", assignee: d.self, due: d.day(-2, 12)})
	phase, _ := d.entry(ctx, "mine.phase", classic.coll["plan"], "Lenkungskreis: Freigabe der Ausführungsplanung",
		entryOptions{entryType: "deliverable", assignee: d.self, start: d.day(-3, 8), due: d.day(3, 12)})
	incident, _ := d.entry(ctx, "mine.incident", desk.coll["desk"], "Geschäftsführung erreicht das Board-Portal nicht",
		entryOptions{entryType: "incident", assignee: d.self, fields: map[string]any{"impact": "High", "urgency": "High", "priority": "P1"}})
	return []string{story, card, phase, incident}
}

// runningSprint is the collection's active sprint, or empty where none runs or it cannot be read.
func (d *demo) runningSprint(ctx context.Context, collection string) string {
	if collection == "" {
		return ""
	}
	var sprints struct {
		Data []struct {
			ID    string `json:"id"`
			State string `json:"state"`
		} `json:"data"`
	}
	if err := d.call(ctx, func(ctx context.Context) error {
		return d.c.Get(ctx, "/collections/"+collection+"/sprints", nil, &sprints)
	}); err != nil {
		return ""
	}
	for _, s := range sprints.Data {
		if s.State == "ACTIVE" {
			return s.ID
		}
	}
	return ""
}

func (d *demo) kanban(ctx context.Context) demoHub {
	out := demoHub{coll: map[string]string{}}
	h, fresh := d.hub(ctx, "kanban", "Marketing & Kommunikation", "Content, Kampagnen und Messen – ein Board, das alle sehen.", "megaphone", "violet")
	out.hub = h
	if h == "" {
		return out
	}
	if fresh {
		d.take(ctx, "kanban_team", h, false)
	}
	board := d.collection(ctx, "kanban.board", h, "Content-Board Q4", "Blog, newsletter, social and the trade fair in November.")
	out.coll["board"] = board
	lab := map[string]any{}
	if board != "" && d.held["entry.kanban.card0"] == "" {
		lab = d.labels(ctx, board, [][2]string{{"Blog", "blue"}, {"Newsletter", "green"}, {"Social", "magenta"}, {"Messe", "orange"}})
	}
	sofia, priya, jonas := d.who("Sofia Rossi"), d.who("Priya Natarajan"), d.who("Jonas Weber")
	cards := []struct {
		title, label string
		who          any
		walk         []string
	}{
		{"Fallstudie: Wie die Stadtwerke Lüneburg ihre Störungsannahme von 3 Tagen auf 4 Stunden verkürzt haben", "Blog", priya, []string{"pull", "review"}},
		{"Newsletter November: Rückblick Herbstmesse, neue Integrationen, Webinar-Termine", "Newsletter", sofia, []string{"pull"}},
		{"LinkedIn carousel: five signs your service desk has outgrown spreadsheets", "Social", jonas, []string{"pull", "review", "finish"}},
		{"Messestand IT-Expo Köln: Standplan, Bildschirme, Demo-Laptops und Give-aways", "Messe", priya, nil},
		{"Webinar: Running ITIL change management without a 40-page process manual", "Blog", sofia, nil},
		{"Pressemitteilung zur Version 4.2 abstimmen (Freigabe durch Geschäftsführung)", "", priya, []string{"pull"}},
		{"Short video: setting up a Kanban board in under two minutes", "Social", jonas, nil},
	}
	comments := [][2]string{
		{"Priya Natarajan", "Freigabe der Stadtwerke liegt vor, Zitat von Frau Albers ist abgestimmt."},
		{"Sofia Rossi", "Webinar dates: 19 and 26 November, both 10:00 CET."},
		{"Jonas Weber", "Bildschirme für die Messe sind bestellt, Lieferung KW 46. Das Karussell ist live, 1.240 Impressionen am ersten Tag."},
	}
	for i, c := range cards {
		id, made := d.entry(ctx, fmt.Sprintf("kanban.card%d", i), board, c.title, entryOptions{
			entryType: "card", assignee: c.who, labels: []any{lab[c.label]}, due: d.day(i*3+2, 9)})
		if made {
			d.steps(ctx, id, c.walk...)
			if i < len(comments) {
				d.comment(ctx, comments[i][0], id, comments[i][1], false)
			}
		}
		out.entries = append(out.entries, id)
	}
	return out
}

func (d *demo) classic(ctx context.Context) demoHub {
	out := demoHub{coll: map[string]string{}}
	h, fresh := d.hub(ctx, "classic", "Neubau Logistikzentrum Hamburg-Billbrook",
		"Klassisches Projekt mit Phasen, Meilensteinen, Risiken und Änderungsanträgen. Budget 18,4 Mio. €.", "building", "orange")
	out.hub = h
	if h == "" {
		return out
	}
	if fresh {
		d.take(ctx, "classic_project", h, false)
	}
	plan := d.collection(ctx, "classic.plan", h, "Projektplan", "Phasen nach HOAI, Abhängigkeiten und Meilensteine.")
	risks := d.collection(ctx, "classic.risks", h, "Risiken & Offene Punkte", "")
	out.coll["plan"], out.coll["risks"] = plan, risks
	lukas, priya, mehmet := d.who("Lukas Brandt"), d.who("Priya Natarajan"), d.who("Mehmet Yılmaz")
	phases := []struct {
		title      string
		start, end int
		walk       []string
		who        any
	}{
		{"Grundlagenermittlung und Vorplanung", -60, -35, []string{"start", "accept"}, lukas},
		{"Entwurfs- und Genehmigungsplanung", -34, -5, []string{"start", "accept"}, priya},
		{"Bauantrag einreichen", -4, -4, []string{"start", "accept"}, lukas},
		{"Ausführungsplanung Tragwerk, TGA und Fördertechnik", -3, 40, []string{"start"}, priya},
		{"Ausschreibung und Vergabe der Gewerke", 20, 70, nil, mehmet},
		{"Rohbau Halle A und B", 71, 180, nil, nil},
		{"Inbetriebnahme Fördertechnik und Probebetrieb", 181, 220, nil, nil},
		{"Übergabe an den Betrieb", 221, 221, nil, lukas},
	}
	ids := make([]string, len(phases))
	phasesFresh := false
	for i, p := range phases {
		var made bool
		ids[i], made = d.entry(ctx, fmt.Sprintf("classic.phase%d", i), plan, p.title, entryOptions{
			entryType: "deliverable", assignee: p.who, start: d.day(p.start, 8), due: d.day(p.end, 17)})
		if made {
			d.steps(ctx, ids[i], p.walk...)
			phasesFresh = true
		}
	}
	// The work packages and activities lie inside their phase's window (-3 to 40), so the plan
	// draws each under its phase rather than in the tray of entries without dates.
	type span struct {
		title      string
		start, end int
	}
	for i, wp := range []span{{"Statik Hallendach (Spannweite 42 m)", -3, 12}, {"Sprinkleranlage nach VdS CEA 4001", 5, 25}, {"Hochregallager 24 m – Regalbediengeräte", 10, 35}} {
		d.datedChild(ctx, fmt.Sprintf("classic.phase3.wp%d", i), ids[3], "WORK_PACKAGE", wp.title, entryOptions{start: d.day(wp.start, 8), due: d.day(wp.end, 17)})
	}
	sub, _ := d.datedChild(ctx, "classic.phase3.air", ids[3], "WORK_PACKAGE", "Lüftung und Wärmerückgewinnung", entryOptions{start: d.day(0, 8), due: d.day(30, 17)})
	for i, a := range []span{{"Lastannahmen mit dem Betreiber abstimmen", 0, 6}, {"Variantenvergleich Wärmepumpe vs. Fernwärme", 4, 14}, {"Kosten nach DIN 276 schätzen", 12, 20}} {
		d.datedChild(ctx, fmt.Sprintf("classic.phase3.air.a%d", i), sub, "ACTIVITY", a.title, entryOptions{start: d.day(a.start, 8), due: d.day(a.end, 17)})
	}
	if phasesFresh && ids[7] != "" {
		for _, m := range []int{2, 7} {
			d.write(ctx, "milestone", "PUT", "/items/"+ids[m]+"/scheduling", map[string]any{"milestone": true, "mode": "MANUAL"})
		}
		// The Gantt's dependencies: finish-to-start with lags where the building authority needs time.
		for _, dep := range []struct {
			before, after int
			kind          string
			lag           int
		}{{0, 1, "FS", 0}, {1, 2, "FS", 0}, {2, 3, "SS", 1}, {3, 4, "SS", 20}, {4, 5, "FS", 0}, {5, 6, "FS", 0}, {6, 7, "FS", 0}} {
			if made := d.link(ctx, ids[dep.after], ids[dep.before], "depends_on"); made != nil && str(made["id"]) != "" {
				d.write(ctx, "dependency timing", "PUT", "/links/"+str(made["id"])+"/timing", map[string]any{"dependency_kind": dep.kind, "lag_days": dep.lag})
			}
		}
		d.write(ctx, "baseline", "POST", "/plans/"+plan+"/baselines", map[string]any{"name": "Freigegebener Rahmenterminplan (Lenkungskreis 12.09.)"})
		if read := d.write(ctx, "plan budget read", "GET", "/plans/"+plan+"/budget", nil); read != nil {
			version := read["version"]
			if version == nil {
				version = 0
			}
			d.write(ctx, "plan budget", "PUT", "/plans/"+plan+"/budget", map[string]any{"minutes": 4800 * 60, "amount_minor": 1_840_000_000,
				"currency": "EUR", "expected_version": version})
		}
	}
	// Each risk, the issue and the change request is due for its next review or decision, which
	// is the day the plan draws it on.
	for i, r := range []struct {
		title, probability, impact string
		walk                       []string
		review                     int
	}{
		{"Lieferverzug Stahlträger wegen Engpass im Walzwerk", "High", "High", []string{"mitigate"}, 7},
		{"Baugrund: Altlasten im Bereich der ehemaligen Tankstelle", "Medium", "High", nil, 14},
		{"Fachkräftemangel Elektro – Nachunternehmer springt ab", "Medium", "Medium", []string{"mitigate"}, 10},
		{"Genehmigung verzögert sich durch Einwendungen der Nachbarschaft", "Low", "High", []string{"close"}, -10},
	} {
		if id, made := d.entry(ctx, fmt.Sprintf("classic.risk%d", i), risks, r.title, entryOptions{entryType: "risk",
			fields: map[string]any{"probability": r.probability, "impact": r.impact}, assignee: priya, due: d.day(r.review, 12)}); made {
			d.steps(ctx, id, r.walk...)
		}
	}
	if issue, made := d.entry(ctx, "classic.access", risks, "Zufahrt für Schwerlastverkehr während der Bauphase ungeklärt", entryOptions{entryType: "issue", assignee: lukas, due: d.day(3, 10)}); made {
		d.comment(ctx, "Lukas Brandt", issue, "Termin mit dem Bezirksamt Hamburg-Mitte am Donnerstag, 10 Uhr. Unterlagen liegen im Wiki.", false)
	}
	if cr, made := d.entry(ctx, "classic.pv", risks, "Änderungsantrag: zusätzliche Photovoltaik auf Halle B (420 kWp)", entryOptions{entryType: "change_request",
		notes: "Mehrkosten ca. 380.000 €, Amortisation laut Gutachten nach 7,5 Jahren.", due: d.day(14, 12)}); made {
		d.steps(ctx, cr, "assess")
	}
	out.entries = ids
	return out
}

func (d *demo) serviceDesk(ctx context.Context) (demoHub, bool) {
	out := demoHub{coll: map[string]string{}}
	h, fresh := d.hub(ctx, "desk", "IT-Service-Desk", "Störungen, Serviceanfragen, Probleme und Changes nach ITIL 4.", "life-buoy", "teal")
	out.hub = h
	if h == "" {
		return out, false
	}
	desk := d.collection(ctx, "desk.intake", h, "Störungen & Anfragen", "Was über das Portal, per Mail oder am Telefon ankommt.")
	changes := d.collection(ctx, "desk.changes", h, "Changes", "")
	problems := d.collection(ctx, "desk.problems", h, "Probleme", "")
	out.coll["desk"], out.coll["changes"], out.coll["problems"] = desk, changes, problems
	if fresh {
		// After the collections: the template's sample incidents land in the hub's first one.
		d.take(ctx, "itil_service_desk", h, true)
		if cab := d.groups["Change Advisory Board"]; cab != "" {
			d.write(ctx, "cab role", "POST", "/role-grants", map[string]any{"role": "cab_member", "scope_type": "HUB", "scope_id": h, "group_id": cab})
		}
		for _, o := range []struct {
			key, name, domain string
			shares            bool
		}{{"stadtwerke", "Stadtwerke Lüneburg GmbH", "stadtwerke-lueneburg.example", true}, {"nordwind", "Nordwind Logistics Ltd.", "nordwind-logistics.example", false}} {
			if d.write(ctx, "organisation "+o.key, "PUT", "/requester-organisations/"+o.key,
				map[string]any{"name": o.name, "domains": []string{o.domain}, "shares_requests": o.shares}) != nil {
				d.counts["organisations"]++
			}
		}
	}
	mehmet, sofia := d.who("Mehmet Yılmaz"), d.who("Sofia Rossi")
	// The portal's requests: raised by seed-demo.sh's requesters under --voices, otherwise entered
	// by the person running the showcase, as the desk enters a call.
	requests := []struct{ requester, kind, title, notes string }{
		{"Hannah Albers", "incident", "Seit dem Update lässt sich das Kundenportal in Firefox nicht mehr öffnen",
			"Fehlermeldung: »Die Verbindung wurde zurückgesetzt«. In Edge funktioniert es. Betroffen sind alle 14 Arbeitsplätze im Kundenzentrum."},
		{"Felix Hartmann", "service_request", "Neuer Zugang für Auszubildende ab 1. Dezember (3 Personen)", "Bitte Zugänge mit Rolle »Sachbearbeitung« anlegen."},
		{"Grace O'Neill", "incident", "Delivery notes are printed without the barcode since this morning",
			"Warehouse 2 in Felixstowe. Printer model Zebra ZT411, the test page prints fine."},
		{"Hannah Albers", "service_request", "Lizenz für das Planungswerkzeug auf eine weitere Person übertragen", ""},
	}
	raised := make([]string, len(requests))
	for i, r := range requests {
		var made bool
		body := map[string]any{"collection_id": desk, "entry_type": r.kind, "title": r.title, "notes": r.notes}
		if d.voices {
			raised[i], made = d.spokenObject(ctx, r.requester, fmt.Sprintf("entry.desk.request%d", i), "ENTRY", "portal requests", "/requests", body)
		} else {
			body["type"] = "TASK"
			raised[i], made = d.object(ctx, fmt.Sprintf("entry.desk.request%d", i), "ENTRY", "portal requests", "/items", body)
		}
		if !made || raised[i] == "" {
			continue
		}
		switch i {
		case 0:
			if mehmet != nil {
				d.write(ctx, "assign", "POST", "/items/"+raised[i]+":assign", map[string]any{"account_id": mehmet})
			}
			for _, f := range [][2]string{{"impact", "High"}, {"urgency", "High"}, {"priority", "P1"}} {
				d.write(ctx, "triage "+f[0], "PUT", "/items/"+raised[i]+"/custom-fields/"+f[0], map[string]any{"value": f[1]})
			}
			d.steps(ctx, raised[i], "start")
			d.comment(ctx, "Mehmet Yılmaz", raised[i], "Danke für die Meldung – wir sehen das auch intern. Ein Zertifikat der Zwischen-CA fehlt in der Kette, wir tauschen es heute bis 14 Uhr.", false)
			d.comment(ctx, "Mehmet Yılmaz", raised[i], "Intern: betrifft auch das Partnerportal, Problem-Ticket angelegt.", true)
			if d.voices {
				d.say(ctx, r.requester, "requester reply", "POST", "/requests/"+raised[i]+"/comments",
					map[string]any{"body": "Vielen Dank, wir warten ab und melden uns, falls es danach nicht geht."})
			}
		case 1:
			// The desk acknowledges it with the template's macro, so the run log holds a run.
			if macro := d.acknowledgement(ctx, h); macro != "" {
				if d.write(ctx, "acknowledge", "POST", "/automation/rules/"+macro+":trigger", map[string]any{"item_id": raised[i]}) != nil {
					d.counts["automation runs"]++
				}
			}
		case 2:
			if sofia != nil {
				d.write(ctx, "assign", "POST", "/items/"+raised[i]+":assign", map[string]any{"account_id": sofia})
			}
			d.steps(ctx, raised[i], "start", "resolve")
			d.comment(ctx, "Sofia Rossi", raised[i], "The label template lost its barcode field in last night's update; restored it, please try again.", false)
		}
	}
	for i, inc := range []struct {
		title, impact, urgency, priority string
		who                              any
	}{
		{"VPN-Gateway Frankfurt antwortet sporadisch nicht (Paketverlust 30 %)", "High", "Medium", "P2", mehmet},
		{"Shared mailbox 'invoices@' stopped receiving mail from the ERP", "Medium", "High", "P2", sofia},
		{"Drucker 2. OG druckt nur leere Seiten", "Low", "Low", "P4", nil},
	} {
		id, _ := d.entry(ctx, fmt.Sprintf("desk.incident%d", i), desk, inc.title, entryOptions{entryType: "incident", assignee: inc.who,
			fields: map[string]any{"impact": inc.impact, "urgency": inc.urgency, "priority": inc.priority}})
		out.entries = append(out.entries, id)
	}
	if problem, made := d.entry(ctx, "desk.problem", problems, "Wiederkehrende Zertifikatsfehler nach automatischer Erneuerung", entryOptions{
		entryType: "problem", assignee: mehmet,
		notes: "Drei Störungen in sechs Wochen mit derselben Ursache: die Zwischenzertifikate werden bei der Erneuerung nicht mitgeliefert."}); made {
		d.steps(ctx, problem, "investigate", "record_known_error")
	}
	if change, made := d.entry(ctx, "desk.change", changes, "Zertifikatskette im Load Balancer um Zwischen-CA ergänzen (Wartungsfenster Do 22–23 Uhr)", entryOptions{
		entryType: "change", assignee: mehmet,
		notes: "**Risiko:** gering. **Rückfall:** altes Bundle liegt auf lb-01:/etc/haproxy/certs/backup.\n\n**Test:** SSL Labs ≥ A."}); made {
		d.steps(ctx, change, "submit", "approve")
	}
	if change2, made := d.entry(ctx, "desk.change2", changes, "Upgrade PostgreSQL 15 → 16 on the reporting cluster", entryOptions{entryType: "change", assignee: sofia}); made {
		d.steps(ctx, change2, "submit")
	}
	// Two changes ask the CAB; seed-demo.sh's CAB decides one, the other waits in its approvals.
	if change3, made := d.entry(ctx, "desk.change3", changes, "Firewall-Regelwerk Standort Hamburg bereinigen (42 ungenutzte Regeln)", entryOptions{
		entryType: "change", assignee: mehmet,
		notes: "**Risiko:** mittel. **Rückfall:** Export des Regelwerks vom 01.11. liegt im Change."}); made {
		d.steps(ctx, change3, "submit", "approve")
	}
	out.coll["raised"] = raised[0]
	return out, fresh
}

// acknowledgement is the desk's macro that only answers the requester: the enabled MANUAL rule of
// the hub whose every action is a comment. Empty where the template left none.
func (d *demo) acknowledgement(ctx context.Context, hub string) string {
	listed := d.write(ctx, "automation rules", "GET", "/automation/rules", nil)
	rules, _ := listed["data"].([]any)
	for _, raw := range rules {
		rule, _ := raw.(map[string]any)
		trigger, _ := rule["trigger"].(map[string]any)
		scope, _ := rule["scope"].(map[string]any)
		if rule["enabled"] != true || trigger["kind"] != "MANUAL" || (scope != nil && str(scope["id"]) != hub) {
			continue
		}
		actions, _ := rule["actions"].([]any)
		comments := len(actions) > 0
		for _, a := range actions {
			if action, _ := a.(map[string]any); action["kind"] != "ADD_COMMENT" {
				comments = false
			}
		}
		if comments {
			return str(rule["id"])
		}
	}
	return ""
}

// demoCI is one of the desk's configuration items as a kept inventory holds it: its class's
// attributes, who answers for it (a person of seed-demo.sh, else one of its groups) and the day of
// its next review, in days from today.
type demoCI struct {
	class, name   string
	attributes    map[string]any
	person, group string
	review        int
}

// demoCIs are kept well, so the health view shows two believable findings rather than all of
// them: the Exchange connector without the host it runs on (ORPHAN) and a laptop whose review is
// overdue.
var demoCIs = []demoCI{
	{"business_service", "Kundenportal", nil, "", "Plattform-Team", 120},
	{"business_service", "E-Mail & Kalender", nil, "", "Service Desk – 1st Level", 120},
	{"application_service", "Portal-Frontend (Nginx)", map[string]any{"url": "https://portal.nordlicht.example.org"}, "", "Plattform-Team", 90},
	{"application_service", "Portal-API", map[string]any{"url": "https://api.nordlicht.example.org/v3"}, "", "Plattform-Team", 90},
	{"application_service", "Exchange Online Connector", nil, "", "Service Desk – 1st Level", 90},
	{"server", "lb-01.hh.intern", demoServer("lb-01", "CZJ3120A41", "10.20.1.10"), "", "Plattform-Team", 150},
	{"server", "app-01.hh.intern", demoServer("app-01", "CZJ3120A57", "10.20.1.21"), "", "Plattform-Team", 150},
	{"server", "app-02.hh.intern", demoServer("app-02", "CZJ3120A58", "10.20.1.22"), "", "Plattform-Team", 150},
	{"server", "db-01.hh.intern", demoServer("db-01", "CZJ3120B02", "10.20.1.30"), "", "Plattform-Team", 150},
	{"laptop", "NB-0417 (Mehmet Yılmaz)", demoLaptop("nb-0417", "PF4KX2Q7"), "Mehmet Yılmaz", "", 200},
	{"laptop", "NB-0422 (Sofia Rossi)", demoLaptop("nb-0422", "PF4KX3B1"), "Sofia Rossi", "", -12},
	{"network_device", "VPN-Gateway Frankfurt (FortiGate 200F)", map[string]any{"manufacturer": "Fortinet", "model": "FortiGate 200F",
		"serial_number": "FG200FTK22904417", "hostname": "vpn-fra-01", "ip_address": "10.30.0.1", "operating_system": "FortiOS 7.4.4"},
		"Mehmet Yılmaz", "", 60},
}

func demoServer(host, serial, ip string) map[string]any {
	return map[string]any{"manufacturer": "Dell", "model": "PowerEdge R650", "serial_number": serial, "hostname": host,
		"domain": "hh.intern", "ip_address": ip, "operating_system": "Ubuntu 22.04 LTS"}
}

func demoLaptop(host, serial string) map[string]any {
	return map[string]any{"manufacturer": "Lenovo", "model": "ThinkPad T14 Gen 4", "serial_number": serial, "hostname": host,
		"operating_system": "Windows 11 Enterprise"}
}

// keep gives a CI who answers for it and its next review, as the inventory's keeper would; the
// signed-in person where the workspace has neither the person nor the group.
func (d *demo) keep(ctx context.Context, id string, c demoCI) {
	body := map[string]any{"review_due": d.date(c.review)}
	switch {
	case d.who(c.person) != nil:
		body["owner_account_id"] = d.who(c.person)
	case d.groups[c.group] != "":
		body["owner_group_id"] = d.groups[c.group]
	case d.self != "":
		body["owner_account_id"] = d.self
	}
	d.write(ctx, "ci compliance "+c.name, "PUT", "/cis/"+id+"/compliance", body)
}

// keepWritten fills the CIs an earlier showcase wrote bare - without attributes, owner or review -
// and leaves every CI someone has kept since as it is.
func (d *demo) keepWritten(ctx context.Context, hub string) {
	var listed struct {
		Data []any `json:"data"`
	}
	if err := d.call(ctx, func(ctx context.Context) error {
		return d.c.Get(ctx, "/hubs/"+hub+"/cis", url.Values{"limit": {"500"}}, &listed)
	}); err != nil {
		d.fail("cis", err)
		return
	}
	bare := map[string]string{}
	for _, raw := range listed.Data {
		ci, _ := raw.(map[string]any)
		attributes, _ := ci["attributes"].(map[string]any)
		if len(attributes) == 0 && ci["owner_account_id"] == nil && ci["owner_group_id"] == nil && ci["review_due"] == nil {
			bare[str(ci["name"])] = str(ci["id"])
		}
	}
	for _, c := range demoCIs {
		id := bare[c.name]
		if id == "" {
			continue
		}
		if len(c.attributes) > 0 {
			d.write(ctx, "ci attributes "+c.name, "PATCH", "/cis/"+id, map[string]any{"attributes": c.attributes})
		}
		d.keep(ctx, id, c)
		d.counts["configuration items kept"]++
	}
}

// cmdb writes the desk's configuration items. They live in the desk's hub and leave with it, so
// they carry no mark of their own and are written once, with the hub; a later run only keeps the
// ones an earlier showcase left bare.
func (d *demo) cmdb(ctx context.Context, desk demoHub, fresh bool) {
	if desk.hub == "" {
		return
	}
	if !fresh {
		d.keepWritten(ctx, desk.hub)
		return
	}
	cis := map[string]string{}
	relate := func(source, target, kind string) {
		if cis[source] != "" && cis[target] != "" &&
			d.write(ctx, "relation "+kind, "POST", "/ci-relations", map[string]any{"source_ci_id": cis[source], "target_ci_id": cis[target], "type": kind}) != nil {
			d.counts["CI relations"]++
		}
	}
	for _, c := range demoCIs {
		attributes := c.attributes
		if attributes == nil {
			attributes = map[string]any{}
		}
		if made := d.write(ctx, "ci "+c.name, "POST", "/hubs/"+desk.hub+"/cis", map[string]any{"class": c.class, "name": c.name, "attributes": attributes}); made != nil {
			cis[c.name] = str(made["id"])
			d.counts["configuration items"]++
			d.keep(ctx, cis[c.name], c)
		}
	}
	for _, r := range [][3]string{
		{"Kundenportal", "Portal-Frontend (Nginx)", "depends_on"}, {"Kundenportal", "Portal-API", "depends_on"},
		{"Portal-Frontend (Nginx)", "lb-01.hh.intern", "runs_on"}, {"Portal-API", "app-01.hh.intern", "runs_on"},
		{"Portal-API", "app-02.hh.intern", "runs_on"}, {"Portal-API", "db-01.hh.intern", "depends_on"},
		{"E-Mail & Kalender", "Exchange Online Connector", "depends_on"}, {"E-Mail & Kalender", "VPN-Gateway Frankfurt (FortiGate 200F)", "depends_on"},
	} {
		relate(r[0], r[1], r[2])
	}
	for _, a := range []struct {
		name, tag, owner string
		cost             int
	}{{"NB-0417 (Mehmet Yılmaz)", "INV-2024-0417", "Mehmet Yılmaz", 148900}, {"NB-0422 (Sofia Rossi)", "INV-2024-0422", "Sofia Rossi", 152500}} {
		if cis[a.name] != "" {
			d.write(ctx, "asset "+a.name, "PUT", "/cis/"+cis[a.name]+"/asset", compact(map[string]any{"asset_tag": a.tag, "cost_minor": a.cost,
				"currency": "EUR", "purchased_on": d.date(-400), "supplier": "Bechtle AG", "warranty_until": d.date(695),
				"assigned_to": d.who(a.owner), "state": "DEPLOYED"}))
		}
	}
	if cis["Kundenportal"] != "" {
		d.write(ctx, "ci group", "PUT", "/hubs/"+desk.hub+"/ci-groups/portal_servers", map[string]any{"name": "Server des Kundenportals",
			"filter": map[string]any{"classes": []string{"server"}, "name_has": "hh.intern"}, "service_id": cis["Kundenportal"]})
	}
	if raised := desk.coll["raised"]; raised != "" && cis["Portal-Frontend (Nginx)"] != "" {
		d.write(ctx, "affected CI", "PUT", "/items/"+raised+"/cis/"+cis["Portal-Frontend (Nginx)"], nil)
	}
	if len(desk.entries) > 0 && desk.entries[0] != "" && cis["VPN-Gateway Frankfurt (FortiGate 200F)"] != "" {
		d.write(ctx, "affected CI", "PUT", "/items/"+desk.entries[0]+"/cis/"+cis["VPN-Gateway Frankfurt (FortiGate 200F)"], nil)
	}
	if change := d.held["entry.desk.change"]; change != "" && cis["lb-01.hh.intern"] != "" {
		d.write(ctx, "change CI", "PUT", "/items/"+change+"/cis/"+cis["lb-01.hh.intern"], nil)
	}
}

const demoArchitecture = `# Architekturüberblick Kunden-App

Die App spricht ausschließlich mit der **Public API v3**. Es gibt keinen direkten Zugriff auf Datenbanken.

## Bausteine

| Baustein | Verantwortung | Team |
|---|---|---|
| App (iOS/Android) | Darstellung, Offline-Warteschlange | Atlas |
| Public API | Authentifizierung, Bestellungen, Webhooks | Boreal |
| Event-Pipeline | Benachrichtigungen, Auswertungen | Boreal |

## Entscheidungen

1. Passkeys vor Passwort (siehe Sprint 24).
2. Bestellungen werden offline mit einem Idempotenzschlüssel gepuffert.
`

func (d *demo) page(ctx context.Context, key, hub string, body map[string]any) (string, bool) {
	if hub == "" {
		return "", false
	}
	return d.object(ctx, "page."+key, "PAGE", "pages", "/hubs/"+hub+"/pages", body)
}

func (d *demo) wiki(ctx context.Context, scrum string, desk demoHub, classic string) {
	root, _ := d.page(ctx, "scrum.handbook", scrum, map[string]any{"title": "Handbuch Produktentwicklung", "icon": "📘",
		"body": "Alles, was ein neues Teammitglied in der ersten Woche braucht.\n\nStart here: the pages below are kept current by both teams."})
	if root != "" {
		arch, fresh := d.page(ctx, "scrum.architecture", scrum, map[string]any{"title": "Architekturüberblick Kunden-App", "parent_id": root,
			"body": strings.Replace(demoArchitecture, "## Entscheidungen", "## Offene Fragen\n\n- Wie lange halten wir Offline-Bestellungen?", 1)})
		if fresh {
			// Two more versions: each change keeps the one before in the page's history.
			for _, body := range []string{demoArchitecture, demoArchitecture + "\n## Glossar\n\n- **Passkey** – FIDO2-Anmeldedaten auf dem Gerät.\n"} {
				if d.write(ctx, "page version", "PATCH", "/pages/"+arch, map[string]any{"body": body}) != nil {
					d.counts["page versions"]++
				}
			}
			if image := d.upload(ctx, "bausteine.png", png(320, 180, [3]byte{37, 99, 235})); image != "" {
				d.write(ctx, "page image", "POST", "/pages/"+arch+"/media", map[string]any{"media_id": image})
			}
			d.say(ctx, "Amelia Hughes", "page comment", "POST", "/pages/"+arch+"/comments", map[string]any{"body": "Could we add the sequence diagram for the passkey sign-in here?"})
		}
		d.page(ctx, "scrum.onboarding", scrum, map[string]any{
			"title":     "Onboarding for new developers – laptop, accounts, first pull request in your first three days",
			"parent_id": root,
			"body":      "## Day 1\n\n- Pick up your laptop at the IT desk (ground floor)\n- Sign in with your passkey\n\n## Day 2\n\n- Pair with your buddy on a small bug\n\n## Day 3\n\n- Open your first pull request",
			"subpages": []any{map[string]any{"title": "Lokale Entwicklungsumgebung einrichten", "body": "```bash\nmake db-up && make run\n```",
				"subpages": []any{map[string]any{"title": "Fehlerbehebung: Docker startet nicht", "body": "Docker Desktop neu starten, dann `docker info` prüfen."}}}},
		})
		if ritual, fresh := d.page(ctx, "scrum.rituals", scrum, map[string]any{"title": "Team-Rituale: Daily, Refinement, Review, Retro", "parent_id": root,
			"body": "| Termin | Wann | Dauer |\n|---|---|---|\n| Daily | täglich 9:15 | 15 min |\n| Refinement | Mi 14:00 | 60 min |\n| Review | letzter Sprinttag 13:00 | 45 min |\n| Retro | letzter Sprinttag 14:00 | 60 min |"}); fresh {
			d.say(ctx, "Lukas Brandt", "page comment", "POST", "/pages/"+ritual+"/comments", map[string]any{"body": "Retro ab Dezember bitte remote – die Hälfte des Teams ist im Homeoffice."})
		}
	}
	if kb, _ := d.page(ctx, "desk.knowledge", desk.hub, map[string]any{"title": "Wissensdatenbank Service Desk", "icon": "💡",
		"body": "Lösungen für häufige Anfragen. Artikel mit ✅ sind für das Kundenportal freigegeben."}); kb != "" {
		for i, a := range [][2]string{
			{"VPN verbindet nicht: die fünf häufigsten Ursachen", "1. Zeit auf dem Laptop falsch\n2. Zertifikat abgelaufen\n3. Hotel-WLAN blockiert UDP 4500\n4. Alte Client-Version\n5. Konto gesperrt"},
			{"How to reset your password when you are locked out", "Use *Forgot password* on the sign-in page. The link is valid for 30 minutes."},
			{"Drucker im 2. OG: Treiber und Warteschlangen", "Warteschlange `\\\\print-01\\OG2-Farbe`, Treiber Version 7.3."},
		} {
			article, fresh := d.page(ctx, fmt.Sprintf("desk.article%d", i), desk.hub, map[string]any{"title": a[0], "parent_id": kb, "body": a[1]})
			// The first two are the ones released for the portal, which its requesters read.
			if fresh && i < 2 && desk.coll["desk"] != "" {
				if d.write(ctx, "portal article", "POST", "/pages/"+article+"/articles", map[string]any{"collection_id": desk.coll["desk"]}) != nil {
					d.counts["portal articles"]++
				}
			}
		}
	}
	if minutes, _ := d.page(ctx, "classic.minutes", classic, map[string]any{"title": "Protokoll Lenkungskreis 12.09.", "icon": "📝",
		"body": "**Teilnehmende:** Lukas Brandt, Priya Natarajan, Bauherrenvertretung\n\n## Beschlüsse\n\n1. Rahmenterminplan freigegeben\n2. PV-Erweiterung wird geprüft (Änderungsantrag folgt)"}); minutes != "" {
		d.page(ctx, "classic.minutes2", classic, map[string]any{"title": "Protokoll Lenkungskreis 10.10.", "parent_id": minutes,
			"body": "## Beschlüsse\n\n1. Ausschreibung Tragwerk startet KW 44"})
	}
}

func (d *demo) board(ctx context.Context, key, hub, title string, elements []any) (string, bool) {
	if hub == "" {
		return "", false
	}
	return d.object(ctx, "board."+key, "WHITEBOARD", "whiteboards", "/hubs/"+hub+"/whiteboards", map[string]any{"title": title, "elements": elements})
}

func sticky(id string, x, y int, text, color string) map[string]any {
	return map[string]any{"id": id, "kind": "STICKY", "x": x, "y": y, "width": 180, "height": 120, "text": text, "color_token": color}
}

func (d *demo) whiteboards(ctx context.Context, scrum, kanban demoHub) {
	elements := []any{
		map[string]any{"id": "frame-retro", "kind": "FRAME", "x": 0, "y": 0, "width": 1200, "height": 700, "text": "Retro Sprint 23"},
		map[string]any{"id": "col-good", "kind": "TEXT", "x": 40, "y": 30, "text": "Was lief gut?"},
		map[string]any{"id": "col-better", "kind": "TEXT", "x": 440, "y": 30, "text": "Was können wir besser machen?"},
		map[string]any{"id": "col-actions", "kind": "TEXT", "x": 840, "y": 30, "text": "Actions"},
		sticky("s1", 40, 90, "Dark mode shipped two days early 🎉", "green"),
		sticky("s2", 40, 230, "Pairing zwischen Atlas und Boreal hat super funktioniert", "green"),
		sticky("s3", 440, 90, "Review-Termin kollidiert mit dem Release", "amber"),
		sticky("s4", 440, 230, "Flaky UI tests cost us half a day", "red"),
		sticky("s5", 840, 90, "Review eine Stunde früher legen (Lukas)", "blue"),
		map[string]any{"id": "c1", "kind": "CONNECTOR", "from": "s4", "to": "s5", "route": "CURVED", "end": "ARROW"},
	}
	for i, story := range scrum.entries {
		if i == 2 {
			break
		}
		if story != "" {
			elements = append(elements, map[string]any{"id": fmt.Sprintf("card%d", i), "kind": "CARD", "x": 40 + 220*i, "y": 420,
				"width": 200, "height": 90, "target": map[string]any{"kind": "ENTRY", "id": story}})
		}
	}
	if retro, fresh := d.board(ctx, "scrum.retro", scrum.hub, "Retro Sprint 23", elements); fresh {
		d.say(ctx, "Sofia Rossi", "whiteboard comment", "POST", "/whiteboards/"+retro+"/comments", map[string]any{"body": "Let's look at the flaky tests together on Thursday.", "element_id": "s4"})
	}
	d.board(ctx, "scrum.flow", scrum.hub, "Anmeldefluss mit Passkey (Entwurf)", []any{
		map[string]any{"id": "start", "kind": "SHAPE", "shape": "ELLIPSE", "x": 0, "y": 0, "width": 160, "height": 70, "text": "Kunde öffnet App"},
		map[string]any{"id": "q", "kind": "SHAPE", "shape": "DIAMOND", "x": 0, "y": 140, "width": 160, "height": 110, "text": "Passkey vorhanden?"},
		map[string]any{"id": "yes", "kind": "SHAPE", "shape": "ROUNDED", "x": -220, "y": 320, "width": 180, "height": 70, "text": "Face ID / Fingerabdruck"},
		map[string]any{"id": "no", "kind": "SHAPE", "shape": "ROUNDED", "x": 220, "y": 320, "width": 180, "height": 70, "text": "E-Mail-Code senden"},
		map[string]any{"id": "e1", "kind": "CONNECTOR", "from": "start", "to": "q", "end": "ARROW"},
		map[string]any{"id": "e2", "kind": "CONNECTOR", "from": "q", "to": "yes", "end": "ARROW", "route": "ELBOW"},
		map[string]any{"id": "e3", "kind": "CONNECTOR", "from": "q", "to": "no", "end": "ARROW", "route": "ELBOW"},
	})
	notes := []any{}
	for i, n := range [][2]string{{"Messe-Motto: »Weniger Tickets, mehr Zeit«", "violet"}, {"Live demo: portal in 60 seconds", "blue"},
		{"Goodies: Kaffeebecher aus Bambus", "green"}, {"Badge scanner rental?", "amber"},
		{"Standpersonal: 4 Personen pro Tag", "teal"}, {"Follow-up mail within 48 h", "blue"}} {
		notes = append(notes, sticky(fmt.Sprintf("n%d", i), (i%4)*200, (i/4)*150, n[0], n[1]))
	}
	d.board(ctx, "kanban.expo", kanban.hub, "Brainstorming IT-Expo Köln", notes)
}

// demoTime is one person's two weeks of time.
type demoTime struct {
	person, workType string
	entries          []string
}

// timeTracking records two weeks of time: last week handed in, this week's days so far recorded.
// Signed in alone, the person records the first plan as their own; under --voices each person of
// seed-demo.sh records their own and the signed-in person approves two of the weeks after.
// The kinds of work, absences and rates are the workspace's, written with the first week.
func (d *demo) timeTracking(ctx context.Context, scrum, classic, desk demoHub, mine []string) {
	for _, w := range []struct {
		key, name string
		billable  bool
	}{{"development", "Entwicklung", true}, {"meeting", "Abstimmung & Meetings", false}, {"support", "Support", true}, {"planning", "Planung", true}} {
		d.write(ctx, "work type "+w.key, "PUT", "/time-work-types/"+w.key, map[string]any{"name": w.name, "billable": w.billable})
	}
	plans := []demoTime{{"", "development", first(scrum.entries, 0, 2)}}
	if d.voices {
		plans = []demoTime{
			{"Amelia Hughes", "development", first(scrum.entries, 0, 2)},
			{"Mehmet Yılmaz", "support", append(first(scrum.entries, 2, 4), first(desk.entries, 0, 1)...)},
			{"Sofia Rossi", "development", first(scrum.entries, 1, 3)},
			{"Priya Natarajan", "planning", first(classic.entries, 1, 4)},
		}
	}
	lastMonday := d.monday(-7)
	freshWeek := false
	for _, plan := range plans {
		if d.recordWeeks(ctx, plan, lastMonday) {
			freshWeek = true
		}
	}
	if d.voices && len(mine) > 0 {
		// The person it runs as records their own time too, on their own work: a Today without
		// it reads 0:00 for the one meant to see what exists.
		d.recordWeeks(ctx, demoTime{"", "planning", mine[:1]}, lastMonday)
	}
	if !freshWeek {
		return
	}
	if d.voices {
		for _, name := range []string{"Amelia Hughes", "Mehmet Yılmaz"} {
			if id := d.who(name); id != nil {
				d.later(ctx, "approve "+name, "POST", "/timesheets/"+d.date(lastMonday)+":approve", map[string]any{"account_id": id})
			}
		}
	}
	if sofia := d.who("Sofia Rossi"); sofia != nil {
		d.write(ctx, "absence", "POST", "/time-absences", map[string]any{"account_id": sofia, "from": d.date(10), "to": d.date(14), "kind": "LEAVE"})
	}
	if jonas := d.who("Jonas Weber"); jonas != nil {
		d.write(ctx, "absence", "POST", "/time-absences", map[string]any{"account_id": jonas, "from": d.date(3), "to": d.date(3), "kind": "TRAINING"})
	}
	for _, r := range []struct {
		name string
		rate int
	}{{"Amelia Hughes", 9500}, {"Mehmet Yılmaz", 8200}, {"Sofia Rossi", 8800}, {"Priya Natarajan", 10500}} {
		if id := d.who(r.name); id != nil {
			d.write(ctx, "cost rate", "POST", "/cost-rates", map[string]any{"account_id": id, "hour_minor": r.rate, "currency": "EUR", "from": d.date(-365)})
		}
	}
	if len(plans[0].entries) > 0 && plans[0].entries[0] != "" {
		d.say(ctx, "Amelia Hughes", "timer", "POST", "/timer:start",
			map[string]any{"entry_id": plans[0].entries[0], "work_type": "development", "note": "Passkey-Fehlerfälle"})
	}
}

// recordWeeks records one plan's two weeks and hands the first in; it answers whether anything
// was made.
func (d *demo) recordWeeks(ctx context.Context, plan demoTime, lastMonday int) bool {
	entries := plan.entries
	if len(entries) == 0 || entries[0] == "" {
		return false
	}
	prefix := "time."
	if plan.person != "" {
		prefix = "time." + strings.ToLower(strings.Fields(plan.person)[0]) + "."
	}
	fresh := false
	minutes := []int{90, 150, 45, 210, 120}
	notes := []string{"Implementierung", "Code review", "Pairing session", "Tests und Dokumentation", "Abstimmung mit dem Fachbereich"}
	record := func(key string, body map[string]any) {
		if _, made := d.spokenObject(ctx, plan.person, prefix+key, "TIME_RECORD", "time records", "/time-records", body); made {
			fresh = true
		}
	}
	for day := range 5 {
		for n, entry := range entries {
			if entry == "" {
				continue
			}
			record(fmt.Sprintf("last.d%d.e%d", day, n), map[string]any{
				"entry_id": entry, "work_type": plan.workType, "day": d.date(lastMonday + day), "minutes": minutes[(day+n)%5],
				"note": notes[(day*2+n)%5], "billable": true, "zone": "Europe/Berlin"})
		}
		record(fmt.Sprintf("last.d%d.daily", day), map[string]any{
			"work_type": "meeting", "day": d.date(lastMonday + day), "minutes": 30, "note": "Daily", "zone": "Europe/Berlin"})
	}
	thisMonday := d.monday(0)
	for day := 0; thisMonday+day <= 0 && day < 5; day++ {
		record("this."+strings.ReplaceAll(d.date(thisMonday+day), "-", ""), map[string]any{"entry_id": entries[0],
			"work_type": plan.workType, "day": d.date(thisMonday + day), "minutes": 120, "note": "Weiterarbeit", "zone": "Europe/Berlin"})
	}
	if fresh && d.say(ctx, plan.person, "timesheet", "POST", "/timesheets/"+d.date(lastMonday)+":submit", nil) != nil {
		d.counts["timesheets submitted"]++
	}
	return fresh
}

func first(ids []string, from, to int) []string {
	if from >= len(ids) {
		return nil
	}
	return ids[from:min(to, len(ids))]
}

func (d *demo) goals(ctx context.Context, scrum, kanban, classic, desk demoHub) {
	year := d.today.Year()
	period := d.period(ctx, fmt.Sprintf("H2 %d", year), fmt.Sprintf("%d-07-01", year), fmt.Sprintf("%d-12-31", year))
	if period == "" {
		return
	}
	lukas, amelia, priya, mehmet := d.who("Lukas Brandt"), d.who("Amelia Hughes"), d.who("Priya Natarajan"), d.who("Mehmet Yılmaz")
	company, companyFresh := d.goal(ctx, "company", map[string]any{"period_id": period, "title": "Kundinnen und Kunden lösen ihr Anliegen selbst – ohne Anruf",
		"owner_id": lukas, "status": "ON_TRACK", "description": "Self-service first: portal, app and knowledge base answer the common questions.",
		"key_results": []any{
			map[string]any{"title": "Anteil Anfragen über das Portal", "kind": "PERCENT", "start": 22, "target": 60, "current": 41, "unit": "%"},
			map[string]any{"title": "Average first response time (hours)", "kind": "NUMBER", "start": 9, "target": 2, "current": 4.5, "unit": "h"},
			map[string]any{"title": "Passkey-Anmeldung live", "kind": "BINARY", "start": 0, "target": 1, "current": 0},
		}})
	appGoal, appFresh := d.goal(ctx, "company.app", map[string]any{"period_id": period, "title": "App rating of 4.6 stars or better in both stores",
		"owner_id": amelia, "parent_id": company, "status": "AT_RISK", "hub_id": scrum.hub,
		"key_results": []any{map[string]any{"title": "Crash-free sessions", "kind": "PERCENT", "start": 97.1, "target": 99.5, "current": 98.2, "unit": "%"}}})
	d.goal(ctx, "company.desk", map[string]any{"period_id": period, "title": "Keine P1-Störung länger als vier Stunden offen",
		"owner_id": mehmet, "parent_id": company, "status": "ON_TRACK", "hub_id": desk.hub})
	build, buildFresh := d.goal(ctx, "build", map[string]any{"period_id": period, "title": "Logistikzentrum im Zeit- und Kostenrahmen (± 5 %)",
		"owner_id": priya, "status": "OFF_TRACK", "hub_id": classic.hub})
	if appFresh && len(scrum.entries) >= 3 {
		d.write(ctx, "goal entries", "POST", "/goals/"+appGoal+"/entries", map[string]any{"entry_ids": scrum.entries[:3]})
		d.say(ctx, "Amelia Hughes", "check-in", "POST", "/goals/"+appGoal+"/check-ins", map[string]any{"status": "AT_RISK", "confidence": 5,
			"note": "The push-link crash costs us; fix is in Sprint 24."})
	}
	if buildFresh && len(classic.entries) >= 4 {
		d.write(ctx, "goal entries", "POST", "/goals/"+build+"/entries", map[string]any{"entry_ids": classic.entries[:4]})
		d.say(ctx, "Priya Natarajan", "check-in", "POST", "/goals/"+build+"/check-ins", map[string]any{"status": "OFF_TRACK", "confidence": 3,
			"note": "Stahlpreise +11 %, Lieferverzug 3 Wochen. Gegenmaßnahmen im Lenkungskreis."})
	}
	if companyFresh {
		d.write(ctx, "check-in", "POST", "/goals/"+company+"/check-ins", map[string]any{"status": "ON_TRACK", "confidence": 7,
			"note": "Portalanteil steigt seit der neuen Startseite deutlich."})
	}

	items := []any{}
	for _, it := range []struct {
		hub        string
		start, end int
		status     string
	}{{scrum.hub, -90, 60, "ON_TRACK"}, {classic.hub, -60, 221, "OFF_TRACK"}, {desk.hub, -200, 200, "ON_TRACK"}, {kanban.hub, -30, 45, "AT_RISK"}} {
		if it.hub != "" {
			items = append(items, map[string]any{"container_id": it.hub, "start_on": d.date(it.start), "end_on": d.date(it.end), "status": it.status})
		}
	}
	portfolio, fresh := d.object(ctx, "portfolio.strategy", "PORTFOLIO", "portfolios", "/portfolios", map[string]any{"name": "Strategische Vorhaben 2026",
		"owner_id": lukas, "status": "AT_RISK", "description": "Everything the management board reviews monthly.", "items": items})
	if fresh {
		d.write(ctx, "portfolio update", "POST", "/portfolios/"+portfolio+"/updates", map[string]any{"status": "AT_RISK",
			"summary": "Bau im Verzug (Stahl), App und Service Desk im Plan. Entscheidung zur PV-Erweiterung im November."})
	}
	if portfolio != "" {
		digital := []any{}
		for _, it := range items {
			row, _ := it.(map[string]any)
			if hub := str(row["container_id"]); hub == scrum.hub || hub == desk.hub {
				digital = append(digital, it)
			}
		}
		d.object(ctx, "portfolio.strategy.digital", "PORTFOLIO", "portfolios", "/portfolios", map[string]any{"name": "Digital customer experience",
			"owner_id": amelia, "parent_id": portfolio, "status": "ON_TRACK", "items": digital})
	}

	widgets := []any{
		map[string]any{"title": "Offene Einträge nach Typ", "chart": "BAR", "source": "QUERY", "x": 0, "y": 0, "w": 6, "h": 4,
			"query": map[string]any{"scope": map[string]any{"workspace": true}, "filter": map[string]any{"op": "EQ", "field": "is_completed", "value": false},
				"aggregate": map[string]any{"function": "COUNT", "group_by": "entry_type"}}},
		map[string]any{"title": "Entries completed per week", "chart": "LINE", "source": "QUERY", "x": 6, "y": 0, "w": 6, "h": 4,
			"query": map[string]any{"scope": map[string]any{"workspace": true}, "filter": map[string]any{"op": "EQ", "field": "is_completed", "value": true},
				"aggregate": map[string]any{"function": "COUNT", "bucket": map[string]any{"field": "completed_at", "period": "week", "week_start": "monday"}}}},
	}
	if company != "" && appGoal != "" && build != "" {
		widgets = append(widgets, map[string]any{"title": "Ziele H2", "chart": "PROGRESS", "source": "GOALS", "x": 0, "y": 4, "w": 6, "h": 3,
			"goal_ids": []string{company, appGoal, build}})
	}
	if desk.hub != "" {
		widgets = append(widgets, map[string]any{"title": "Incidents by priority", "chart": "PIE", "source": "QUERY", "x": 6, "y": 4, "w": 6, "h": 3,
			"query": map[string]any{"scope": map[string]any{"container_id": desk.hub, "include_descendants": true},
				"aggregate": map[string]any{"function": "COUNT", "group_by": "custom_fields.priority"}}})
	}
	d.object(ctx, "dashboard.board", "DASHBOARD", "dashboards", "/dashboards", map[string]any{"name": "Geschäftsführung – Wochenüberblick", "sharing": "SCOPE", "widgets": widgets})
	if scrum.hub != "" {
		d.object(ctx, "dashboard.atlas", "DASHBOARD", "dashboards", "/dashboards", map[string]any{"name": "Team Atlas", "sharing": "SCOPE", "hub_id": scrum.hub,
			"widgets": []any{map[string]any{"title": "Open stories", "chart": "NUMBER", "source": "QUERY", "x": 0, "y": 0, "w": 4, "h": 2,
				"query": map[string]any{"scope": map[string]any{"container_id": scrum.hub, "include_descendants": true},
					"filter": map[string]any{"op": "EQ", "field": "is_completed", "value": false}, "aggregate": map[string]any{"function": "COUNT"}}}}})
	}
}

func (d *demo) goal(ctx context.Context, key string, body map[string]any) (string, bool) {
	return d.object(ctx, "goal."+key, "GOAL", "goals", "/goals", body)
}

// period answers the goal period of that name, made where the workspace has none. A period is a
// frame, not content: no mark names it, and removing the showcase leaves it to a person.
func (d *demo) period(ctx context.Context, name, start, end string) string {
	var listed struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := d.call(ctx, func(ctx context.Context) error { return d.c.Get(ctx, "/goal-periods", nil, &listed) }); err == nil {
		for _, p := range listed.Data {
			if p.Name == name {
				return p.ID
			}
		}
	}
	made := d.write(ctx, "goal period", "POST", "/goal-periods", map[string]any{"name": name, "start_on": start, "end_on": end})
	if made == nil {
		return ""
	}
	d.counts["goal periods"]++
	return str(made["id"])
}
