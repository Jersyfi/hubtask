---
id: UC-AUD-05
title: Check a workspace without seeing its content
context: audit
actors: [PE-auditor, PE-owner]
deployments: [D3, D4, D6]
serves: [P-05, P-07, P-11]
state: built
tasks: [G-12, F4-19]
checked_by: [test/integration/audit_read_test.go, apps/webapp/src/lib/data/capability.test.ts]
---

# Check a workspace without seeing its content

## Goal

A data protection officer or an internal auditor can read how the workspace is configured and what
happened in it, and can change nothing and read no one's work — so nobody has to be made an
administrator just to be audited.

## Story

The owner adds the company's data protection officer with the *auditor* role. The officer sees the
administration area: the trail, the backup targets and runs, the retention rules and what they
would do, the legal holds, the automation rules and their runs, the webhook subscriptions. Every
control that would change something is absent. The workspace's hubs, tasks and comments are not
there at all. If the officer is also an ordinary member of one hub, they read that hub's work as a
member and keep the auditor's view besides — the two roles add up.

## How to check

1. A person holding only the auditor role reads the whole trail, verifies it and exports it.
2. The same person reads the backup targets and runs, the retention rules and their preview, the
   legal holds, the automation rules and their runs, and the webhook subscriptions.
3. No secret appears in anything they read: no target credential, no webhook signing secret.
4. Every attempt to read a hub, a collection, a task or a comment is refused; the navigation shows
   none of them.
5. Every attempt to change configuration is refused, and the web app shows no control that would
   change it.
6. A person who is auditor and member at once keeps both: they read the work they are a member of
   and the whole trail.

## Where it ends

* The auditor does not read the register of data subject requests; they read the `dsr.*` entries
  in the trail. Whether a data protection officer should read the register itself is not decided
  here.
* The auditor role is not a rung on the role ladder and does not grant anything the other roles
  have.
* An operator of the installation is not an auditor of a workspace
  ([NG-operator-reads-content](../../vision/non-goals.md)).

See [audit.md](../../architecture/audit.md) §5.
