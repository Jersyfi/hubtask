---
id: UC-SVC-12
title: Answer a requester by mail and get their reply on the request
context: servicedesk
actors: [PE-handler, PE-requester]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/RequesterMail_test.go, core/application/service/servicedesk/RequesterReplies_test.go, core/application/service/work/AddComment_test.go, core/application/service/jumble/MailDelivery_test.go, core/domain/event/Comment_test.go, test/integration/requester_test.go]
---

# Answer a requester by mail and get their reply on the request

## Goal

A person without an account who asked by mail reads the team's answer in their own mailbox and
answers it there, so the conversation stays on the request - and never reads what the team wrote
for itself.

## Story

Frau Kaya writes to the workspace's intake with *#it* in the subject; the intake admits her and
files *Drucker druckt leer* for her. Jonas answers on the entry: "Toner replaced, please try
again." Frau Kaya receives a mail *[IT-42] Drucker druckt leer* with his words, answers "Works,
thank you", and her answer is a comment on IT-42. Jonas's internal note to Lena never leaves the
team.

## How to check

1. A public reply on an entry the mail intake filed for a requester without an account queues one
   mail per reply, however often its event is delivered; an internal note, a reply under one, a
   system comment and the requester's own words the intake filed queue none.
2. The mail goes to each address the entry was filed for, one mail per address, in the workspace's
   language, with the reply's words, its author's name and the ticket key in the subject
   (`[IT-42] Drucker druckt leer`); the Reply-To is the entry's address on the receiving domain
   where the intake has one.
3. A reply deleted before its mail is sent, an entry gone to the trash and a requester erased
   meanwhile are mailed nothing; the queued job names the comment alone.
4. The requester's answer by mail becomes a comment on the entry by its ticket key (UC-JUM-12
   check 2) under the intake's sender rules (check 4), marked as theirs, so it is not mailed back.
5. Who an entry was filed for is read in its own workspace only.

## Where it ends

* Requesters with an account follow their requests in the portal (UC-SVC-05); a mail to them about
  a reply is not built.
* The satisfaction question is asked in the portal (UC-SVC-07); asking a requester without an
  account by mail needs a link that answers without signing in, which is not built.
* Attachments of a reply are not mailed; the mail carries the words.
