# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The demo workspace's content, written through the product's own REST operations - never SQL - so
# that every row arrives the way a person's would: authorised, audited, validated. Called by
# scripts/seed-demo.sh, which says how to run it; the standard library only, like the other
# scripts here.
#
# Each product is one section. A section that is refused is reported and the next one runs: a
# demo with one product missing is still worth walking, and the closing summary names what is
# missing and why, with the server's own problem code. The run exits non-zero when anything was
# refused, so nobody mistakes a partial demo for a whole one.

import datetime as dt
import json
import os
import struct
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zlib

API = os.environ["SEED_API"].rstrip("/") + "/api/v1"
PASSWORD = os.environ["SEED_PASSWORD"]
EMPTY = os.environ.get("SEED_EMPTY") == "true"
SLUG = os.environ["SEED_SLUG"]
DOMAIN = os.environ.get("SEED_MAIL_DOMAIN", "example.org")
TIMEOUT = 60  # rule 7: no call without a deadline

NOW = dt.datetime.now(dt.timezone.utc).replace(microsecond=0)
TODAY = NOW.date()


def iso(when):
    return when.strftime("%Y-%m-%dT%H:%M:%SZ")


def day(offset, hour=9):
    """A moment `offset` days from today at a working hour, UTC."""
    d = TODAY + dt.timedelta(days=offset)
    return iso(dt.datetime(d.year, d.month, d.day, hour, 0, tzinfo=dt.timezone.utc))


def date(offset):
    return (TODAY + dt.timedelta(days=offset)).isoformat()


def week_of(offset_days):
    """The week a day falls in, as the timesheet path names it: its Monday."""
    d = TODAY + dt.timedelta(days=offset_days)
    return (d - dt.timedelta(days=d.weekday())).isoformat()


def say(text):
    print(text, flush=True)


class Refused(Exception):
    def __init__(self, method, path, status, problem):
        self.status, self.problem = status, problem
        code = (problem.get("detail_code") or problem.get("code") or "") if isinstance(problem, dict) else ""
        super().__init__(f"{method} {path} -> {status} {code} {json.dumps(problem)[:400]}")


class Client:
    """One person's session against the workspace, refreshed when the access token runs out."""

    def __init__(self, tenant_id, tokens):
        self.tenant_id = tenant_id
        self.access = tokens["access_token"]
        self.refresh_token = tokens["refresh_token"]
        self.expires = time.time() + 12 * 60

    def _refresh(self):
        answer = raw("POST", "/auth/sessions:refresh", {"refresh_token": self.refresh_token}, tenant=self.tenant_id)
        self.access, self.refresh_token = answer["access_token"], answer["refresh_token"]
        self.expires = time.time() + 12 * 60

    def call(self, method, path, body=None, *, ctype="application/json", headers=None, data=None):
        if time.time() > self.expires:
            self._refresh()
        h = {"Authorization": f"Bearer {self.access}"}
        h.update(headers or {})
        return raw(method, path, body, tenant=self.tenant_id, ctype=ctype, headers=h, data=data)

    get = lambda self, path: self.call("GET", path)  # noqa: E731
    post = lambda self, path, body=None, **kw: self.call("POST", path, body if body is not None else {}, **kw)  # noqa: E731
    put = lambda self, path, body=None, **kw: self.call("PUT", path, body, **kw)  # noqa: E731
    patch = lambda self, path, body, **kw: self.call("PATCH", path, body, ctype="application/merge-patch+json", **kw)  # noqa: E731


def raw(method, path, body=None, *, tenant=None, ctype="application/json", headers=None, data=None, attempt=0):
    url = path if path.startswith("http") else API + path
    h = {"Accept": "application/json"}
    if tenant:
        h["X-Hubtask-Tenant"] = tenant
    if method in ("POST", "PUT", "PATCH", "DELETE"):
        # A fresh key per call: the seed never repeats a write on purpose, and a retry below reuses it.
        h["Idempotency-Key"] = h.get("Idempotency-Key") or str(uuid.uuid4())
    h.update(headers or {})
    payload = data
    if payload is None and body is not None:
        payload = json.dumps(body).encode()
        h["Content-Type"] = ctype
    elif data is not None:
        h["Content-Type"] = ctype
    req = urllib.request.Request(url, data=payload, method=method, headers=h)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            text = resp.read()
            etag = resp.headers.get("ETag")
            if not text:
                return {"_etag": etag} if etag else {}
            out = json.loads(text)
            if isinstance(out, dict) and etag:
                out["_etag"] = etag
            return out
    except urllib.error.HTTPError as err:
        text = err.read()
        try:
            problem = json.loads(text)
        except ValueError:
            problem = {"body": text[:200].decode(errors="replace")}
        if err.code in (429, 503) and attempt < 3:
            wait = float(err.headers.get("Retry-After") or 2)
            time.sleep(min(wait, 30))
            return raw(method, path, body, tenant=tenant, ctype=ctype, headers=headers, data=data, attempt=attempt + 1)
        raise Refused(method, path, err.code, problem) from None


failures = []
counts = {}


def section(name):
    def wrap(fn):
        def run(*args, **kwargs):
            say(f"- {name}")
            try:
                return fn(*args, **kwargs)
            except Refused as err:
                failures.append(f"{name}: {err}")
                say(f"    refused: {err}")
            except Exception as err:  # noqa: BLE001 - a broken section must not stop the next
                failures.append(f"{name}: {type(err).__name__}: {err}")
                say(f"    failed: {type(err).__name__}: {err}")
            return None
        return run
    return wrap


def tried(what, fn, *args, **kwargs):
    """One write inside a section that may be refused without losing the rest of the section."""
    try:
        return fn(*args, **kwargs)
    except Refused as err:
        failures.append(f"{what}: {err}")
        say(f"    refused ({what}): {err}")
        return None


def count(kind, n=1):
    counts[kind] = counts.get(kind, 0) + n


def png(width, height, rgb):
    """A small solid PNG, drawn here so the demo needs no image file and no library."""
    row = b"\x00" + bytes(rgb) * width
    raw_data = row * height
    def chunk(kind, data):
        c = struct.pack(">I", len(data)) + kind + data
        return c + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(raw_data, 9)) + chunk(b"IEND", b""))


# ---------------------------------------------------------------------------- the workspace

def provision():
    """A new workspace through the control plane, its owner's password set by redeeming the token."""
    admin = os.environ["SEED_ADMIN_TOKEN"]
    body = {"slug": SLUG, "display_name": os.environ.get("SEED_NAME") or SLUG.capitalize(),
            "owner_email": f"owner@{SLUG}.{DOMAIN}", "owner_display_name": "Jana Krüger",
            "default_locale": "de", "default_time_zone": "Europe/Berlin"}
    made = raw("POST", "/admin/tenants", body, headers={"Authorization": f"Bearer {admin}"})
    tokens = raw("POST", "/auth/invitations:redeem", {"token": made["owner_redemption_token"], "password": PASSWORD})
    return made, Client(made["id"], tokens)


def finish_onboarding(client, account_id):
    """The tour covers the first screen of every fresh account; the demo is walked, not toured."""
    client.patch(f"/accounts/{account_id}/preferences", {"onboarding_completed_at": iso(NOW)})


PEOPLE = [
    # display name, local part, workspace role, locale
    ("Lukas Brandt", "lukas.brandt", "ADMIN", "de"),
    ("Amelia Hughes", "amelia.hughes", "MEMBER", "en"),
    ("Mehmet Yılmaz", "mehmet.yilmaz", "MEMBER", "de"),
    ("Sofia Rossi", "sofia.rossi", "MEMBER", "en"),
    ("Jonas Weber", "jonas.weber", "CONTRIBUTOR", "de"),
    ("Priya Natarajan", "priya.natarajan", "MEMBER", "en"),
    ("Clara Neumann", "clara.neumann", "VIEWER", "de"),
    ("Thomas Becker", "thomas.becker", "AUDITOR", "de"),
]


@section("people and their roles")
def people(client, ws):
    """Invited the way an administrator invites; each invitation redeemed with the demo password."""
    for name, local, role, locale in PEOPLE:
        email = f"{local}@{SLUG}.{DOMAIN}"
        made = tried(f"invite {name}", client.post, "/accounts:invite", {"email": email, "display_name": name})
        if not made:
            continue
        ws["people"][local] = made["id"]
        link = made.get("invitation_link")
        if link:
            token = urllib.parse.parse_qs(urllib.parse.urlparse(link).query).get("token", [""])[0] \
                or urllib.parse.urlparse(link).fragment.split("token=")[-1]
            redeemed = tried(f"redeem {name}", raw, "POST", "/auth/invitations:redeem", {"token": token, "password": PASSWORD})
            if redeemed:
                person = Client(ws["tenant_id"], redeemed)
                ws["sessions"][local] = person
                tried(f"preferences {name}", person.patch, f"/accounts/{made['id']}/preferences",
                      {"locale": locale, "time_zone": "Europe/Berlin", "onboarding_completed_at": iso(NOW)})
        # An invitation grants nothing by itself: the role on the workspace is its own grant.
        tried(f"role {name}", client.post, "/memberships",
              {"account_id": made["id"], "scope_type": "TENANT", "role": role})
        count("people")
    for gname, members, desc in [
        ("Service Desk – 1st Level", ["mehmet.yilmaz", "sofia.rossi"], "Nimmt Störungen und Anfragen an."),
        ("Change Advisory Board", ["lukas.brandt", "priya.natarajan"], "Approves normal and emergency changes."),
        ("Plattform-Team", ["amelia.hughes", "jonas.weber", "mehmet.yilmaz"], None),
    ]:
        ids = [ws["people"][m] for m in members if m in ws["people"]]
        made = tried(f"group {gname}", client.post, "/groups", {"name": gname, "description": desc, "members": ids})
        if made:
            ws["groups"][gname] = made["id"]
            count("groups")


# ---------------------------------------------------------------------------- templates and hubs

def take(client, key, *, hub_id=None, collection_id=None, answers=None, samples=False):
    """A template taken the way the gallery takes it: a plan first, then exactly that plan applied."""
    body = {"answers": answers} if answers else {}
    if hub_id:
        body["hub_id"] = hub_id
    if collection_id:
        body["collection_id"] = collection_id
    if samples:
        body["samples"] = True
    plan = client.post(f"/config/templates/{key}:plan", body)
    if not plan.get("plan_hash"):
        raise Refused("PLAN", f"/config/templates/{key}", 422, {"refusals": plan.get("refusals")})
    apply = {"document": plan["document"], "mode": plan.get("mode") or "MERGE", "plan_hash": plan["plan_hash"],
             "template": plan["template"], "acknowledge": True}
    if plan.get("hub_id"):
        apply["hub_id"] = plan["hub_id"]
    if plan.get("collection_id"):
        apply["collection_id"] = plan["collection_id"]
    return client.post("/config:apply", apply)


def hub(client, name, *, description=None, icon=None, color=None, preset=None):
    body = {"type": "HUB", "name": name, "description": description, "icon": icon, "color_token": color}
    if preset:
        body["preset"] = preset
    count("hubs")
    return client.post("/containers", body)["id"]


def collection(client, parent, name, *, description=None, preset=None, icon=None):
    body = {"type": "COLLECTION", "parent_id": parent, "name": name, "description": description, "icon": icon}
    if preset:
        body["preset"] = preset
    count("collections")
    return client.post("/containers", body)["id"]


MODULES = ["WORKFLOWS", "PLANNING", "KNOWLEDGE", "WHITEBOARD", "SERVICE_DESK", "ASSETS", "TIME", "GOALS", "AGENTS"]


@section("modules and templates")
def templates(client, ws):
    """Every product switched on, then the four templates taken by the workspace.

    The switches come first because a template's plan reads the sections it changes, and a section
    whose module is off is withheld from that read (config.section_withheld)."""
    for module in MODULES:
        tried(f"module {module}", client.put, f"/tenant/modules/{module}", {"switch": "ON"})
    answers = {"first_group": ws["groups"].get("Service Desk – 1st Level"),
               "cab_group": ws["groups"].get("Change Advisory Board")}
    for key in ["scrum_team", "kanban_team", "classic_project", "itil_service_desk"]:
        if tried(f"template {key}", take, client, key,
                 answers={k: v for k, v in answers.items() if v} if key == "itil_service_desk" else None):
            ws["templates"].append(key)
            count("templates")


def entry(client, collection_id, title, *, entry_type=None, kind="TASK", parent=None, notes=None,
          assignee=None, start=None, due=None, fields=None, labels=None, language=None):
    body = {"type": kind, "title": title, "collection_id": collection_id}
    if parent:
        body = {"type": kind, "title": title, "parent_id": parent}
    for key, value in [("entry_type", entry_type), ("notes", notes), ("assignee_id", assignee), ("start_at", start),
                       ("due_at", due), ("custom_fields", fields), ("label_ids", labels), ("content_language", language)]:
        if value:
            body[key] = value
    made = client.post("/items", body)
    count("entries")
    return made["id"]


def steps(client, item_id, *keys):
    for key in keys:
        client.post(f"/items/{item_id}:step", {"step": key})


def comment(client, item_id, text, *, internal=False):
    count("comments")
    return client.post(f"/items/{item_id}/comments", {"body": text, "internal": internal})["id"]


def link(client, source, target, kind):
    count("links")
    return client.post(f"/items/{source}/links", {"target_id": target, "type": kind})


def labels(client, container_id, spec):
    out = {}
    for name, color in spec:
        made = tried(f"label {name}", client.post, f"/containers/{container_id}/labels", {"name": name, "color_token": color})
        if made:
            out[name] = made["id"]
    return out


def person(ws, local):
    """The session of a person, or the owner's where that person could not be signed in."""
    return ws["sessions"].get(local) or ws["owner"]


def who(ws, local):
    return ws["people"].get(local)


# ---------------------------------------------------------------------------- Scrum: product development

@section("Scrum hub: product development")
def scrum(client, ws):
    h = hub(client, "Produktentwicklung", description="Zwei Scrum-Teams bauen die Kunden-App und die Plattform dahinter.",
            icon="rocket", color="blue")
    ws["hubs"]["scrum"] = h
    tried("hub template scrum_team", take, client, "scrum_team", hub_id=h)
    tried("membership Jonas", client.post, "/memberships",
          {"account_id": who(ws, "jonas.weber"), "scope_type": "HUB", "scope_id": h, "role": "MEMBER"})
    app = collection(client, h, "Team Atlas – Mobile App", description="iOS- und Android-App für Endkundinnen und Endkunden.")
    api = collection(client, h, "Team Boreal – Platform & API", description="Public REST API, billing and the event pipeline.")
    ws["collections"].update({"app": app, "api": api})
    lab = labels(client, app, [("Kundenwunsch", "violet"), ("Tech-Schuld", "orange"), ("Barrierefreiheit", "teal"), ("Sicherheit", "red")])
    lukas, amelia, mehmet, sofia, jonas = (who(ws, k) for k in ["lukas.brandt", "amelia.hughes", "mehmet.yilmaz", "sofia.rossi", "jonas.weber"])

    stories = []
    login = entry(client, app, "Als Kundin möchte ich mich mit Passkey anmelden, damit ich mir kein Passwort mehr merken muss",
                  entry_type="story", assignee=amelia, labels=[lab.get("Kundenwunsch"), lab.get("Sicherheit")],
                  notes="## Hintergrund\n\n38 % der Support-Tickets im letzten Quartal betrafen vergessene Passwörter.\n\n"
                        "## Akzeptanzkriterien\n\n- Passkey lässt sich in den Einstellungen hinzufügen\n- Anmeldung ohne Passwort auf iOS 17+ und Android 14+\n"
                        "- Fallback auf E-Mail-Code bleibt erhalten", due=day(9))
    stories.append(login)
    wp = entry(client, None, "Passkey-Registrierung im Profil", kind="WORK_PACKAGE", parent=login, assignee=amelia)
    for title in ["WebAuthn-Challenge vom Server holen", "Systemdialog auf iOS anbinden", "Systemdialog auf Android anbinden",
                  "Fehlerfälle: abgebrochen, kein Gerät, Zeitüberschreitung"]:
        entry(client, None, title, kind="ACTIVITY", parent=wp)
    wp2 = entry(client, None, "Anmeldung mit Passkey", kind="WORK_PACKAGE", parent=login, assignee=mehmet)
    for title in ["Login-Screen um Passkey-Button erweitern", "Telemetrie: Anteil Passkey-Anmeldungen messen"]:
        entry(client, None, title, kind="ACTIVITY", parent=wp2)
    steps(client, login, "plan", "start")
    comment(person(ws, "mehmet.yilmaz"), login, "Android 13 kann Passkeys nur mit Google Play Services ≥ 23.x – sollen wir das als Mindestversion festlegen?")
    comment(person(ws, "amelia.hughes"), login, "Yes, let's require it; the share of Android 13 without current Play Services is under 2 % in our analytics.")

    offline = entry(client, app, "Offline mode: orders placed without a connection are queued and sent once the phone is back online",
                    entry_type="story", assignee=sofia, labels=[lab.get("Kundenwunsch")], due=day(16),
                    notes="Customers in rural areas lose their basket when the train enters a tunnel. Queue the order locally, "
                          "show a clear *pending* state and send it within 30 seconds of reconnecting.")
    stories.append(offline)
    steps(client, offline, "plan")
    for title, kind in [("Local queue with retry and idempotency key", "WORK_PACKAGE"), ("Pending badge in the order list", "WORK_PACKAGE")]:
        entry(client, None, title, kind=kind, parent=offline)

    darkmode = entry(client, app, "Dunkelmodus folgt der Systemeinstellung", entry_type="story", assignee=jonas,
                     labels=[lab.get("Barrierefreiheit")])
    stories.append(darkmode)
    steps(client, darkmode, "plan", "start", "review", "finish")
    a11y = entry(client, app, "Screenreader liest Preise inklusive Währung vor (VoiceOver und TalkBack)", entry_type="story",
                 labels=[lab.get("Barrierefreiheit")], assignee=sofia)
    stories.append(a11y)
    steps(client, a11y, "plan", "start", "review")
    bug1 = entry(client, app, "App stürzt beim Öffnen eines Push-Links ab, wenn die App vorher beendet war", entry_type="bug",
                 assignee=mehmet, labels=[lab.get("Tech-Schuld")],
                 notes="Reproduzierbar auf Pixel 8, Android 15. Stacktrace im Crash-Reporter: `NullPointerException in DeepLinkRouter.resolve`.")
    steps(client, bug1, "plan", "start")
    comment(person(ws, "sofia.rossi"), bug1, "Seen on iOS too? I could not reproduce it on an iPhone 15 with iOS 18.1.")
    bug2 = entry(client, app, "Checkout button hidden behind the keyboard on small screens", entry_type="bug", assignee=jonas)
    stories.append(bug2)
    link(client, bug1, login, "blocks")
    link(client, offline, darkmode, "depends_on")

    platform = []
    for title, kind_, who_ in [
        ("Rate limiting per API key with a clear Retry-After header", "story", amelia),
        ("Webhooks: Zustellungen bei 5xx exponentiell wiederholen und nach 24 h aufgeben", "story", lukas),
        ("Billing: invoices as PDF/A-3 with embedded ZUGFeRD XML", "story", mehmet),
        ("Event-Pipeline: Partitionierung nach Mandant statt nach Zeit", "story", amelia),
        ("Pagination cursor breaks when two entries share a timestamp", "bug", lukas),
    ]:
        made = entry(client, api, title, entry_type=kind_, assignee=who_)
        platform.append(made)
    steps(client, platform[0], "plan", "start", "review")
    steps(client, platform[1], "plan", "start")
    steps(client, platform[4], "plan", "start", "review", "finish")
    link(client, platform[3], platform[1], "blocks")
    comment(person(ws, "lukas.brandt"), platform[1], "Wir sollten die Wiederholungen im Audit sichtbar machen, sonst fragt der Support jedes Mal nach.")
    comment(person(ws, "amelia.hughes"), platform[0], "Draft of the limits: 600 requests/minute per key, burst 1200. @Lukas please sanity-check before Friday.")

    # Sprints: one finished, one running, one planned; estimates in points.
    for i, story in enumerate(stories + platform):
        tried("estimate", client.put, f"/items/{story}/estimate", {"unit": "POINTS", "value": [3, 5, 8, 2, 13, 1, 5, 3, 8][i % 9]})
    s1 = tried("sprint 23", client.post, f"/collections/{app}/sprints",
               {"name": "Sprint 23 – Barrierefreiheit", "goal": "Dunkelmodus und Screenreader-Preise ausliefern",
                "start_on": date(-21), "end_on": date(-8), "unit": "POINTS"})
    if s1:
        tried("sprint 23 entries", client.post, f"/sprints/{s1['id']}/entries", {"item_ids": [darkmode, a11y]})
        tried("sprint 23 start", client.post, f"/sprints/{s1['id']}:start")
        tried("sprint 23 close", client.post, f"/sprints/{s1['id']}:close", {})
        count("sprints")
    s2 = tried("sprint 24", client.post, f"/collections/{app}/sprints",
               {"name": "Sprint 24 – Passkeys", "goal": "Passwortlose Anmeldung für alle Kundinnen und Kunden",
                "start_on": date(-7), "end_on": date(6), "unit": "POINTS"})
    if s2:
        tried("sprint 24 entries", client.post, f"/sprints/{s2['id']}/entries", {"item_ids": [login, bug1, a11y]})
        tried("sprint 24 start", client.post, f"/sprints/{s2['id']}:start")
        count("sprints")
    s3 = tried("sprint 25", client.post, f"/collections/{app}/sprints",
               {"name": "Sprint 25 – Offline", "start_on": date(7), "end_on": date(20), "unit": "POINTS"})
    if s3:
        tried("sprint 25 entries", client.post, f"/sprints/{s3['id']}/entries", {"item_ids": [offline, bug2]})
        count("sprints")
    rel = tried("release 4.2", client.post, f"/hubs/{h}/releases",
                {"name": "App 4.2 „Schlüssellos“", "description": "Passkeys, dark mode and screen reader prices.", "release_on": date(14)})
    if rel:
        tried("release entries", client.post, f"/releases/{rel['id']}/entries", {"item_ids": [login, darkmode, a11y]})
        count("releases")
    rel2 = tried("release api", client.post, f"/hubs/{h}/releases",
                 {"name": "API v3.8", "release_on": date(30)})
    if rel2:
        tried("release api entries", client.post, f"/releases/{rel2['id']}/entries", {"item_ids": platform[:3]})
        count("releases")
    ws["entries"]["scrum"] = stories + platform


# ---------------------------------------------------------------------------- Kanban: marketing

@section("Kanban hub: marketing")
def kanban(client, ws):
    h = hub(client, "Marketing & Kommunikation", description="Content, Kampagnen und Messen – ein Board, das alle sehen.",
            icon="megaphone", color="violet")
    ws["hubs"]["kanban"] = h
    tried("hub template kanban_team", take, client, "kanban_team", hub_id=h)
    board = collection(client, h, "Content-Board Q4", description="Blog, newsletter, social and the trade fair in November.")
    ws["collections"]["board"] = board
    lab = labels(client, board, [("Blog", "blue"), ("Newsletter", "green"), ("Social", "magenta"), ("Messe", "orange")])
    sofia, priya, jonas = who(ws, "sofia.rossi"), who(ws, "priya.natarajan"), who(ws, "jonas.weber")
    cards = [
        ("Fallstudie: Wie die Stadtwerke Lüneburg ihre Störungsannahme von 3 Tagen auf 4 Stunden verkürzt haben", "Blog", priya, ["pull", "review"]),
        ("Newsletter November: Rückblick Herbstmesse, neue Integrationen, Webinar-Termine", "Newsletter", sofia, ["pull"]),
        ("LinkedIn carousel: five signs your service desk has outgrown spreadsheets", "Social", jonas, ["pull", "review", "finish"]),
        ("Messestand IT-Expo Köln: Standplan, Bildschirme, Demo-Laptops und Give-aways", "Messe", priya, []),
        ("Webinar: Running ITIL change management without a 40-page process manual", "Blog", sofia, []),
        ("Pressemitteilung zur Version 4.2 abstimmen (Freigabe durch Geschäftsführung)", None, priya, ["pull"]),
        ("Short video: setting up a Kanban board in under two minutes", "Social", jonas, []),
    ]
    made = []
    for title, label, assignee, walk in cards:
        cid = entry(client, board, title, entry_type="card", assignee=assignee,
                    labels=[lab[label]] if label in lab else None, due=day(len(made) * 3 + 2))
        if walk:
            steps(client, cid, *walk)
        made.append(cid)
    comment(person(ws, "priya.natarajan"), made[0], "Freigabe der Stadtwerke liegt vor, Zitat von Frau Albers ist abgestimmt.")
    comment(person(ws, "sofia.rossi"), made[1], "Webinar dates: 19 and 26 November, both 10:00 CET.")
    comment(person(ws, "jonas.weber"), made[2], "Bildschirme für die Messe sind bestellt, Lieferung KW 46. Das Karussell ist live, 1.240 Impressionen am ersten Tag.")
    ws["entries"]["kanban"] = made


# ---------------------------------------------------------------------------- classic project: construction

@section("classic project hub: new logistics centre")
def classic(client, ws):
    h = hub(client, "Neubau Logistikzentrum Hamburg-Billbrook", icon="building", color="orange",
            description="Klassisches Projekt mit Phasen, Meilensteinen, Risiken und Änderungsanträgen. Budget 18,4 Mio. €.")
    ws["hubs"]["classic"] = h
    tried("hub template classic_project", take, client, "classic_project", hub_id=h)
    plan = collection(client, h, "Projektplan", description="Phasen nach HOAI, Abhängigkeiten und Meilensteine.")
    risks = collection(client, h, "Risiken & Offene Punkte")
    ws["collections"].update({"plan": plan, "risks": risks})
    lukas, priya, mehmet = who(ws, "lukas.brandt"), who(ws, "priya.natarajan"), who(ws, "mehmet.yilmaz")

    phases = [
        ("Grundlagenermittlung und Vorplanung", -60, -35, ["start", "accept"], lukas),
        ("Entwurfs- und Genehmigungsplanung", -34, -5, ["start", "accept"], priya),
        ("Bauantrag einreichen", -4, -4, ["start", "accept"], lukas),
        ("Ausführungsplanung Tragwerk, TGA und Fördertechnik", -3, 40, ["start"], priya),
        ("Ausschreibung und Vergabe der Gewerke", 20, 70, [], mehmet),
        ("Rohbau Halle A und B", 71, 180, [], None),
        ("Inbetriebnahme Fördertechnik und Probebetrieb", 181, 220, [], None),
        ("Übergabe an den Betrieb", 221, 221, [], lukas),
    ]
    ids = []
    for title, start, end, walk, assignee in phases:
        did = entry(client, plan, title, entry_type="deliverable", assignee=assignee, start=day(start, 8), due=day(end, 17))
        if walk:
            tried(f"steps {title}", steps, client, did, *walk)
        ids.append(did)
    for milestone in (2, 7):
        tried("milestone", client.put, f"/items/{ids[milestone]}/scheduling", {"milestone": True, "mode": "MANUAL"})
    for title in ["Statik Hallendach (Spannweite 42 m)", "Sprinkleranlage nach VdS CEA 4001", "Hochregallager 24 m – Regalbediengeräte"]:
        entry(client, None, title, kind="WORK_PACKAGE", parent=ids[3])
    sub = entry(client, None, "Lüftung und Wärmerückgewinnung", kind="WORK_PACKAGE", parent=ids[3])
    for title in ["Lastannahmen mit dem Betreiber abstimmen", "Variantenvergleich Wärmepumpe vs. Fernwärme", "Kosten nach DIN 276 schätzen"]:
        entry(client, None, title, kind="ACTIVITY", parent=sub)
    # The Gantt's dependencies: finish-to-start with lags where the building authority needs time.
    for before, after, kind_, lag in [(0, 1, "FS", 0), (1, 2, "FS", 0), (2, 3, "SS", 1), (3, 4, "SS", 20), (4, 5, "FS", 0), (5, 6, "FS", 0), (6, 7, "FS", 0)]:
        made = tried("dependency", link, client, ids[after], ids[before], "depends_on")
        if made and made.get("id"):
            tried("dependency timing", client.put, f"/links/{made['id']}/timing", {"dependency_kind": kind_, "lag_days": lag})
    tried("baseline", client.post, f"/plans/{plan}/baselines", {"name": "Freigegebener Rahmenterminplan (Lenkungskreis 12.09.)"})
    count("baselines")
    plan_read = tried("plan budget read", client.get, f"/plans/{plan}/budget")
    if plan_read is not None:
        tried("plan budget", client.put, f"/plans/{plan}/budget",
              {"minutes": 4800 * 60, "amount_minor": 1_840_000_000, "currency": "EUR", "expected_version": plan_read.get("version", 0)})

    for title, prob, impact, walk in [
        ("Lieferverzug Stahlträger wegen Engpass im Walzwerk", "High", "High", ["mitigate"]),
        ("Baugrund: Altlasten im Bereich der ehemaligen Tankstelle", "Medium", "High", []),
        ("Fachkräftemangel Elektro – Nachunternehmer springt ab", "Medium", "Medium", ["mitigate"]),
        ("Genehmigung verzögert sich durch Einwendungen der Nachbarschaft", "Low", "High", ["close"]),
    ]:
        rid = entry(client, risks, title, entry_type="risk", fields={"probability": prob, "impact": impact}, assignee=priya)
        if walk:
            tried("risk steps", steps, client, rid, *walk)
    issue = entry(client, risks, "Zufahrt für Schwerlastverkehr während der Bauphase ungeklärt", entry_type="issue", assignee=lukas)
    comment(person(ws, "lukas.brandt"), issue, "Termin mit dem Bezirksamt Hamburg-Mitte am Donnerstag, 10 Uhr. Unterlagen liegen im Wiki.")
    cr = entry(client, risks, "Änderungsantrag: zusätzliche Photovoltaik auf Halle B (420 kWp)", entry_type="change_request",
               notes="Mehrkosten ca. 380.000 €, Amortisation laut Gutachten nach 7,5 Jahren.")
    tried("change request steps", steps, client, cr, "assess")
    ws["entries"]["classic"] = ids


# ---------------------------------------------------------------------------- ITIL: the service desk

REQUESTERS = [
    # name, address, organisation key
    ("Hannah Albers", "hannah.albers@stadtwerke-lueneburg.example", "stadtwerke"),
    ("Felix Hartmann", "felix.hartmann@stadtwerke-lueneburg.example", "stadtwerke"),
    ("Grace O'Neill", "grace.oneill@nordwind-logistics.example", "nordwind"),
]


def invite_and_redeem(client, ws, name, email):
    made = client.post("/accounts:invite", {"email": email, "display_name": name})
    link_ = made.get("invitation_link") or ""
    token = urllib.parse.urlparse(link_).fragment.split("token=")[-1]
    session = Client(ws["tenant_id"], raw("POST", "/auth/invitations:redeem", {"token": token, "password": PASSWORD}))
    tried(f"preferences {name}", session.patch, f"/accounts/{made['id']}/preferences", {"onboarding_completed_at": iso(NOW)})
    return made["id"], session


@section("ITIL hub: service desk, SLAs, approvals, organisations")
def service_desk(client, ws):
    h = hub(client, "IT-Service-Desk", description="Störungen, Serviceanfragen, Probleme und Changes nach ITIL 4.",
            icon="life-buoy", color="teal")
    ws["hubs"]["desk"] = h
    desk = collection(client, h, "Störungen & Anfragen", description="Was über das Portal, per Mail oder am Telefon ankommt.")
    changes = collection(client, h, "Changes")
    problems = collection(client, h, "Probleme")
    # After the collections: the template's sample incidents land in the hub's first one.
    tried("hub template itil_service_desk", take, client, "itil_service_desk", hub_id=h, samples=True)
    ws["collections"].update({"desk": desk, "changes": changes, "problems": problems})
    cab = ws["groups"].get("Change Advisory Board")
    if cab:
        tried("cab role", client.post, "/role-grants", {"role": "cab_member", "scope_type": "HUB", "scope_id": h, "group_id": cab})

    for key, name, domains, shares in [("stadtwerke", "Stadtwerke Lüneburg GmbH", ["stadtwerke-lueneburg.example"], True),
                                       ("nordwind", "Nordwind Logistics Ltd.", ["nordwind-logistics.example"], False)]:
        tried(f"organisation {key}", client.put, f"/requester-organisations/{key}",
              {"name": name, "domains": domains, "shares_requests": shares})
        count("organisations")
    tried("requester role", client.post, "/roles", {"key": "customer", "name": "Kunde (Portal)", "permissions": ["REQUEST"]})

    mehmet, sofia = who(ws, "mehmet.yilmaz"), who(ws, "sofia.rossi")
    requests_ = [
        (0, "incident", "Seit dem Update lässt sich das Kundenportal in Firefox nicht mehr öffnen",
         "Fehlermeldung: »Die Verbindung wurde zurückgesetzt«. In Edge funktioniert es. Betroffen sind alle 14 Arbeitsplätze im Kundenzentrum."),
        (1, "service_request", "Neuer Zugang für Auszubildende ab 1. Dezember (3 Personen)", "Bitte Zugänge mit Rolle »Sachbearbeitung« anlegen."),
        (2, "incident", "Delivery notes are printed without the barcode since this morning",
         "Warehouse 2 in Felixstowe. Printer model Zebra ZT411, the test page prints fine."),
        (0, "service_request", "Lizenz für das Planungswerkzeug auf eine weitere Person übertragen", None),
    ]
    raised = []
    sessions = []
    for name, email, org in REQUESTERS:
        made = tried(f"requester {name}", invite_and_redeem, client, ws, name, email)
        if not made:
            sessions.append(None)
            continue
        account_id, session = made
        tried(f"requester grant {name}", client.post, "/role-grants",
              {"role": "customer", "scope_type": "COLLECTION", "scope_id": desk, "account_id": account_id})
        sessions.append(session)
        count("requesters")
    for who_, kind_, title, notes in requests_:
        session = sessions[who_]
        if not session:
            continue
        body = {"collection_id": desk, "entry_type": kind_, "title": title}
        if notes:
            body["notes"] = notes
        made = tried(f"portal request {title[:30]}", session.post, "/requests", body)
        if made:
            raised.append((made.get("id") or made.get("item_id"), session))
            count("portal requests")
    if raised:
        first = raised[0][0]
        tried("assign", client.post, f"/items/{first}:assign", {"account_id": mehmet})
        for key, value in [("impact", "High"), ("urgency", "High"), ("priority", "P1")]:
            tried(f"triage {key}", client.put, f"/items/{first}/custom-fields/{key}", {"value": value})
        tried("start", steps, client, first, "start")
        comment(person(ws, "mehmet.yilmaz"), first, "Danke für die Meldung – wir sehen das auch intern. Ein Zertifikat der Zwischen-CA fehlt in der Kette, wir tauschen es heute bis 14 Uhr.")
        comment(person(ws, "mehmet.yilmaz"), first, "Intern: betrifft auch das Partnerportal, Problem-Ticket angelegt.", internal=True)
        tried("requester reply", raised[0][1].post, f"/requests/{first}/comments", {"body": "Vielen Dank, wir warten ab und melden uns, falls es danach nicht geht."})
        if len(raised) > 2:
            third = raised[2][0]
            tried("assign", client.post, f"/items/{third}:assign", {"account_id": sofia})
            tried("resolve", steps, client, third, "start", "resolve")
            comment(person(ws, "sofia.rossi"), third, "The label template lost its barcode field in last night's update; restored it, please try again.")
    for title, impact, urgency, priority, assignee in [
        ("VPN-Gateway Frankfurt antwortet sporadisch nicht (Paketverlust 30 %)", "High", "Medium", "P2", mehmet),
        ("Shared mailbox 'invoices@' stopped receiving mail from the ERP", "Medium", "High", "P2", sofia),
        ("Drucker 2. OG druckt nur leere Seiten", "Low", "Low", "P4", None),
    ]:
        iid = entry(client, desk, title, entry_type="incident", assignee=assignee,
                    fields={"impact": impact, "urgency": urgency, "priority": priority})
        ws["entries"].setdefault("incidents", []).append(iid)

    problem = entry(client, problems, "Wiederkehrende Zertifikatsfehler nach automatischer Erneuerung", entry_type="problem", assignee=mehmet,
                    notes="Drei Störungen in sechs Wochen mit derselben Ursache: die Zwischenzertifikate werden bei der Erneuerung nicht mitgeliefert.")
    tried("problem steps", steps, client, problem, "investigate", "record_known_error")
    change = entry(client, changes, "Zertifikatskette im Load Balancer um Zwischen-CA ergänzen (Wartungsfenster Do 22–23 Uhr)",
                   entry_type="change", assignee=mehmet,
                   notes="**Risiko:** gering. **Rückfall:** altes Bundle liegt auf lb-01:/etc/haproxy/certs/backup.\n\n**Test:** SSL Labs ≥ A.")
    tried("change submit", steps, client, change, "submit", "approve")
    change2 = entry(client, changes, "Upgrade PostgreSQL 15 → 16 on the reporting cluster", entry_type="change", assignee=sofia)
    tried("change2 submit", steps, client, change2, "submit")
    ws["entries"]["change"] = change
    # The CAB decides as itself: the approval waits for one of its members.
    approver = person(ws, "priya.natarajan")
    waiting = tried("approvals", approver.get, "/approvals")
    for approval in (waiting or {}).get("data", [])[:1]:
        tried("approve", approver.post, f"/approvals/{approval['id']}:decide",
              {"verdict": "APPROVE", "reason": "Geringes Risiko, Rückfall dokumentiert. Freigegeben."})
        count("approvals")
    ws["entries"]["problem"] = problem
    ws["entries"]["raised"] = [r[0] for r in raised]


# ---------------------------------------------------------------------------- CMDB

@section("CMDB: configuration items, relations, a service")
def cmdb(client, ws):
    h = ws["hubs"].get("desk") or ws["default_hub"]

    def ci(cls, name, **attributes):
        made = client.post(f"/hubs/{h}/cis", {"class": cls, "name": name, "attributes": attributes})
        ws["cis"][name] = made["id"]
        count("configuration items")
        return made["id"]

    def relate(source, target, kind):
        tried(f"relation {kind}", client.post, "/ci-relations",
              {"source_ci_id": ws["cis"][source], "target_ci_id": ws["cis"][target], "type": kind})
        count("CI relations")

    ci("business_service", "Kundenportal")
    ci("business_service", "E-Mail & Kalender")
    ci("application_service", "Portal-Frontend (Nginx)")
    ci("application_service", "Portal-API")
    ci("application_service", "Exchange Online Connector")
    ci("server", "lb-01.hh.intern")
    ci("server", "app-01.hh.intern")
    ci("server", "app-02.hh.intern")
    ci("server", "db-01.hh.intern")
    ci("laptop", "NB-0417 (Mehmet Yılmaz)")
    ci("laptop", "NB-0422 (Sofia Rossi)")
    ci("network_device", "VPN-Gateway Frankfurt (FortiGate 200F)")
    relate("Kundenportal", "Portal-Frontend (Nginx)", "depends_on")
    relate("Kundenportal", "Portal-API", "depends_on")
    relate("Portal-Frontend (Nginx)", "lb-01.hh.intern", "runs_on")
    relate("Portal-API", "app-01.hh.intern", "runs_on")
    relate("Portal-API", "app-02.hh.intern", "runs_on")
    relate("Portal-API", "db-01.hh.intern", "depends_on")
    relate("E-Mail & Kalender", "Exchange Online Connector", "depends_on")
    relate("E-Mail & Kalender", "VPN-Gateway Frankfurt (FortiGate 200F)", "depends_on")
    for name, tag, cost, owner in [("NB-0417 (Mehmet Yılmaz)", "INV-2024-0417", 148900, "mehmet.yilmaz"),
                                   ("NB-0422 (Sofia Rossi)", "INV-2024-0422", 152500, "sofia.rossi")]:
        tried(f"asset {name}", client.put, f"/cis/{ws['cis'][name]}/asset",
              {"asset_tag": tag, "cost_minor": cost, "currency": "EUR", "purchased_on": date(-400), "supplier": "Bechtle AG",
               "warranty_until": date(695), "assigned_to": who(ws, owner), "state": "DEPLOYED"})
    tried("ci group", client.put, f"/hubs/{h}/ci-groups/portal_servers",
          {"name": "Server des Kundenportals", "filter": {"classes": ["server"], "name_has": "hh.intern"},
           "service_id": ws["cis"]["Kundenportal"]})
    raised = ws["entries"].get("raised") or []
    if raised:
        tried("affected CI", client.put, f"/items/{raised[0]}/cis/{ws['cis']['Portal-Frontend (Nginx)']}")
    for incident in (ws["entries"].get("incidents") or [])[:1]:
        tried("affected CI", client.put, f"/items/{incident}/cis/{ws['cis']['VPN-Gateway Frankfurt (FortiGate 200F)']}")
    if ws["entries"].get("change"):
        tried("change CI", client.put, f"/items/{ws['entries']['change']}/cis/{ws['cis']['lb-01.hh.intern']}")


# ---------------------------------------------------------------------------- wiki

def upload(client, name, data, content_type, usage="ATTACHMENT"):
    staged = client.post("/media", {"file_name": name, "content_type": content_type, "size": len(data), "usage": usage})
    target = staged.get("upload") or {}
    url = target.get("url")
    if not url:
        raise Refused("UPLOAD", "/media", 0, {"answer": {k: v for k, v in staged.items() if k != "_etag"}})
    raw(target.get("method", "PUT"), url, data=data, ctype=content_type, tenant=client.tenant_id,
        headers={k: v for k, v in (target.get("headers") or {}).items()})
    client.post(f"/media/{staged['id']}:confirm")
    count("images")
    return staged["id"]

ARCHITECTURE = """# Architekturüberblick Kunden-App

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
"""


@section("wiki: pages, versions, images, comments")
def wiki(client, ws):
    for key in ("scrum", "desk", "classic"):
        h = ws["hubs"].get(key)
        if not h:
            continue
        if key == "scrum":
            root = client.post(f"/hubs/{h}/pages", {"title": "Handbuch Produktentwicklung", "icon": "📘",
                               "body": "Alles, was ein neues Teammitglied in der ersten Woche braucht.\n\nStart here: the pages below are kept current by both teams."})
            arch = client.post(f"/hubs/{h}/pages", {"title": "Architekturüberblick Kunden-App", "parent_id": root["id"],
                               "body": ARCHITECTURE.replace("## Entscheidungen", "## Offene Fragen\n\n- Wie lange halten wir Offline-Bestellungen?")})
            count("pages", 2)
            # Three versions: each change keeps the one before in the page's history.
            etag = arch.get("_etag")
            for body in [ARCHITECTURE, ARCHITECTURE + "\n## Glossar\n\n- **Passkey** – FIDO2-Anmeldedaten auf dem Gerät.\n"]:
                changed = client.patch(f"/pages/{arch['id']}", {"body": body}, headers={"If-Match": etag} if etag else None)
                etag = changed.get("_etag")
                count("page versions")
            image = tried("diagram upload", upload, client, "bausteine.png", png(320, 180, (37, 99, 235)), "image/png")
            if image:
                tried("page image", client.post, f"/pages/{arch['id']}/media", {"media_id": image})
            client.post(f"/hubs/{h}/pages", {
                "title": "Onboarding for new developers – laptop, accounts, first pull request in your first three days",
                "parent_id": root["id"],
                "body": "## Day 1\n\n- Pick up your laptop at the IT desk (ground floor)\n- Sign in with your passkey\n\n## Day 2\n\n- Pair with your buddy on a small bug\n\n## Day 3\n\n- Open your first pull request",
                "subpages": [{"title": "Lokale Entwicklungsumgebung einrichten", "body": "```bash\nmake db-up && make run\n```",
                              "subpages": [{"title": "Fehlerbehebung: Docker startet nicht", "body": "Docker Desktop neu starten, dann `docker info` prüfen."}]}],
            })
            count("pages", 3)
            ritual = client.post(f"/hubs/{h}/pages", {"title": "Team-Rituale: Daily, Refinement, Review, Retro", "parent_id": root["id"],
                                 "body": "| Termin | Wann | Dauer |\n|---|---|---|\n| Daily | täglich 9:15 | 15 min |\n| Refinement | Mi 14:00 | 60 min |\n| Review | letzter Sprinttag 13:00 | 45 min |\n| Retro | letzter Sprinttag 14:00 | 60 min |"})
            count("pages")
            tried("page comment", person(ws, "amelia.hughes").post, f"/pages/{arch['id']}/comments",
                  {"body": "Could we add the sequence diagram for the passkey sign-in here?"})
            tried("page comment", person(ws, "lukas.brandt").post, f"/pages/{ritual['id']}/comments",
                  {"body": "Retro ab Dezember bitte remote – die Hälfte des Teams ist im Homeoffice."})
            count("page comments", 2)
            ws["pages"]["architecture"] = arch["id"]
        elif key == "desk":
            root = client.post(f"/hubs/{h}/pages", {"title": "Wissensdatenbank Service Desk", "icon": "💡",
                               "body": "Lösungen für häufige Anfragen. Artikel mit ✅ sind für das Kundenportal freigegeben."})
            for title, body in [
                ("VPN verbindet nicht: die fünf häufigsten Ursachen", "1. Zeit auf dem Laptop falsch\n2. Zertifikat abgelaufen\n3. Hotel-WLAN blockiert UDP 4500\n4. Alte Client-Version\n5. Konto gesperrt"),
                ("How to reset your password when you are locked out", "Use *Forgot password* on the sign-in page. The link is valid for 30 minutes."),
                ("Drucker im 2. OG: Treiber und Warteschlangen", "Warteschlange `\\\\print-01\\OG2-Farbe`, Treiber Version 7.3."),
            ]:
                client.post(f"/hubs/{h}/pages", {"title": title, "parent_id": root["id"], "body": body})
            count("pages", 4)
        elif key == "classic":
            minutes = client.post(f"/hubs/{h}/pages", {"title": "Protokoll Lenkungskreis 12.09.", "icon": "📝",
                                  "body": "**Teilnehmende:** Lukas Brandt, Priya Natarajan, Bauherrenvertretung\n\n## Beschlüsse\n\n1. Rahmenterminplan freigegeben\n2. PV-Erweiterung wird geprüft (Änderungsantrag folgt)"})
            client.post(f"/hubs/{h}/pages", {"title": "Protokoll Lenkungskreis 10.10.", "parent_id": minutes["id"],
                        "body": "## Beschlüsse\n\n1. Ausschreibung Tragwerk startet KW 44"})
            count("pages", 2)


# ---------------------------------------------------------------------------- whiteboards

@section("whiteboards")
def whiteboards(client, ws):
    h = ws["hubs"].get("scrum")
    if h:
        stories = (ws["entries"].get("scrum") or [])[:2]
        elements = [
            {"id": "frame-retro", "kind": "FRAME", "x": 0, "y": 0, "width": 1200, "height": 700, "text": "Retro Sprint 23"},
            {"id": "col-good", "kind": "TEXT", "x": 40, "y": 30, "text": "Was lief gut?"},
            {"id": "col-better", "kind": "TEXT", "x": 440, "y": 30, "text": "Was können wir besser machen?"},
            {"id": "col-actions", "kind": "TEXT", "x": 840, "y": 30, "text": "Actions"},
            {"id": "s1", "kind": "STICKY", "x": 40, "y": 90, "width": 180, "height": 120, "text": "Dark mode shipped two days early 🎉", "color_token": "green"},
            {"id": "s2", "kind": "STICKY", "x": 40, "y": 230, "width": 180, "height": 120, "text": "Pairing zwischen Atlas und Boreal hat super funktioniert", "color_token": "green"},
            {"id": "s3", "kind": "STICKY", "x": 440, "y": 90, "width": 180, "height": 120, "text": "Review-Termin kollidiert mit dem Release", "color_token": "amber"},
            {"id": "s4", "kind": "STICKY", "x": 440, "y": 230, "width": 180, "height": 120, "text": "Flaky UI tests cost us half a day", "color_token": "red"},
            {"id": "s5", "kind": "STICKY", "x": 840, "y": 90, "width": 180, "height": 120, "text": "Review eine Stunde früher legen (Lukas)", "color_token": "blue"},
            {"id": "c1", "kind": "CONNECTOR", "from": "s4", "to": "s5", "route": "CURVED", "end": "ARROW"},
        ]
        for i, story in enumerate(stories):
            elements.append({"id": f"card{i}", "kind": "CARD", "x": 40 + 220 * i, "y": 420, "width": 200, "height": 90,
                             "target": {"kind": "ENTRY", "id": story}})
        board = client.post(f"/hubs/{h}/whiteboards", {"title": "Retro Sprint 23", "elements": elements})
        count("whiteboards")
        tried("whiteboard comment", person(ws, "sofia.rossi").post, f"/whiteboards/{board['id']}/comments",
              {"body": "Let's look at the flaky tests together on Thursday.", "element_id": "s4"})
        flow = [
            {"id": "start", "kind": "SHAPE", "shape": "ELLIPSE", "x": 0, "y": 0, "width": 160, "height": 70, "text": "Kunde öffnet App"},
            {"id": "q", "kind": "SHAPE", "shape": "DIAMOND", "x": 0, "y": 140, "width": 160, "height": 110, "text": "Passkey vorhanden?"},
            {"id": "yes", "kind": "SHAPE", "shape": "ROUNDED", "x": -220, "y": 320, "width": 180, "height": 70, "text": "Face ID / Fingerabdruck"},
            {"id": "no", "kind": "SHAPE", "shape": "ROUNDED", "x": 220, "y": 320, "width": 180, "height": 70, "text": "E-Mail-Code senden"},
            {"id": "e1", "kind": "CONNECTOR", "from": "start", "to": "q", "end": "ARROW"},
            {"id": "e2", "kind": "CONNECTOR", "from": "q", "to": "yes", "end": "ARROW", "route": "ELBOW"},
            {"id": "e3", "kind": "CONNECTOR", "from": "q", "to": "no", "end": "ARROW", "route": "ELBOW"},
        ]
        client.post(f"/hubs/{h}/whiteboards", {"title": "Anmeldefluss mit Passkey (Entwurf)", "elements": flow})
        count("whiteboards")
    h = ws["hubs"].get("kanban")
    if h:
        notes = [{"id": f"n{i}", "kind": "STICKY", "x": (i % 4) * 200, "y": (i // 4) * 150, "width": 180, "height": 120,
                  "text": text, "color_token": color}
                 for i, (text, color) in enumerate([("Messe-Motto: »Weniger Tickets, mehr Zeit«", "violet"), ("Live demo: portal in 60 seconds", "blue"),
                                                    ("Goodies: Kaffeebecher aus Bambus", "green"), ("Badge scanner rental?", "amber"),
                                                    ("Standpersonal: 4 Personen pro Tag", "teal"), ("Follow-up mail within 48 h", "blue")])]
        client.post(f"/hubs/{h}/whiteboards", {"title": "Brainstorming IT-Expo Köln", "elements": notes})
        count("whiteboards")


# ---------------------------------------------------------------------------- time

@section("time: records, timesheets, absences, rates")
def time_tracking(client, ws):
    for key, name, billable in [("development", "Entwicklung", True), ("meeting", "Abstimmung & Meetings", False),
                                ("support", "Support", True), ("planning", "Planung", True)]:
        tried(f"work type {key}", client.put, f"/time-work-types/{key}", {"name": name, "billable": billable})
    scrum = ws["entries"].get("scrum") or []
    classic = ws["entries"].get("classic") or []
    incidents = ws["entries"].get("incidents") or []
    plan = [
        ("amelia.hughes", scrum[:2], "development"), ("mehmet.yilmaz", scrum[2:4] + incidents[:1], "support"),
        ("sofia.rossi", scrum[1:3], "development"), ("priya.natarajan", classic[1:4], "planning"),
    ]
    monday_last = -(TODAY.weekday()) - 7  # the Monday of last week, as an offset from today
    for local, entries, work_type in plan:
        session = person(ws, local)
        if not entries:
            continue
        for d in range(5):
            for n, entry_id in enumerate(entries):
                minutes = [90, 150, 45, 210, 120][(d + n) % 5]
                note = ["Implementierung", "Code review", "Pairing session", "Tests und Dokumentation", "Abstimmung mit dem Fachbereich"][(d * 2 + n) % 5]
                if tried("time record", session.post, "/time-records",
                         {"entry_id": entry_id, "work_type": work_type, "day": date(monday_last + d), "minutes": minutes,
                          "note": note, "billable": work_type != "meeting", "zone": "Europe/Berlin"}):
                    count("time records")
            tried("meeting", session.post, "/time-records",
                  {"work_type": "meeting", "day": date(monday_last + d), "minutes": 30, "note": "Daily", "zone": "Europe/Berlin"})
        # Last week handed in; this week's days so far recorded, not yet handed in.
        if tried(f"timesheet {local}", session.post, f"/timesheets/{week_of(monday_last)}:submit"):
            count("timesheets submitted")
        for d in range(min(TODAY.weekday(), 4) + 1):
            tried("time record", session.post, "/time-records",
                  {"entry_id": entries[0], "work_type": work_type, "day": date(-(TODAY.weekday()) + d), "minutes": 120,
                   "note": "Weiterarbeit", "zone": "Europe/Berlin"})
    for local in ("amelia.hughes", "mehmet.yilmaz"):
        if who(ws, local) and tried(f"approve {local}", client.post, f"/timesheets/{week_of(monday_last)}:approve", {"account_id": who(ws, local)}):
            count("timesheets approved")
    tried("absence", client.post, "/time-absences", {"account_id": who(ws, "sofia.rossi"), "from": date(10), "to": date(14), "kind": "LEAVE"})
    tried("absence", client.post, "/time-absences", {"account_id": who(ws, "jonas.weber"), "from": date(3), "to": date(3), "kind": "TRAINING"})
    for local, rate in [("amelia.hughes", 9500), ("mehmet.yilmaz", 8200), ("sofia.rossi", 8800), ("priya.natarajan", 10500)]:
        tried(f"cost rate {local}", client.post, "/cost-rates",
              {"account_id": who(ws, local), "hour_minor": rate, "currency": "EUR", "from": date(-365)})
    tried("timer", person(ws, "amelia.hughes").post, "/timer:start", {"entry_id": scrum[0], "work_type": "development", "note": "Passkey-Fehlerfälle"} if scrum else {})


# ---------------------------------------------------------------------------- goals, portfolios, dashboards

@section("goals, portfolios and dashboards")
def goals(client, ws):
    period = client.post("/goal-periods", {"name": "H2 2026", "start_on": f"{TODAY.year}-07-01", "end_on": f"{TODAY.year}-12-31"})
    lukas, amelia, priya, mehmet = (who(ws, k) for k in ["lukas.brandt", "amelia.hughes", "priya.natarajan", "mehmet.yilmaz"])
    company = client.post("/goals", {"period_id": period["id"], "title": "Kundinnen und Kunden lösen ihr Anliegen selbst – ohne Anruf",
                                     "owner_id": lukas, "status": "ON_TRACK",
                                     "description": "Self-service first: portal, app and knowledge base answer the common questions.",
                                     "key_results": [
                                         {"title": "Anteil Anfragen über das Portal", "kind": "PERCENT", "start": 22, "target": 60, "current": 41, "unit": "%"},
                                         {"title": "Average first response time (hours)", "kind": "NUMBER", "start": 9, "target": 2, "current": 4.5, "unit": "h"},
                                         {"title": "Passkey-Anmeldung live", "kind": "BINARY", "start": 0, "target": 1, "current": 0},
                                     ]})
    count("goals")
    app_goal = client.post("/goals", {"period_id": period["id"], "title": "App rating of 4.6 stars or better in both stores",
                                      "owner_id": amelia, "parent_id": company["id"], "status": "AT_RISK", "hub_id": ws["hubs"].get("scrum"),
                                      "key_results": [{"title": "Crash-free sessions", "kind": "PERCENT", "start": 97.1, "target": 99.5, "current": 98.2, "unit": "%"}]})
    count("goals")
    client.post("/goals", {"period_id": period["id"], "title": "Keine P1-Störung länger als vier Stunden offen",
                                       "owner_id": mehmet, "parent_id": company["id"], "status": "ON_TRACK", "hub_id": ws["hubs"].get("desk")})
    count("goals")
    build = client.post("/goals", {"period_id": period["id"], "title": "Logistikzentrum im Zeit- und Kostenrahmen (± 5 %)", "owner_id": priya,
                                   "status": "OFF_TRACK", "hub_id": ws["hubs"].get("classic")})
    count("goals")
    if ws["entries"].get("scrum"):
        tried("goal entries", client.post, f"/goals/{app_goal['id']}/entries", {"entry_ids": ws["entries"]["scrum"][:3]})
    if ws["entries"].get("classic"):
        tried("goal entries", client.post, f"/goals/{build['id']}/entries", {"entry_ids": ws["entries"]["classic"][:4]})
    krs = {kr["title"]: kr["id"] for kr in company.get("key_results", [])}
    tried("check-in", client.post, f"/goals/{company['id']}/check-ins",
          {"status": "ON_TRACK", "confidence": 7, "note": "Portalanteil steigt seit der neuen Startseite deutlich.",
           "values": [{"key_result_id": krs["Anteil Anfragen über das Portal"], "value": 44}] if "Anteil Anfragen über das Portal" in krs else []})
    tried("check-in", person(ws, "amelia.hughes").post, f"/goals/{app_goal['id']}/check-ins",
          {"status": "AT_RISK", "confidence": 5, "note": "The push-link crash costs us; fix is in Sprint 24."})
    tried("check-in", person(ws, "priya.natarajan").post, f"/goals/{build['id']}/check-ins",
          {"status": "OFF_TRACK", "confidence": 3, "note": "Stahlpreise +11 %, Lieferverzug 3 Wochen. Gegenmaßnahmen im Lenkungskreis."})
    count("check-ins", 3)

    items = [{"container_id": ws["hubs"][k], "start_on": date(s), "end_on": date(e), "status": st}
             for k, s, e, st in [("scrum", -90, 60, "ON_TRACK"), ("classic", -60, 221, "OFF_TRACK"), ("desk", -200, 200, "ON_TRACK"),
                                 ("kanban", -30, 45, "AT_RISK")] if k in ws["hubs"]]
    portfolio = client.post("/portfolios", {"name": "Strategische Vorhaben 2026", "owner_id": lukas, "status": "AT_RISK",
                                            "description": "Everything the management board reviews monthly.", "items": items})
    count("portfolios")
    tried("portfolio update", client.post, f"/portfolios/{portfolio['id']}/updates",
          {"status": "AT_RISK", "summary": "Bau im Verzug (Stahl), App und Service Desk im Plan. Entscheidung zur PV-Erweiterung im November."})
    client.post("/portfolios", {"name": "Digital customer experience", "owner_id": amelia, "parent_id": portfolio["id"], "status": "ON_TRACK",
                                "items": [i for i in items if i["container_id"] in (ws["hubs"].get("scrum"), ws["hubs"].get("desk"))]})
    count("portfolios")

    widgets = [
        {"title": "Offene Einträge nach Typ", "chart": "BAR", "source": "QUERY", "x": 0, "y": 0, "w": 6, "h": 4,
         "query": {"scope": {"workspace": True}, "filter": {"op": "EQ", "field": "is_completed", "value": False},
                   "aggregate": {"function": "COUNT", "group_by": "entry_type"}}},
        {"title": "Entries completed per week", "chart": "LINE", "source": "QUERY", "x": 6, "y": 0, "w": 6, "h": 4,
         "query": {"scope": {"workspace": True}, "filter": {"op": "EQ", "field": "is_completed", "value": True},
                   "aggregate": {"function": "COUNT", "bucket": {"field": "completed_at", "period": "week", "week_start": "monday"}}}},
        {"title": "Ziele H2", "chart": "PROGRESS", "source": "GOALS", "x": 0, "y": 4, "w": 6, "h": 3,
         "goal_ids": [company["id"], app_goal["id"], build["id"]]},
    ]
    if ws["hubs"].get("desk"):
        widgets.append({"title": "Incidents by priority", "chart": "PIE", "source": "QUERY", "x": 6, "y": 4, "w": 6, "h": 3,
                        "query": {"scope": {"container_id": ws["hubs"]["desk"], "include_descendants": True},
                                  "aggregate": {"function": "COUNT", "group_by": "custom_fields.priority"}}})
    if tried("dashboard", client.post, "/dashboards", {"name": "Geschäftsführung – Wochenüberblick", "sharing": "SCOPE", "widgets": widgets}):
        count("dashboards")
    if ws["hubs"].get("scrum") and tried("team dashboard", client.post, "/dashboards", {
            "name": "Team Atlas", "sharing": "SCOPE", "hub_id": ws["hubs"]["scrum"],
            "widgets": [{"title": "Open stories", "chart": "NUMBER", "source": "QUERY", "x": 0, "y": 0, "w": 4, "h": 2,
                         "query": {"scope": {"container_id": ws["hubs"]["scrum"], "include_descendants": True},
                                   "filter": {"op": "EQ", "field": "is_completed", "value": False}, "aggregate": {"function": "COUNT"}}}]}):
        count("dashboards")


# ---------------------------------------------------------------------------- agents

@section("registered agents")
def agents(client, ws):
    model = client.post("/agent-models", {"key": "local-mistral", "name": "Mistral Small (eigener Server, Frankfurt)", "source": "OWN",
                                          "model": "mistral-small-2501", "region": "SELF_HOSTED", "retention_days": 0})
    triage = client.post("/agents", {"name": "Ticket-Vorsortierung", "kind": "TICKET_TRIAGE", "owner_id": who(ws, "mehmet.yilmaz"),
                                     "purpose": "Schlägt für neue Störungen Priorität und zuständige Gruppe vor; ein Mensch bestätigt.",
                                     "risk_class": "LIMITED", "model_id": model["id"],
                                     "policy": {"reversible": "APPROVAL", "irreversible": "NEVER"},
                                     "budget": {"tokens_per_day": 200000, "actions_per_hour": 60, "concurrent_runs": 2}, "min_accuracy": 85})
    count("agents")
    tried("agent case", client.post, f"/agents/{triage['id']}/cases",
          {"title": "VPN fällt im Homeoffice aus", "notes": "Klassischer P2-Fall.", "expected": {"priority": "P2", "responding_group": "1st Level"}})
    # Approved, not live: going live needs an evaluation run against the model, which a demo
    # installation without the model cannot make (agents.evaluation_required).
    tried("agent approve", client.post, f"/agents/{triage['id']}:approve")
    client.post("/agents", {"name": "Knowledge article drafter", "kind": "KNOWLEDGE_DRAFT", "owner_id": who(ws, "sofia.rossi"),
                            "purpose": "Drafts a knowledge article from a resolved incident for a person to review and publish.",
                            "risk_class": "MINIMAL", "model_id": model["id"], "policy": {"reversible": "NOTIFY", "irreversible": "APPROVAL"}})
    count("agents")


# ---------------------------------------------------------------------------- the run

def main():
    say(f"Provisioning '{SLUG}' at {API} …")
    made, owner = provision()
    if not EMPTY:
        # An empty workspace keeps its tour: it is what a newcomer's first start looks like.
        finish_onboarding(owner, made["owner_account_id"])
    ws = {"tenant_id": made["id"], "owner": owner, "owner_id": made["owner_account_id"],
          "default_hub": made["default_hub_id"], "people": {"owner": made["owner_account_id"]}, "sessions": {},
          "groups": {}, "templates": [], "hubs": {}, "collections": {}, "entries": {}, "pages": {}, "cis": {}}
    if not EMPTY:
        for step in SECTIONS:
            step(owner, ws)
    say("")
    say(f"Workspace '{SLUG}' ({made['id']}): " + (", ".join(f"{n} {k}" for k, n in sorted(counts.items())) or "empty"))
    say(f"Sign in as owner@{SLUG}.{DOMAIN}" + ("" if EMPTY else f", or as any of {', '.join(p[1] + '@' + SLUG + '.' + DOMAIN for p in PEOPLE[:3])} …")
        + " with the demo password.")
    if failures:
        say("")
        say(f"{len(failures)} write(s) were refused; the demo is incomplete:")
        for failure in failures:
            say(f"  - {failure[:600]}")
        return 1
    return 0


SECTIONS = [people, templates, scrum, kanban, classic, service_desk, cmdb, wiki, whiteboards, time_tracking, goals, agents]

if __name__ == "__main__":
    sys.exit(main())
