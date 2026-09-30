---
id: UC-ID-13
title: Require a new password from everyone after a breach
context: identity
actors: [PE-owner, PE-admin, PE-operator]
deployments: [D3, D4, D5, D6]
serves: [P-02, P-04, P-07]
state: built
tasks: [SI-07, SI-08, SI-16]
checked_by: [core/domain/model/identity/SignInPolicy_test.go, core/application/service/identity/SignInStep_test.go]
---

# Require a new password from everyone after a breach

## Goal

After a suspected breach, or to apply a stricter rule at once, an administrator makes every member
choose a new password at their next sign-in and ends every session opened before that moment.

## Story

At the bottom of *Sign-in*, a red action: *Require a new password from everyone*. The dialog says
what it does in one sentence and names the verb on its button. After a fresh proof, everybody is
signed out; at the next sign-in, the correct old password leads to "Choose a new password — your
workspace asked everybody to choose a new one on 30 September".

## How to check

1. The action asks for a confirmation that states the consequence, then for a fresh proof.
2. Every session opened before the action is refused on its next request.
3. The next sign-in with the old password leads to the new-password step, which shows the reason
   and the date; the new password meets the rules in force.
4. Accounts that sign in only through a provider are not affected.
5. The action is in the workspace's trail; the installation can set the same moment for every
   workspace, and that is in the instance journal.

## Where it ends

* No periodic expiry here: that is the *Expires after* rule, off by default.
* No mail to everybody announcing it — the administrator tells people.
