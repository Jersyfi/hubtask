# Use case drafts of the suite

Written with the concept and accepted with it (the owner's word of 2026-10-28,
[`../accepted.md`](../accepted.md)). Each lands in `docs/usecases/<context>/` with the step that
builds it, in `state: specified` first, and is proven there by tests.

| Draft | Lands as | In step | What it needs at landing |
|---|---|---|---|
| INS-21 | landed as [UC-INS-21](../../../usecases/admin/UC-INS-21-switch-a-module-on.md) | S-02 | — |
| FLW-01 | landed as [UC-FLW-01](../../../usecases/workflow/UC-FLW-01-give-our-work-its-own-types.md) | S-03, S-06 | — |
| FLW-02 | landed as [UC-FLW-02](../../../usecases/workflow/UC-FLW-02-describe-how-our-work-moves.md) | S-07 | — |
| FLW-03 | landed as [UC-FLW-03](../../../usecases/workflow/UC-FLW-03-move-an-entry-on.md) | S-07 | — |
| FLW-04 | landed as [UC-FLW-04](../../../usecases/workflow/UC-FLW-04-ask-for-what-a-step-needs.md) | S-07 | — |
| FLW-05 | landed as [UC-FLW-05](../../../usecases/workflow/UC-FLW-05-change-a-workflow-in-use.md) | S-07 | — |

The front matter's `id:` is written without `UC-` until the file moves, for the reason in
[`../README.md`](../README.md#what-is-in-this-folder). The *Today* lines name the step that carries
each check. Every draft is re-read against milestone UX's presets and *Flow*
(UC-WRK-23 … UC-WRK-31) before it lands: a workflow is the next step after *Flow*, and the module
switch reuses the presets' source-and-lock mechanism.
