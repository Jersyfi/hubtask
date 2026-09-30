---
id: UC-INT-05
title: Call an outside service from a rule
context: integration
actors: [PE-admin, PE-integrator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-01, P-09, P-11]
state: built
tasks: [G-09, F8-04]
checked_by: [core/application/service/automation/Outbound_test.go, core/domain/model/automation/Outbound_test.go, test/security/ssrf_test.go]
---

# Call an outside service from a rule

## Goal

A rule can tell another system that something happened — post to a chat channel, open a ticket,
ping a monitoring endpoint — with a body built from the event, a secret that never shows again,
and no way to reach into the installation's own network.

## Story

The administrator adds an *HTTP request* step to the rule that handles overdue invoices: `POST` to
the team chat's incoming-webhook address, with a body template naming the entry's title and a secret
header. They save; the secret is masked from then on. Each run queues the call; if the chat service
is down, the call is retried.

## How to check

1. A rule step can call an address with GET, POST, PUT, PATCH or DELETE; any other method is refused
   with `automation.http_method_unknown`, an address that is not http or https with
   `automation.http_url_invalid`.
2. The body template is checked when the rule is saved and rendered from the run's event at each
   attempt, so a retry sends what the first attempt would have.
3. A secret header value is shown as `***` in every answer after the save, and sending `***` back
   in an edit keeps the stored secret.
4. A call to a private, loopback, link-local or metadata address is refused with
   `automation.http_target_blocked` unless the operator released private networks.
5. The call happens on a job, not inside the run: a failing target is retried with the webhook
   ladder's eight attempts, and a refusal the target answered is recorded with its status.
6. The rule never reads the answer: the response is discarded unread and is available to no
   condition and no later step.
7. A rule step can also hand the run's event to a named webhook subscription, through the
   subscription's own signature, retries and dead letter.
8. Each call is in the trail as `automation.http_requested`, with no secret and no body.

## Where it ends

* No reading data back from another system into a rule
  ([ADR-0009](../../adr/ADR-0009-automation-rules-cel.md)).
* No per-rule allowlist of addresses; the installation's egress guard is the one boundary.
