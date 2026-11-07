<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A29 — Most mailboxes on an installation mail app fail

**Alert:** `HubtaskMailAppFailing` · **Severity:** ticket · **Catalogue:** A-29

## The symptom

For a quarter of an hour, at least half of the enabled mailboxes that sign in through one of the
installation's own mail apps (`HUBTASK_MAIL_OAUTH_{GOOGLE,MICROSOFT}_CLIENT_ID`/`_SECRET`, the
label `app`) have failed for over an hour (`hubtask_mail_app_mailboxes_failing` of
`hubtask_mail_app_mailboxes`, sampled by the scheduler's leader across every workspace). Mail sent
to those mailboxes is not taken in.

A mailbox fails on its own for its own reasons - a person withdrew the consent, the mailbox was
removed - and its workspace's administrators are told of it. Many failing together on one app
point at the app: its client secret expired or was deleted at the provider, the app was disabled,
or the configured secret is wrong after a change. No workspace administrator can fix that. With
only one or two mailboxes on the app, one person's withdrawn consent also fires this alert.

The numbers come from `installation_mail_apps()` (migration 0351): counts per preset, never an
address or a workspace.

## Immediate action

1. Look at the app at the provider (Google Cloud console, Microsoft Entra app registrations):
   is its client secret still valid, is the app enabled?
2. A secret that expired or was withdrawn → create a new one, put it into
   `HUBTASK_MAIL_OAUTH_<APP>_CLIENT_SECRET` and roll the server and worker pods.
3. Which failure the mailboxes record - codes only, no address (as the database owner):

   ```sql
   SELECT oauth_provider, failure, count(*)
   FROM mailbox
   WHERE enabled AND oauth_installation AND failing_since < now() - interval '1 hour'
   GROUP BY 1, 2;
   ```

   `mail.oauth_refused` - the token endpoint refused the request, which is what a client secret
   that is wrong, expired or deleted gives; `mail.oauth_revoked` - the provider no longer takes the
   refresh tokens; `mail.mailbox_unreachable` - the provider answered with a server error or could
   not be reached.
4. If the provider revoked the refresh tokens themselves (`mail.oauth_revoked`: a changed scope,
   a security event), each workspace's administrators have to connect their mailbox again under *Administration → Mail*.

## Afterwards

The gauge falls on the leader's next tick after the mailboxes' next successful read; a mailbox
reads again every minute.
