# A11Y — The keyboard walk of the entry

**2026-09-21, local development machine, `claude/f9-08-entry` on `main` at `f662ac2e`
(F9-06 and F9-07 merged).** `design-system.md` §10's walk repeated for `/items/{id}` after F9-08
rebuilt the route as head, subtree, details and tabs (ADR-0061 decision 4) — the rows 2.1.1,
2.1.2, 2.4.1, 2.4.3, 2.4.7, 2.4.11 and 2.5.7, at a desk's width and at a phone's, because the
shell draws the two differently since F9-06.

| | Value |
|---|---|
| The application | `apps/webapp` built from this branch, served as the binary serves it (`e2e/serve.mjs`), against the stubbed workspace of `e2e/fixture.mjs`: one task with two work packages, one of them holding two activities, one label, no assignee, no dates — the least an entry can be and still show every row |
| Method | Real `Tab` presses round the whole ring in Chromium (Playwright), at 1280 px and at 375 px. At every stop: the role and the accessible name, whether `:focus-visible` matched, whether the element or its shell drew an outline, and whether the stop lay under a `sticky` or `fixed` region it was not part of (`elementFromPoint` at its centre). Then every overlay of the route entered by `Enter` on its control and left by `Escape`, and the focus read afterwards: the nine details rows, the entry's menu, and a subtree row's menu — the keyboard alternative to dragging (2.5.7). On the phone the details fold was opened by `Enter` on its summary first. The script is `e2e/_walk.mjs`'s method; the numbers below are its output |
| What "frame" means below | The stops every signed-in route begins with since F9-06: **skip link** · the rail toggle (or ☰ on a phone) · wordmark · the account menu (on a desk) · Dismiss (the preview banner) · the navigation tree (one stop, arrows within; on a phone it is behind ☰) · Create hub. Seven on a desk, three on a phone — the phone's primary destinations are the four stops of the bottom bar at the end of the ring |

## The ring

Every stop matched `:focus-visible` and drew rule 5's ring; no stop lay under a sticky or fixed
region; exactly one `<main>` and one `<h1>`, the heading read and not drawn (the title the reader
sees is the field they edit it in). `→` a link, `⏺` a button, `☐` a checkbox, `¶` a textarea,
`▹` a disclosure, `⧉` a tab, `▭` a tab panel (focusable by the `Tabs` component's design), `⌗`
the tree (one stop). Names are as the script read them, cut at forty characters.


**1280 px — 50 stops.** → Skip to the content · ⏺ Collapse the navigation · → Hubtask · ⏺ Jérôme Winkel · ⏺ Dismiss · ⌗ Workspace · ⏺ Create hub · → House · → Kitchen · ⏺ Actions for Order the tiles for the spla · ☐ Mark Order the tiles for the splashback and the floor as done · ¶ Title · ¶ Notes · ▹ Details · ⏺ Assignee Add · ⏺ Date Add · ⏺ Starts Add · ⏺ Labels Materials · ⏺ Reminders Add · ⏺ Repeats Add · ⏺ Written in Add · ⏺ Cover Add · ⏺ Attachments Add · ⏺ Close every level · ☐ Select Tiles for the floor · ⏺ Hide what is inside Tiles for the floor · ☐ Mark Tiles for the floor as done · → Tiles for the floor · ⏺ Add Activity inside Tiles for the floor · ⏺ Move Tiles for the floor · ☐ Select Measure the floor · ☐ Mark Measure the floor as not done · → Measure the floor · ⏺ Move Measure the floor · ☐ Select Order the floor tiles · ☐ Mark Order the floor tiles as done · → Order the floor tiles · ⏺ Move Order the floor tiles · ☐ Select Tiles for the splashback · ⏺ Hide what is inside Tiles for the splash · ☐ Mark Tiles for the splashback as not done · → Tiles for the splashback · ⏺ Add Activity inside Tiles for the splash · ⏺ Move Tiles for the splashback · ⏺ Add Work package · ⧉ Comments · 0 · ▭ Nothing has been said about this entry y · ¶ Say something · ⏺ Comment · → Accessibility statement, on hubtask.eu

**375 px — 40 stops.** → Skip to the content · ⏺ Open the navigation · ⏺ Dismiss · → Kitchen · ⏺ Actions for Order the tiles for the spla · ☐ Mark Order the tiles for the splashback and the floor as done · ¶ Title · ¶ Notes · ▹ Details · ⏺ Close every level · ☐ Select Tiles for the floor · ⏺ Hide what is inside Tiles for the floor · ☐ Mark Tiles for the floor as done · → Tiles for the floor · ⏺ Add Activity inside Tiles for the floor · ⏺ Move Tiles for the floor · ☐ Select Measure the floor · ☐ Mark Measure the floor as not done · → Measure the floor · ⏺ Move Measure the floor · ☐ Select Order the floor tiles · ☐ Mark Order the floor tiles as done · → Order the floor tiles · ⏺ Move Order the floor tiles · ☐ Select Tiles for the splashback · ⏺ Hide what is inside Tiles for the splash · ☐ Mark Tiles for the splashback as not done · → Tiles for the splashback · ⏺ Add Activity inside Tiles for the splash · ⏺ Move Tiles for the splashback · ⏺ Add Work package · ⧉ Comments · 0 · ▭ Nothing has been said about this entry y · ¶ Say something · ⏺ Comment · → Accessibility statement, on hubtask.eu · → Workspace · → Search · → Jumble · → You

The order is the document's and it is the order that makes sense (2.4.3): the trail, the entry's
menu, the checkbox, the title, the notes, the details (beside the text on a desk, folded under the
head on a phone — in the document before the subtree on both, so a reader meets the entry's fields
right after its text), the subtree level by level with each row's controls, "Add Work package",
the tabs, the comment field, the footer.

## Every overlay, entered and left

| Overlay | Opened by `Enter` | Focus moved in | `Escape` closed it and focus returned |
|---|---|---|---|

| Assignee → `AssigneePanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Date → `DuePanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Starts → `DuePanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Labels → `LabelsPanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Reminders → `ReminderPanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Repeats → `RecurrencePanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Written in → `LanguagePicker` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Cover → `CoverPanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| Attachments → `AttachmentPanel` (desk / phone) | yes / yes | yes / yes | yes / yes |
| The entry's menu (edit, share) (desk / phone) | yes / yes | yes / yes | yes / yes |
| A subtree row's menu (the drag's alternative, 2.5.7) (desk / phone) | yes / yes | yes / yes | yes / yes |

On a desk a row's editor is a `Popover` beside it; on a phone a `Drawer` from the bottom. The row
menu's eleven items are the list's (up, down, top, bottom, inside the one above, out one level,
another collection, archive, a due date, duplicate, trash), each with its reason where it cannot
be used — "this is already first in its list" — read with the item.

## Findings, and what was done with them

1. **Four stops under the bottom bar** (375 px, the third level's row) — the bar is fixed and a
   `Tab` that scrolled a control into view scrolled it under the bar (2.4.11). Fixed in this
   branch: `scroll-padding-block-end` on the root for the bar's height below `medium`, and
   `scroll-padding-block-start` for the app bar on every width. Re-walked: no stop obscured.
2. **The page scrolled sideways by ten pixels** on the phone — the comment field, a `Textarea`
   at `width: 100%` in the content box. Fixed in this branch (`border-box`); the entry walk
   asserts `scrollWidth` equals the viewport since.
3. **Not a finding, worth recording:** the first `Enter` on a details row after the fold was
   opened by a *pointer* click on its summary did nothing in the script, while the same `Enter`
   after opening the fold by keyboard worked every time, as did a pointer on the row itself. The
   walk opens the fold by keyboard, which is what a keyboard walk should do; a reader mixing
   pointer and keyboard is not affected in the browser (the row takes a click, and a second `Tab`
   reaches it).

**Still owed**, as at F5-13: the screen-reader pass with VoiceOver, NVDA and Orca. Nothing above
replaces it; what is new here for a reader — the title as a field named "Title" after a heading
that says the same words, the details rows named "Assignee, add" — is what that pass has to hear.
