# docs/usecases — the yardstick

One file per use case: the owner's statement of what must be true for a person, and the yardstick
every task is checked against. The format and the states are in [`README.md`](./README.md).

## What must not happen here

* **No change to what a person observes in *Goal*, *How to check* or *Where it ends* without the
  owner.** If the code cannot meet a check, or a check asks for the wrong thing, stop and report it
  in a `decision` issue and the pull request. A check rewritten to match what was built is the drift
  this folder exists to prevent. A wrong reference, check number or typo is a correction: fix it,
  and mark it `correction` in the pull request's *Use cases* section.
* **No use case deleted or renumbered.** One that no longer applies moves to `state: retired` with
  a line saying why. An ID is never reused.
* **No new use case unless the owner asked for it**, in the conversation or the task — and then
  only in `state: specified`, said so in the pull request body.
* **No check a reviewer has to interpret.** Each check is one observable fact: a person does X and
  sees Y; a request without Z is refused with code W; in deployment D1 the screen does not show V.
  "Works well", "is intuitive", "handles errors" are not checks. A check only a walk can confirm
  names the screen and the persona.
* **No *Today* that says more than what is missing.** It lists only the checks not met yet, one line
  per check with where it is tracked, and it is absent once the state is `built` or `verified`.

## What you may change on your own

`state:`, `tasks:` and `checked_by:` in the front matter, and *Today* — in the pull request that
builds or proves a check, naming its evidence. A correction as above.

## How to check a change

```bash
make gate-docs           # front matter, sections, the index, and every UC-… cited anywhere
```

Before building, read every use case the task names completely — *Where it ends* is what stops
you building more than was asked. Before a pull request leaves draft, work through the checklist
in [README.md](README.md#checking-work-against-its-use-cases).
