#!/usr/bin/env bash
# The daily alert check for the integration environment (observability-reliability.md §13.1).
#
# Read-only. It answers four questions, in the order they matter:
#
#   1. Is the alerting path alive? A watchdog mail older than a day means every silence below it
#      is meaningless - that failure hides all the others, so it is asked first.
#   2. Is anything firing, and which runbook says what to do about it?
#   3. Is anything pending - about to fire, and cheaper to deal with now?
#   4. Is every process still being scraped? A target that stopped answering reports nothing, and
#      an alert that reads its metrics cannot fire while it is gone.
#
# It changes nothing. What to do about what it finds is the runbook it names for each alert; this
# script's job is to make sure somebody is asked the question every day.
#
# The host address and the key path are deliberately not in this repository - they belong to the
# machine (deploy/integration/README.md). Point the variable at the host:
#
#   export HUBTASK_INTEGRATION_SSH="ssh -i ~/.ssh/<key> root@<host>"
#   deploy/integration/alert-check.sh
#
# Exit code: 0 when nothing is firing and the path is alive, 1 when something needs a person.
set -euo pipefail

: "${HUBTASK_INTEGRATION_SSH:?set it to the ssh command that reaches the host, e.g. \"ssh -i ~/.ssh/<key> root@<host>\" - the address and the key path are deliberately not in this repository}"

readonly REMOTE="$HUBTASK_INTEGRATION_SSH -o BatchMode=yes -o ConnectTimeout=15"
readonly RUNBOOKS="$(cd "$(dirname "${BASH_SOURCE[0]}")/../observability/runbooks" && pwd)"

# One helper for all three APIs: exec into the pod that serves it and fetch one path. wget rather
# than curl because that is what these images carry, and a port-forward would need a background
# process and a cleanup trap for no gain.
fetch() {
  local app="$1" port="$2" path="$3"
  $REMOTE "kubectl -n monitoring exec deploy/${app} -- wget -qO- localhost:${port}${path}" 2>/dev/null
}

status=0

echo "=== 1. The alerting path"
watchdog="$(fetch mailpit 8025 '/api/v1/search?query=subject%3AHubtaskAlertingPathAlive&limit=1')"
python3 - "$watchdog" <<'PY' || status=1
import sys, json, datetime
try:
    messages = json.loads(sys.argv[1] or "{}").get("messages", [])
except json.JSONDecodeError:
    print("  UNREADABLE - the catcher did not answer. Is the monitoring namespace up?")
    raise SystemExit(1)
if not messages:
    print("  BROKEN - no watchdog mail has ever arrived. Nothing below this line means anything:")
    print("           a silent alert and a broken alerting path look identical from here.")
    raise SystemExit(1)
last = datetime.datetime.fromisoformat(messages[0]["Created"].replace("Z", "+00:00"))
age = datetime.datetime.now(datetime.timezone.utc) - last
hours = age.total_seconds() / 3600
# It is delivered every 12 hours. A day of silence is past any repeat interval and any restart.
if hours > 24:
    print(f"  BROKEN - the last watchdog mail is {hours:.0f} h old (repeat interval is 12 h).")
    print("           Prometheus, Alertmanager or the catcher has stopped. Fix this before")
    print("           reading anything else: every silence below is unexplained until it is back.")
    raise SystemExit(1)
print(f"  alive - last watchdog mail {hours:.1f} h ago")
PY

echo
echo "=== 2. Firing, and 3. pending"
rules="$(fetch prometheus 9090 '/api/v1/rules?type=alert')"
python3 - "$rules" "$RUNBOOKS" <<'PY' || status=1
import sys, json, os
try:
    groups = json.loads(sys.argv[1] or "{}")["data"]["groups"]
except (json.JSONDecodeError, KeyError):
    print("  UNREADABLE - Prometheus did not answer with rules.")
    raise SystemExit(1)

firing, pending = [], []
for group in groups:
    for rule in group["rules"]:
        for alert in rule.get("alerts", []):
            # The watchdog fires permanently and on purpose; question 1 is where it is answered.
            # Listing it here as an incident would make this check report "needs a person" every
            # day of its life, which is how a daily check becomes a thing nobody reads.
            if alert["labels"].get("severity") == "watchdog":
                continue
            (firing if alert["state"] == "firing" else pending).append((rule, alert))

# A-12 is true here by decision rather than by accident: this environment keeps no backups, and
# the rule is written as an absence so that "nobody configured one" is loud everywhere else. It is
# routed to a receiver that sends nothing, and it is printed with that said rather than hidden -
# a check that silently drops an alert is a check that lies.
EXPECTED = {"A-12": "expected here: no backups by decision, routed to the muted receiver"}

def show(rule, alert):
    labels = alert["labels"]
    ident = labels.get("alert_id", "-")
    where = " ".join(f"{k}={v}" for k, v in sorted(labels.items())
                     if k not in ("alertname", "alert_id", "severity", "environment"))
    print(f"  [{labels.get('severity','?')}] {ident} {rule['name']} {where}")
    print(f"      since {alert['activeAt']}")
    print(f"      {rule['annotations'].get('summary','')}")
    if ident in EXPECTED:
        print(f"      ({EXPECTED[ident]})")
    book = rule["annotations"].get("runbook")
    if book:
        path = os.path.join(sys.argv[2], book)
        print(f"      runbook: {path}"
              + ("" if os.path.exists(path) else "   <-- MISSING, which the gate should have caught"))

if not firing:
    print("  firing: nothing")
else:
    print(f"  firing: {len(firing)}")
    for rule, alert in firing:
        show(rule, alert)

print()
if not pending:
    print("  pending: nothing")
else:
    print(f"  pending: {len(pending)} - not firing yet, and cheaper to deal with now")
    for rule, alert in pending:
        show(rule, alert)

raise SystemExit(1 if [r for r, a in firing if a["labels"].get("alert_id") not in EXPECTED] else 0)
PY

echo
echo "=== 4. Targets"
targets="$(fetch prometheus 9090 '/api/v1/targets?state=active')"
python3 - "$targets" <<'PY' || status=1
import sys, json
try:
    active = json.loads(sys.argv[1] or "{}")["data"]["activeTargets"]
except (json.JSONDecodeError, KeyError):
    print("  UNREADABLE - Prometheus did not answer with targets.")
    raise SystemExit(1)
hubtask = [t for t in active if t["labels"].get("job") == "hubtask"]
down = [t for t in active if t["health"] != "up"]
print(f"  hubtask pods scraped: {len(hubtask)}")
for t in sorted(hubtask, key=lambda t: t["labels"].get("role", "")):
    print(f"    {t['labels'].get('role','?'):<10} {t['labels'].get('instance','?')}  {t['health']}")
if not hubtask:
    print("    none - either the application is not deployed, or discovery is broken.")
    raise SystemExit(1)
for t in down:
    print(f"  DOWN: {t['labels']} {t.get('lastError','')}")
raise SystemExit(1 if down else 0)
PY

echo
if [[ $status -eq 0 ]]; then
  echo "=== Nothing needs a person today."
else
  echo "=== Something above needs a person. Each alert names its runbook; follow it, and if the"
  echo "    fix is a change to this repository, it goes through a branch and a pull request."
fi
exit $status
