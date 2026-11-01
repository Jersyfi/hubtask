<!-- SPDX-License-Identifier: Apache-2.0 -->
# RB-A31 — Notifications are failing to send

**Alert:** `HubtaskNotificationsFailing` · **Severity:** ticket · **Catalogue:** A-31

## The symptom

For a quarter of an hour more than one in twenty notification sends failed
(`hubtask_notification_failures_total` against the successful sends,
`hubtask_notification_send_duration_seconds_count`), at least five of them. A failure is retried by
the queue; a refusal for one recipient is final at once. People are not receiving reminders,
mentions or approvals they wait for.

## Immediate action

```promql
sum by (channel, reason) (increase(hubtask_notification_failures_total[15m]))
```

* **`dependency.unavailable` on the mail channel** → the outbound mail server cannot be reached or
  refuses the session: its address and credentials (`HUBTASK_SMTP_HOST`, `_PORT`, `_USER`,
  `_PASSWORD`, `_SECURITY`); `/meta/health` names the dependency.
* **`mail.recipient_rejected` across many recipients** → the mail server rejects the sender
  (`HUBTASK_SMTP_FROM`) or its domain's SPF/DKIM; its log says which.
* **Another channel** → that channel's own credentials and its provider's status.

## Afterwards

Sends that kept failing end in the dead letter (A-07); redeliver them from there once the cause is
gone.
