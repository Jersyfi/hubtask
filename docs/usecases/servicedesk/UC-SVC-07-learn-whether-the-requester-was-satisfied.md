---
id: UC-SVC-07
title: Learn whether the requester was satisfied
context: servicedesk
actors: [PE-requester, PE-handler]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: built
tasks: []
checked_by: [core/domain/model/servicedesk/Satisfaction_test.go, core/application/service/servicedesk/Satisfaction_test.go, infrastructure/postgres/query/Satisfaction_test.go, test/integration/service_desk_rating_test.go]
---

# Learn whether the requester was satisfied

## Goal

Once a request is resolved, the person who raised it says in one answer how satisfied they were, and the team sees per queue how satisfied its requesters are and which of them were not.

## Story

Mehmet's VPN request is resolved. In the portal he gives it 2 of 5 and writes "Took three days." Lena's queue *Network* shows an average of 4.1 this month; her queue *Unhappy*, the resolved requests scored 2 or less, now lists Mehmet's request with his words in its discussion.

## How to check

1. The person who raised a request rates it once it is resolved: a score and, if they like, words the team reads as their comment. An open request is refused with `satisfaction.request_open`; a request somebody else raised is not found.
2. A score is a whole number from 1 to 5; any other is refused with `satisfaction.score_invalid`. Rating again replaces the score.
3. The requester reads their own score with the request in the portal (`satisfaction`, null while they gave none).
4. A queue's satisfaction is its view's aggregation `AVG` over `satisfaction`, read with the view's filter and the reader's permissions; a query or a view filters by `satisfaction` - `LTE 2` keeps the unhappy ones, `IS_NULL` the unrated.
5. A rating stays in its workspace: the workspace next door neither reads it nor rates the request, and it is in the workspace's archive.

## Where it ends

* The question is asked where the requester follows the request - the portal; asking it by mail is the mail channel's for requesters (S-18, with the owner's safeguards for requesters without an account).
* One question on one scale; a survey with several questions is not built.
