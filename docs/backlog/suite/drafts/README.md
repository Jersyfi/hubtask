# Use case drafts of the suite

Written with the concept and accepted with it (the owner's word of 2026-10-28,
[`../accepted.md`](../accepted.md)). Each lands in `docs/usecases/<context>/` with the step that
builds it, in `state: specified` first, and is proven there by tests.

| Draft | Lands as | In step | What it needs at landing |
|---|---|---|---|
| [INS-21](./INS-21-switch-a-module-on.md) | `docs/usecases/admin/`, the next free `UC-INS` number | S-02 | the draft number `UC-INS-19` was taken by milestone CD (*Change a workspace's address*) |
| [FLW-01](./FLW-01-give-our-work-its-own-types.md) | `docs/usecases/workflow/`, the prefix `FLW` | S-03, S-06 | the context `workflow` (prefix `FLW`) in the gate's context table (`tools/checkdocs/usecases.go`) and in `docs/usecases/README.md`; *Where it ends* re-read against D-07 (records exist as the *record* behaviour of F-1) |
| [FLW-02](./FLW-02-describe-how-our-work-moves.md) | as FLW-01 | S-07 | as above |
| [FLW-03](./FLW-03-move-an-entry-on.md) | as FLW-01 | S-07 | as above |
| [FLW-04](./FLW-04-ask-for-what-a-step-needs.md) | as FLW-01 | S-07 | as above |
| [FLW-05](./FLW-05-change-a-workflow-in-use.md) | as FLW-01 | S-07 | as above |

The front matter's `id:` is written without `UC-` until the file moves, for the reason in
[`../README.md`](../README.md#what-is-in-this-folder). The *Today* lines name the step that carries
each check. Every draft is re-read against milestone UX's presets and *Flow*
(UC-WRK-23 … UC-WRK-31) before it lands: a workflow is the next step after *Flow*, and the module
switch reuses the presets' source-and-lock mechanism.
