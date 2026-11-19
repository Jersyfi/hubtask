---
id: UC-SVC-14
title: Ask any department without a link
context: servicedesk
actors: [PE-member, PE-requester]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-11, P-18]
state: verified
tasks: []
checked_by: [apps/webapp/src/lib/portal/portal.test.ts, apps/webapp/src/lib/palette/registry.test.ts, apps/webapp/e2e/portal-frame.test.mjs]
---

# Ask any department without a link

## Goal

A person who works in the workspace finds every department that takes their requests in one place,
asks any of them and follows everything they asked, without a link from the department and without
leaving their own work.

## Story

Jonas works in *Produktentwicklung* and needs a new laptop. He opens *Erstellen* and chooses
*Anfrage an eine Abteilung*: *Anfragen & Hilfe* lists *IT-Service-Desk*, *Personal* and *Einkauf*,
the departments whose requester role he holds. He types "laptop"; the search offers *Laptop oder
Zubehör bestellen* at the IT desk and an article on setting one up. He asks, and below *Meine
Anfragen · überall* shows his new request beside the certificate he asked *Personal* for last
week, each in its desk's own words. His sidebar stays where it was the whole time.

## How to check

1. A member whose role carries REQUEST at some desks reaches *Anfragen & Hilfe* through
   *Erstellen* (*request to a department*), through the search (the command palette names it) and
   through *all departments* on every portal's start; it is absent where no department takes their
   requests. The sidebar stays.
2. *Anfragen & Hilfe* lists every portal the server names for the account - the collections where a
   role of the workspace's own carrying REQUEST is held - each with its name and its description or
   kinds of request; one the account may not ask is not listed.
3. One search finds, by every word typed, the kinds of request of every listed department and the
   articles each publishes, each with its department's name; choosing a kind asks it in the request
   dialog of that department (UC-SVC-05 check 2).
4. *Meine Anfragen · überall* lists the reader's newest requests of every listed department
   together, newest first, each once, in the desk's own words with its department named
   (UC-SVC-05 check 12).

## Where it ends

* Raising, following and answering one request is UC-SVC-05; an outsider, who holds READ nowhere,
  gets the portal's frame instead of this page (UC-SVC-05 check 11).
* Which desks a person may ask is set by the roles an administrator gives (UC-SVC-05); this page
  grants nothing.
