---
id: UC-ID-23
title: See Hubtask in my own script
context: identity
actors: [PE-person, PE-member]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-09, P-13]
state: specified
tasks: []
checked_by: []
---

# See Hubtask in my own script

## Goal

A person who uses Hubtask in Arabic, Hindi, Chinese or Bengali sees the interface set in a typeface made for that script, delivered by the installation itself.

## Story

A person switches their language to Bengali; headings and text keep their weights and sizes, set in a face designed for the script, and nothing is loaded from another server.

## How to check

1. For every language the installation lists as supported, text and headings are rendered with a face the installation ships that covers the script: IBM Plex for Latin, Cyrillic, Greek, Arabic, Hebrew, Devanagari, Thai and Simplified Chinese, Noto Sans Bengali for Bengali.
2. Headings in a script the condensed cut does not cover use IBM Plex Sans in its heading weight.
3. Every face is served by the installation; loading the app contacts no other domain (NG-phone-home).
4. A browser loads only the parts of a face its text needs.

## Where it ends

* Which languages are translated is i18n-l10n.md's; this use case is about the letters.

## Today

* Check 1: not met — not built yet, tracked in milestone UX.
* Check 2: not met — not built yet, tracked in milestone UX.
* Check 3: not met — not built yet, tracked in milestone UX.
* Check 4: not met — not built yet, tracked in milestone UX.
