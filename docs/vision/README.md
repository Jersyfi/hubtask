# Vision — why Hubtask exists and for whom

This folder is the **why**. Everything else in `docs/` answers *what* Hubtask does
([`usecases/`](../usecases/README.md)), *how it was decided* ([`adr/`](../adr/README.md)) and *how it
is built* ([`architecture/`](../architecture/arc42.md)). Those change all the time. This one changes
rarely, and only by the owner's decision.

| File | Answers | IDs other documents cite |
|---|---|---|
| [vision.md](./vision.md) | What Hubtask promises, in one page | — |
| [principles.md](./principles.md) | The product principles every feature is measured against | `P-01` … |
| [personas.md](./personas.md) | The people who use, run and build on Hubtask | `PE-…` |
| [deployments.md](./deployments.md) | The seven shapes in which Hubtask is run, from one person to a platform | `D1` … `D7` |
| [non-goals.md](./non-goals.md) | What Hubtask deliberately is not, so nobody builds it by accident | `NG-…` |

## How the pieces hold each other

```text
vision/       WHY          principles · personas · deployments · non-goals     the owner decides
usecases/     WHAT         one file per use case, filed by bounded context      the owner decides the goal;
                           goal · story · how to check · where it ends · state  a task moves the state
adr/          HOW DECIDED                                                        unchanged
architecture/ HOW BUILT                                                          unchanged
backlog/      WHEN         every task names the use cases it serves              cut per milestone
```

A use case cites the principles it serves, the personas who act in it and the deployments it must
work in. A backlog task cites the use cases it builds. A pull request says which of their checks it
meets. `make gate-docs` holds the IDs together: a use case citing a principle that does not exist,
or a task citing a use case that does not exist, fails the gate.

**Why this exists.** Coding sessions read the task and the architecture, and both describe
mechanisms. Neither says who a feature is for, so a session filling a gap fills it with its own
reading — and a screen ends up correct by every gate and wrong for the person using it. The use
cases are the part that says what *must* be true for that person, in words that cannot be bent by
interpretation, and where the feature stops.

## Changing this folder

Only by the owner, or by a session he asked to. A principle that turns out to be wrong is
**replaced**, not edited quietly: the old text moves under *Replaced* with the date and the reason,
because use cases written against it have to be re-read.
