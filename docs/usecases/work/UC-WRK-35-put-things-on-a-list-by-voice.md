---
id: UC-WRK-35
title: Put things on a list by voice
context: work
actors: [PE-person, PE-member, PE-admin, PE-selfhoster, PE-operator]
deployments: [D1, D2, D3, D4, D5, D6, D7]
serves: [P-03, P-09, P-11]
state: verified
tasks: []
checked_by: [core/domain/model/voice/Phrase_test.go, core/domain/model/voice/Place_test.go, core/domain/model/voice/Settings_test.go, core/application/service/voice/Assistant_test.go, core/application/service/voice/Settings_test.go, presentation/voice/Alexa_test.go, infrastructure/alexa/Verifier_test.go, presentation/rest/OauthForm_test.go, test/integration/voice_test.go, apps/webapp/e2e/voice.test.mjs]
---

# Put things on a list by voice

## Goal

In the kitchen, a person says what is missing and it is on the right list - the shopping list, or
under "Metro" on it - without opening anything, and a workspace's administrator sets this up in a
few minutes, whether Hubtask runs at home or at an operator's.

## Story

Anna says "Alexa, sag Hubtask: zwei Liter Milch und Butter auf die Einkaufsliste". Alexa answers
"Notiert auf Einkauf: 2 l Milch und Butter", and both entries are in *Einkauf*. "Reis auf die
Metro-Liste" files rice under the category *Metro*. "Was steht auf der Einkaufsliste?" reads the
open entries; "Butter ist erledigt" ticks it off. The workspace's administrator switched voice on,
chose *Einkauf* as the default list, named the fridge as another name for it, tried a sentence on
the page, and connected Alexa by copying each value into Amazon's console.

At an operator's, the operator publishes the skill; the administrator only switches voice on.

## How to check

1. An administrator switches spoken entries on and chooses the default list in *Administration →
   Voice assistants*; off, Alexa says that voice entries are switched off.
2. Every collection and category is understood by its name, with or without its list word
   ("Einkaufsliste" for "Einkauf"); the administrator adds names people say for a list or a category.
3. *Try a sentence* says what a sentence would add where, and adds nothing.
4. A sentence adds every thing it names, with its amount, to the list or category named - or to the
   default list where none is named; several matching lists are asked back and an unknown one is
   said back.
5. "What is on …", "… is done" and "what is due today" read a list, tick off its entry and tell the
   person's day.
6. Connecting Alexa: the page gives the invocation name, the interaction model, the endpoint and
   the account linking's values to copy, and makes the linking's client from the vendor ID with
   Alexa's three addresses; with the operator's published skill it says to enable and link it.
7. Only Alexa's signed requests from a known skill are answered; entries are made as the person who
   linked the account, with their rights; an unlinked account is asked to link.

## Where it ends

* Alexa only; other assistants follow the same service when asked for.
* No dialogue beyond one question back; no reminders or dates by voice.
* The skill needs the installation reachable over HTTPS with a trusted certificate.
