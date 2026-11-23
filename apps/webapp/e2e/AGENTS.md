# apps/webapp/e2e — the browser walks

Each `*.test.mjs` here is a suite of walks: the built web app (`dist/`), loaded in a browser and
used as a person uses it. A walk is what proves a use case check about a screen
([`docs/usecases/README.md`](../../../docs/usecases/README.md)). No walk talks to a server: every
request to `/api/v1` is answered by `fixture.mjs` or by a stub in the suite. A walk therefore
proves the screen against answers a worker wrote, and the rules below exist to keep those answers
and the walk honest.

The other files are shared by the suites: `fixture.mjs` (the signed-in workspace and its answers),
`browser.mjs` (one browser per suite), `serve.mjs` (serves `dist/` as the binary does),
`record.mjs` (writes down the answers a walk gives) and a few helpers. `bounds.json` lists what
the first two rules still allow. Walking the running product by hand or by a temporary script is
in [`docs/evidence/README.md`](../../../docs/evidence/README.md), "How to walk".

## What must not happen here

* **No suite that does not say what it proves.** The comment at the top of a suite names the use
  case checks its walks prove, in a form the documentation gate reads: `UC-ID-12 check 4`,
  `UC-ID-12/4` or `UC-WRK-32 checks 2, 3 and 6`. A suite that proves no use case check says what
  it holds instead, in a line of that comment that starts `// No use case check:`. The 55 suites
  that do neither are listed under `no_check_cited` in `bounds.json`; a suite that is not listed
  fails without one of the two.
  `[partial: ci:node; open: a citation below the top comment counts too, whether the walk proves the check it cites, a suite added to the list]`
* **No fixed wait.** A walk waits for what the person would see — an element, a text, a request
  that left — never for a number of milliseconds: a fixed wait is too short on a slow machine and
  wasted time on a fast one. It is written in two ways, and `bounds.json` lists how many of each
  a file still has: under `fixed_waits` Playwright's `waitForTimeout`, 355 in all, and under
  `timer_waits` a timer (`setTimeout`), 37 in all. A file may have fewer and never more, and a
  file that is not listed has none. The timers include a stub that answers late on purpose and a
  loop that asks again until a request has left; the count does not tell them from a plain wait.
  `[partial: ci:node; open: a wait written a third way, a number raised in the list]`
* **No answer the contract does not allow.** What a walk answers for `/api/v1` is what the server
  could answer: an operation `api/openapi.yaml` declares, with a status it declares, in a media
  type it declares and with a body its schema allows. The walks write their answers down and
  `test/contract` judges them. A wrong answer is counted once, by its kind, its operation and its
  status, however many suites give it. The 207 known when the gate landed are listed in
  `test/contract/testdata/walks-baseline.txt`; any other fails, and a line is only ever removed
  from that list. One answer is written down and not judged: the empty page `fixture.mjs` gives
  to a request that nothing stubbed. Nobody wrote it for the operation, and which requests a walk
  leaves unanswered until it ends differs from run to run. Only `fixture.mjs` marks an answer so.
  `[partial: ci:engines, ci:node, gate-walks, gate-pr; open: another wrong body for an operation and
  status the baseline lists (`make gate-walks` prints how many answers that is); the fixture's empty page for a request nothing stubbed; the lines of
  an answer in application/x-ndjson, which the contract gives no schema; a body taken from a file;
  response headers; whether the answer is the one the server gives for that request]`
* **No private copy of an answer that several suites need.** Such an answer lives in
  `fixture.mjs`, where every suite and the preview (`preview.mjs`) get it; a suite stubs only what
  is particular to its walk.
  `[unchecked: whether two suites need the same answer is judgement; the walks' check prints how many route stubs the suites hold]`
* **No element found in a way a person could not.** A walk finds a control by its role and its
  name. A `data-` attribute is used only where the element has no role.
  `[unchecked: no tool tells whether a role would have found the element]`
* **No write asserted once.** A walk that changes something asserts both the request that left the
  browser and what the screen shows afterwards. The fixture agrees with whatever the client sends,
  so the screen alone proves nothing about the request.
  `[unchecked: no tool reads what a walk asserts]`
* **No check walked at other widths or by another path than it names.** Where a check names a
  width or the keyboard, the walk uses that width and the keyboard.
  `[unchecked: no tool compares a walk with the words of its check]`
* **No check that crosses the server proven by a walk alone.** Such a check is proven by the walk
  and by a test of the same operation against the real server — an integration or a contract
  test — and the use case's `checked_by` names both.
  `[unchecked: the reviewer reads checked_by]`
* **No walk that passes without the change it proves.** A walk written for a change fails on the
  build without that change.
  `[unchecked: the reviewer runs the walk against the build without the change]`

## How to run a walk

```bash
make e2e SUITES="terms board"            # build the web app, walk these suites in Chromium
make e2e SUITES="terms board" RECORD=1   # the same, then hold their answers to the contract
make e2e                                 # every suite, as CI's browser job does
```

With `RECORD=1` each suite writes its answers to one file, in a directory of this run under
`apps/webapp/.walk-record/`, and `make gate-walks` runs afterwards. The gate judges the suites of
the newest run only, so it works for one suite and for one of CI's three shards. The directories
of earlier runs stay until somebody removes them. `ENGINES=firefox,webkit` walks the other
engines.

## How to check a change

```bash
node --test apps/webapp/build/walks.test.js   # the lists in bounds.json, in seconds
make gate-walks                               # after a recorded walk: the answers against the contract
```

When a fixed wait is removed or a suite cites its check, the first command says which entry of
`bounds.json` to lower or remove. When an answer is corrected, walk every suite with
`make e2e RECORD=1`; `WALK_BASELINE=prune make gate-walks` then removes the baseline's lines that
no suite met. It refuses where a suite has no record in that run.
