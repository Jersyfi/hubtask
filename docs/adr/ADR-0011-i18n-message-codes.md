# ADR-0011 — i18n through message codes rather than server text

**Status:** accepted · **Date:** 2026-08-14

## Context
The application "must be capable of supporting every language". The frontend scope is open, and
several clients are expected (web, mobile, CLI, agents). History entries, error messages, and
notifications are human-readable and therefore relevant to translation.

## Decision
The API delivers **no finished display text**, but stable codes plus structured parameters
(`{"code": "activity.item_completed", "params": {...}}`). Localisation happens in the client.
Where the server has to render (email, push, ICS, export), it renders through the `i18n` port in the
locale of the **recipient**, using **ICU MessageFormat** and CLDR plural rules
(`golang.org/x/text`). Locale resolution: request header → account → tenant → installation.
Time zones exclusively as IANA names, with tzdata embedded in the binary. The supported locales are
data (from `locales/*.json`) and are published through `/meta/capabilities`.

## Options
1. **Codes plus parameters, rendering in the client (chosen).**
2. The server renders all text in the language of the request — every new language then requires a backend release; clients cannot deviate; caching becomes language-dependent.
3. English only in the backend, with clients translating freely — inconsistent wording, and email localisation becomes impossible.
4. A translation service at runtime — cost, latency, and a quality risk.

## Consequences
**Positive:** a new language is a new resource file, with no code change (QS-08); clients stay free
in wording and tone; caching is language-independent; correct plural forms for Arabic, Polish, and
Russian; agents receive stable codes instead of prose.
**Negative:** clients must maintain a catalogue; codes and parameters are part of the contract
(changing one is a breaking change); two catalogues (the client's, and the server's for emails) must
stay consistent.
**Countermeasures:** a shared source file `locales/en.json` for the server *and* for the shipped
client catalogues; CI checks placeholder consistency and unknown keys; the codes are documented in
the capability manifest.
