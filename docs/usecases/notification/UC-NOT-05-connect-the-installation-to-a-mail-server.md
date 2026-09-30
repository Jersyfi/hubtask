---
id: UC-NOT-05
title: Connect the installation to a mail server
context: notification
actors: [PE-selfhoster, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-09, P-10, P-11]
state: built
tasks: [C-09, K-06]
checked_by: [infrastructure/mail/SMTP_test.go, infrastructure/mail/Health_test.go, test/integration/notification_delivery_test.go]
---

# Connect the installation to a mail server

## Goal

Whoever runs the installation points it at a mail server once, and from then on every message the
product sends goes out through it — and when there is no mail server, nothing breaks, nothing is
lost, and the installation says plainly why nobody receives mail.

## Story

The self-hoster sets the SMTP host, port, credentials and sender address in the environment and
restarts. Invitations and notifications start arriving. A household that has no mail server leaves
the settings empty; the product works, the health report says mail is not configured, and the
notifications that would have been sent are recorded.

## How to check

1. With the SMTP settings given, the product's mails go out through that server, from the
   configured sender address; no caller can choose another sender.
2. Without SMTP settings the installation starts and works; notifications are still recorded, and
   the health report names mail as the dependency that is down.
3. When the mail server is unreachable, deliveries are retried later rather than lost; the entry,
   comment or assignment that caused them is never held up or undone.
4. A subject or address containing a line break is refused (`mail.header_injection`) rather than
   sent.
5. Mails are plain text, one recipient each, with no attachments and no headers a caller can set.
6. No mail is sent to any address the installation did not derive from an account or an
   invitation.

## Where it ends

* No mail server in the image; the installation uses one the operator names
  ([NG-phone-home](../../vision/non-goals.md)).
* No per-workspace mail server or sender address.
* No mail templates editable by the operator; the words are the product's catalogue.
