# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The demo workspace's content, written through the product's own REST operations - never SQL - so
# that every row arrives the way a person's would: authorised, audited, validated. Called by
# scripts/seed-demo.sh, which says how to run it; the standard library only, like the other
# scripts here.
#
# The showcase itself - hubs, entries, pages, boards, time, goals - is `hubctl workspace demo`,
# written once in cmd/hubctl/DemoContent.go and run here as the owner with --voices: what the
# other people say there (their comments, check-ins and time, a requester's request) comes back in
# order and is written here, each as the person who says it. What only this script does stays
# here: the workspace and its people, the requesters, the CAB's approval and the agents.
#
# A step that is refused is reported and the next one runs: a demo with one product missing is
# still worth walking, and the closing summary names what is missing and why, with the server's
# own problem code. The run exits non-zero when anything was refused, so nobody mistakes a
# partial demo for a whole one.

import datetime as dt
import json
import os
import re
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

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



def who(ws, local):
    return ws["people"].get(local)


# ---------------------------------------------------------------------------- the showcase

def hubctl(*args, env=None):
    """hubctl as the owner, the binary seed-demo.sh built; its JSON answer and its exit code."""
    done = subprocess.run([os.environ["SEED_HUBCTL"], *args], capture_output=True, text=True,
                          env={**os.environ, **(env or {})}, timeout=45 * 60, check=False)
    return done.stdout, done.stderr, done.returncode


@section("the showcase: hubctl workspace demo --voices")
def showcase(client, ws):
    profile = os.path.join(ws["scratch"], "profile.json")
    env = {"HUBTASK_PROFILE": profile, "HUBTASK_PASSWORD": PASSWORD}
    _, err, code = hubctl("login", "--url", API[: -len("/api/v1")], "--email", f"owner@{SLUG}.{DOMAIN}",
                          "--tenant", ws["tenant_id"], env=env)
    if code != 0:
        raise RuntimeError(f"hubctl login: {err.strip()[:400]}")
    out, err, code = hubctl("--json", "workspace", "demo", "--workspace", SLUG, "--voices", env={"HUBTASK_PROFILE": profile})
    try:
        answer = json.loads(out)
    except ValueError:
        raise RuntimeError(f"hubctl workspace demo answered no JSON (exit {code}): {err.strip()[:400]}") from None
    for kind, n in (answer.get("made") or {}).items():
        count(kind, n)
    for refusal in answer.get("refused") or []:
        failures.append(f"showcase: {refusal}")
        say(f"    refused: {refusal[:300]}")
    ws["voices"] = answer.get("voices") or []
    say(f"    {answer.get('marked', 0)} objects marked, {len(ws['voices'])} voices to write")


def marks(client):
    """The showcase's marks by key: what hubctl made, under the keys cmd/hubctl/DemoContent.go gives."""
    return {m["key"]: m["object_id"] for m in client.get("/tenant/samples").get("samples", [])}


@section("requesters of the portal")
def requesters(client, ws):
    """Two organisations' people, each allowed to raise requests at the desk's intake."""
    desk = marks(client).get("collection.desk.intake")
    if not desk:
        raise RuntimeError("the showcase made no service desk intake")
    tried("requester role", client.post, "/roles", {"key": "customer", "name": "Kunde (Portal)", "permissions": ["REQUEST"]})
    for name, email, _org in REQUESTERS:
        made = tried(f"requester {name}", invite_and_redeem, client, ws, name, email)
        if not made:
            continue
        account_id, session = made
        tried(f"requester grant {name}", client.post, "/role-grants",
              {"role": "customer", "scope_type": "COLLECTION", "scope_id": desk, "account_id": account_id})
        ws["speakers"][name] = session
        count("requesters")


VOICE = re.compile(r"\{voice:(\d+)\}")


def resolved(value, made):
    """The value with each {voice:N} replaced by what voice N made; None where that voice made nothing."""
    if isinstance(value, str):
        missing = [int(n) for n in VOICE.findall(value) if int(n) not in made]
        if missing:
            return None
        return VOICE.sub(lambda m: made[int(m.group(1))], value)
    if isinstance(value, dict):
        out = {}
        for k, v in value.items():
            r = resolved(v, made)
            if r is None and v is not None:
                return None
            out[k] = r
        return out
    if isinstance(value, list):
        out = [resolved(v, made) for v in value]
        return None if any(r is None and v is not None for r, v in zip(out, value)) else out
    return value


@section("what the other people say")
def voices(client, ws):
    """Each voice written by its speaker, in hubctl's order; the owner's waiting writes between them."""
    made = {}
    for i, voice in enumerate(ws.get("voices") or []):
        speaker = voice.get("speaker") or ""
        session = ws["speakers"].get(speaker, client) if speaker else client
        if speaker and speaker not in ws["speakers"]:
            failures.append(f"voice {i} ({voice['what']}): {speaker} could not be signed in; left out")
            continue
        path, body = resolved(voice["path"], made), resolved(voice.get("body"), made)
        if path is None or (voice.get("body") is not None and body is None):
            failures.append(f"voice {i} ({voice['what']}): it waits for a voice that was refused")
            continue
        if voice["method"] == "MARK":
            # A versioned object is marked at the version the voices left it at.
            now = tried(f"read {voice['what']}", client.get, path)
            if now:
                tried(voice["what"], client.post, "/tenant/samples",
                      {**voice["mark"], "object_id": now["id"], "version": now.get("version")})
            continue
        answer = tried(f"{voice['what']} ({speaker or 'owner'})", session.call, voice["method"], path,
                       body if body is not None else ({} if voice["method"] != "GET" else None))
        if answer is None:
            continue
        object_id = answer.get("id") or answer.get("item_id")
        if object_id:
            made[i] = object_id
        if voice.get("mark") and object_id:
            mark = {**voice["mark"], "object_id": object_id}
            if isinstance(answer.get("version"), int):
                mark["version"] = answer["version"]
            tried(f"mark {voice['mark']['key']}", client.post, "/tenant/samples", mark)
        count("voices")


@section("the change advisory board approves")
def cab(client, ws):
    """The CAB decides as itself: the change's approval waits for one of its members."""
    approver = ws["speakers"].get("Priya Natarajan")
    if not approver:
        raise RuntimeError("Priya Natarajan could not be signed in")
    waiting = approver.get("/approvals")
    for approval in waiting.get("data", [])[:1]:
        tried("approve", approver.post, f"/approvals/{approval['id']}:decide",
              {"verdict": "APPROVE", "reason": "Geringes Risiko, Rückfall dokumentiert. Freigegeben."})
        count("approvals")

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
    with tempfile.TemporaryDirectory(prefix="hubtask-seed-") as scratch:
        ws = {"tenant_id": made["id"], "owner": owner, "owner_id": made["owner_account_id"], "scratch": scratch,
              "people": {"owner": made["owner_account_id"]}, "sessions": {}, "speakers": {}, "groups": {}, "voices": []}
        if not EMPTY:
            people(owner, ws)
            for name, local, _role, _locale in PEOPLE:
                if local in ws["sessions"]:
                    ws["speakers"][name] = ws["sessions"][local]
            showcase(owner, ws)
            requesters(owner, ws)
            voices(owner, ws)
            cab(owner, ws)
            agents(owner, ws)
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


if __name__ == "__main__":
    sys.exit(main())
