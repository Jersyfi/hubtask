---
id: UC-IMP-06
title: Move a wiki space with its history and comments
context: importer
actors: [PE-admin, PE-person]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-12]
state: built
tasks: []
checked_by: [infrastructure/importer/WikiXml_test.go]
---

# Move a wiki space with its history and comments

## Goal

A team leaving a wiki takes a space with what was said and changed on it: each page arrives with
its earlier versions as its history and with its comments, beside what UC-IMP-05 carries (D-10).

## Story

The administrator exports the space as XML, chooses *Import…* on the hub, *Confluence* and the
ZIP. The pages appear under *Pages* in their tree; a page's *History* lists the versions the wiki
kept, oldest first, and its comments stand where they were written, replies under their thread.

## How to check

1. A space's XML export becomes the hub's pages in their tree with their content, their creation
   and their last change; a draft and a page in the trash are not carried.
2. Each page's earlier versions become its history in the order the wiki numbered them, each with
   its title, its content, the wiki's version message as its name and its moment; the current
   content is the last version.
3. A page's comments and replies land on it with their moment and their author named in the text,
   an inline comment with the words it was written about; a removed comment is not carried.
4. The newest version of each attachment becomes the page's file with its name, a picture drawn
   where it stood; an attachment's earlier version and a file of a page not exported are counted
   in the report, and the same export imported twice creates nothing the second time.

## Where it ends

* The wiki's people are not accounts here: a version and a comment are written by the importing
  person, and a comment names its author in its text.
* An inline comment keeps its words but is not pinned to a block; resolved and open threads both
  arrive open.
* An attachment's earlier versions, page permissions, labels and blog posts are not carried.
