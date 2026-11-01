---
id: UC-INT-20
title: See my entries on another product's dashboard and in its search
context: integration
actors: [PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-08, P-09, P-12]
state: built
tasks: []
checked_by: [core/application/service/glance/Glance_test.go, test/integration/private_hub_readers_test.go]
---

# See my entries on another product's dashboard and in its search

## Goal

A person who starts their day in their organisation's collaboration suite - a Nextcloud or an
openDesk - sees their open Hubtask entries on its dashboard and finds entries and pages from its
search box, without the suite learning more than the person may see in Hubtask.

## Story

Mehmet's organisation runs openDesk. Its Nextcloud shows a widget *Hubtask* on his dashboard with
the seven entries assigned to him that are due first, each with its ticket key and a link. He types
"Rahmenvertrag" into the Nextcloud search and finds the entry *Rahmenvertrag verlängern* and the
page *Rahmenverträge* beside his files. The Nextcloud app that draws both is a separate package; it
calls Hubtask with a token Mehmet granted it.

## How to check

1. `GET /glance/open-entries` answers the open entries assigned to the caller anywhere in the
   workspace, due first, at most 25 a page (7 when no size is given), each with its kind, title,
   ticket key, due moment, last change and an absolute link into the web app. It is read through
   the same search as the app's: an entry in a private hub the caller is not on is not listed even
   when it is assigned to them, and another workspace's caller lists none of it.
2. `POST /glance:search` answers the entries and pages whose words match, best first, each with
   its title, the entry's ticket key or the page's passage that matched, and an absolute link; the
   entries a page at a time, the pages at most `size` and on the first page only. A search without
   words is refused as `search.words_required`; what the caller may not see is left out, never
   refused.
3. Both need `items:read` and nothing else; a refusal of the search below them is answered as it is.

## Where it ends

* The Nextcloud app itself - its dashboard widget and unified search provider, a PHP package - is
  built in its own repository after the suite (accepted.md, "The owner's fourth answers", item 8);
  this repository serves the two reads and holds no PHP.
* The token is the caller's own (UC-INT-01); a Nextcloud-wide sign-in that maps a Nextcloud
  account to a Hubtask person is the installation's single sign-on.
* A search reads at most four pages of the search below to fill one page, so a page may be shorter
  than asked where the caller may see little; its cursor walks on.
