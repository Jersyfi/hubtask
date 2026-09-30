# Working in docs/usecases

A use case is the owner's statement of what must be true for a person. It is the yardstick a task
is checked against, so it is changed with more care than the code it describes.

**What a session may change on its own:**

* `state:`, `tasks:` and `checked_by:` in the front matter — when a pull request builds or proves a
  check, it moves the state and names the evidence, in the same pull request.
* The *Today* section — which checks are not met yet, and where that is tracked.
* A new use case in `state: specified`, **only when the owner asked for it** in the conversation or
  in the task. Say so in the pull request body.

**What needs the owner:**

* *Goal*, *How to check* and *Where it ends*. If the code cannot meet a check, or a check turns out
  to be wrong, **stop and report** — in the issue and in the pull request — instead of softening
  the sentence. A check rewritten to match what was built is exactly the drift this folder exists
  to prevent.
* Removing or renumbering a use case. A use case that no longer applies moves to
  `state: retired` with a line saying why; its ID is never reused.

**Writing checks.** Each check under *How to check* is one observable fact a reviewer can confirm
without interpreting it: a person does X and sees Y; a request without Z is refused with code W; in
deployment D1 the screen does not show V. "Works well", "is intuitive", "handles errors" are not
checks. When a check can only be confirmed by walking the app, say which screen and which persona.

**Before building a task**, read every use case the task names, completely — including *Where it
ends*, which is what stops a session from building more than was asked. Measure the change against
the principles the use case serves ([`../vision/principles.md`](../vision/principles.md)).
