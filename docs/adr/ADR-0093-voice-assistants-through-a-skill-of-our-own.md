# ADR-0093 — Voice assistants through a skill of our own

**Status:** accepted · **Date:** 2026-10-25 · **Decided:** 2026-10-25, by the owner ("Ich möchte
Amazon Alexa Echo Dot mit Hubtask verbinden können … setze es für mich um")

**Rule lives in:** [UC-WRK-35](../usecases/work/UC-WRK-35-put-things-on-a-list-by-voice.md),
[identity.md](../architecture/identity.md) §15.3 (the token endpoint's standard form)

## Context

People want to say "put butter on the shopping list" in the kitchen and find it in Hubtask, filed
where it belongs — on the list for the Metro store, say. Amazon closed its lists to other apps on
1 July 2024: the List Management API and list skills are gone, and Anylist and Todoist moved to
custom skills with an invocation name. A self-hosted installation and a large operator both have
to be served, and setting it up must be the workspace administrator's few minutes, not a project.

## Decision

1. **A custom skill, "Hubtask", spoken to by name:** "Alexa, sag Hubtask: Butter auf die
   Einkaufsliste". Without the name a sentence goes to Amazon's own list, and nothing can change
   that.
2. **Alexa transcribes, Hubtask understands.** The interaction model has a thing slot and a list
   slot for "{Thing} auf die {List}" and a free-text intent for every other phrasing; Hubtask
   splits the things (", und"), reads amounts ("zwei Liter" → "2 l") and resolves the place itself:
   an alias set by the administrator, a collection's own name, the name without its list word
   ("Einkaufsliste" → "Einkauf"), a category of a list ("Metro" in "Einkauf" files under the label),
   then a prefix; ambiguity is asked back, nothing known is said back, no place is the default
   list. A new list therefore works before the model is pasted again.
3. **Every act is the linked person's,** through the catalogue: CreateWorkItem, QueryItems,
   CompleteWorkItem — their rights, their audit trail. The skill links accounts through Hubtask's
   own OAuth server with PKCE, which Alexa speaks; the token endpoint accepts RFC 6749's
   form-encoded request and answers its standard document for clients that are not Hubtask's own.
4. **Every request is proven Alexa's:** the certificate from `https://s3.amazonaws.com/echo.api/…`,
   its chain naming `echo-api.amazon.com`, the body's SHA-256 signature, a timestamp within 150
   seconds, and a skill ID the installation publishes (`HUBTASK_ALEXA_SKILL_IDS`) or the workspace
   set. Anything else answers as if the route did not exist.
5. **Two set-ups, one page.** A self-hoster creates a development skill in the free Alexa console
   (it works on every Echo of their account without publishing) and copies each value from
   *Administration → Voice assistants*, where the linking's client is made from the vendor ID with
   Alexa's three redirect addresses. An operator publishes one skill and names it in the
   configuration; workspaces then only switch voice on.
6. **Off by default,** per workspace; an administrator switches it on, picks the default list and
   the names people say, and tries a sentence before anybody speaks one.

## Options

1. **A custom skill with Hubtask understanding the words (chosen).**
2. **A model per list** (one intent per list). Every new list needs a rebuilt model, and the
   model's limits arrive with a busy household.
3. **Amazon's own lists, synchronised.** Closed by Amazon in 2024.
4. **Alexa Routines calling a URL.** No account linking, no answer spoken back, one routine per
   sentence.

## Consequences

**Positive:** one sentence away from the list; filing into categories by voice; the rights and the
trail of the screen; works on a self-hosted installation without Amazon publishing anything.

**Negative:** the installation must be reachable over HTTPS with a trusted certificate — a home
installation needs a reverse proxy or a tunnel, which the page says. A development skill serves the
developer's own Amazon account; other households need the operator's published skill.

**Reconsider when** Alexa+ or another assistant offers a way in that needs no invocation name.
