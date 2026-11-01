---
id: UC-DOC-13
title: Answer a requester from the knowledge base
context: knowledge
actors: [PE-member, PE-requester]
deployments: [D2, D3, D4, D5, D6, D7]
serves: [P-05, P-18]
state: verified
tasks: []
checked_by: [core/domain/model/knowledge/Access_test.go, core/domain/model/knowledge/LanguageVariant_test.go, core/application/service/knowledge/Articles_test.go, core/application/service/knowledge/LanguageVariants_test.go, test/integration/page_repository_test.go]
---

# Answer a requester from the knowledge base

## Goal

A question already answered on a page is answered there for the person asking it in a service's portal, before it becomes a request - and the team sees whether the page helped.

## Story

The IT hub's page "Reset your password" explains the steps. Anna, who manages the hub's members, publishes it to the portal of the *IT help* service. Tom, an employee who reads no IT page, starts a request in the portal and types "password"; the page is offered, he opens it, follows the steps and says it helped - no request is raised. Anna sees beside the page how often it was opened and how many it helped.

## How to check

1. Somebody who manages a hub's members publishes one of its pages to a service's portal - a collection - and takes it back; never a page behind a restriction; the trail records each.
2. A requester of that service - REQUEST at the collection, reading no hub - is offered the service's articles that the words typed so far find, best first, or the most read without words, and opens one: the page alone, as it stands now; a page trashed, archived or restricted since, or taken back, is not found.
3. The requester says whether an article helped, one answer per person that a second replaces; beside the page its publisher sees, per service, how often it was opened and how many it helped and did not - never who said what.
4. Somebody who manages a hub's members links one of its pages to another page of the same space as its variant in one language - a BCP 47 tag, one page per tag, at most 20; a variant is no source and a source no variant; a tag that is no language is refused as `pages.language_malformed`, one taken as `pages.variant_language_taken` - and takes the link back; the trail records each. A requester offered or opening the source's article reads it in their language: the variant under the exact tag first, then the one under its language alone (`de` for `de-AT`), then the source; a variant trashed, archived or restricted since gives way to the next, and the opening and the rating stay the article's.

## Where it ends

* The portal's own screens - where a requester types and reads - are the service desk's portal (M5); this use case answers them.
* A variant is written by a person: nothing translates a page, and a page without a variant in the reader's language is read as it is written.
* An article is not turned into a request's answer automatically; an agent links or quotes it as any page.
