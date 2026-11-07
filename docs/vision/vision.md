# The vision

**Hubtask is the work suite that is genuinely yours** — from a shopping list to a company's
processes, for one person, a household, a team, a company and a provider who runs it for thousands
of others — built with the seriousness of infrastructure: nothing is lost, nothing leaks between
workspaces, every guarantee can be checked.

## The promise, in six sentences

1. **One model holds a life, a client roster and a company's way of working.** "Buy milk", a
   release plan, an IT request and the wiki page that explains it live in the same five levels —
   Hub → Collection → Task → Work Package → Activity — with types, statuses and pages around them,
   without becoming five products.
2. **It is yours to run.** One image and PostgreSQL, `docker compose up`, the full feature set, no
   licence key that switches anything off, nothing that phones home.
3. **Everything you can click, a script and an agent can do too** — through the same API, under
   the same permissions, recorded in the same trail.
4. **It keeps your work safe and says so.** Backups that are restored in a drill, a trail that
   verifies, deletion that announces itself and can be stopped, a health report that names what is
   missing instead of failing silently.
5. **Every person sees what is theirs to use, and nothing else.** A control that may not be used is
   not shown; a value set by someone else says who set it.
6. **It grows with you, and never forces the growth on you.** Workflows, sprints, pages, forms,
   time and goals are modules a workspace switches on where it needs them. A module that is off
   leaves no word on any screen; a module that is on starts from a default that works before
   anybody configures it ([P-17](./principles.md#p-17-depth-is-switched-on-and-starts-from-a-default-that-works)).

## Who it is for, in one line each

The full lists are [personas.md](./personas.md) and [deployments.md](./deployments.md).

* **A person** who wants their tasks on their own machine and their data exportable forever.
* **A family or a small group** sharing some lists and keeping others to themselves.
* **A company** that signs its people in with its own directory and must answer to its auditors.
* **A provider** who runs Hubtask for customers — consumers or companies — and wants each one to
  feel it is their own.
* **A builder** who automates Hubtask or lets an agent work in it, and needs the API to be real.

## What makes it different

Nobody else sells operational seriousness at personal scale. The guarantees Hubtask ships are
things people meet in infrastructure software and never in a to-do list
([market analysis](../archive/marketing/market-analysis.md) §2). That is a promise about how the product is
*built*, which is why it has to be checked on every change — and why each use case in
[`usecases/`](../usecases/README.md) says how to check it.

It is the only suite of this breadth that a household can run on a small box and an auditor can
verify — every module inside the same tenant boundary, the same trail, the same backup, the same
API and agent surface. How the suite is built, and in which order, is the running plan in
[`backlog/suite/`](../backlog/suite/README.md).

## How to read the rest

* A feature starts from a **use case**: who wants what, and how we will know it works.
* A use case is measured against the **principles**. When two principles pull in different
  directions, the lower number wins.
* A use case must work in every **deployment** it names. "Works for the platform, confuses the
  private person" is a failed use case, not a trade-off.
