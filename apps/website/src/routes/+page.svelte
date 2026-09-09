<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Levels from '$lib/Levels.svelte';
  import Proof from '$lib/Proof.svelte';

  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>Hubtask — task management, built like infrastructure</title>
  <meta
    name="description"
    content="A self-hostable task manager with five levels, an API that is the product, a native MCP server for agents, and guarantees about backup, audit and deletion that are checked in a pipeline. Source available under BSL 1.1, free for private use."
  />
</svelte:head>

<section class="hero">
  <div class="wrap">
    <p class="kicker">Self-hosted · API first · Agent ready</p>
    <h1>Built like infrastructure.<br />Shaped like a to‑do&nbsp;list.</h1>
    <p class="lede">
      Hubtask holds “buy milk” and a client’s release plan in the same five levels. It is yours to
      run on your own machine, its API is the product rather than an export of a screen, and the
      things it promises about your data — restorable backups, a verifiable trail, deletion that
      announces itself — are checked in a pipeline instead of asserted in a brochure.
    </p>

    <Levels />

    <p class="cta-row">
      <a class="cta" href="/self-hosting/">Run it yourself</a>
      <a class="cta-quiet" href="/product/">See what it does</a>
    </p>
    <p class="hero-fineprint">
      Free for private use, for non-profits and for evaluation. Source available under BSL 1.1, and
      every version becomes Apache-2.0 three years after it is published.
      <a href="/licence/">What that means</a>
    </p>

    <dl class="figures">
      <div class="figure">
        <dt>5</dt>
        <dd>levels, from an area of life down to a single tick</dd>
      </div>
      <div class="figure">
        <dt>200+</dt>
        <dd>use cases, each reachable from all three doors</dd>
      </div>
      <div class="figure">
        <dt>1</dt>
        <dd>dependency to operate: PostgreSQL</dd>
      </div>
      <div class="figure">
        <dt>0</dt>
        <dd>bytes sent anywhere you did not choose</dd>
      </div>
    </dl>

    <!-- The maturity stage ADR-0035 puts in place of a second version number, said in public. It
         is the honest frame for everything below, and for this audience it reads as seriousness
         rather than as a warning. -->
    <p class="site-standing">
      <strong>Where this stands.</strong> The server is in active development and complete through
      milestone <code>0.7</code> — the model, the API, automation, multi-workspace operation,
      backup, audit and the agent surface are built and tested. The browser interface is at preview
      stage, and the installed applications for desktop and mobile are being built.
      <a href="/roadmap/">The plan, milestone by milestone</a>
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The model</p>
      <h2>Five levels, one shape</h2>
      <p class="lede">
        Most task tools give you two levels and ask you to fake the rest with tags. Hubtask gives
        you five, and lets each one carry only what makes sense at that depth.
      </p>
    </div>

    <div class="site-rows">
      <div class="site-row">
        <h3>Hub</h3>
        <div>
          <p>
            An area of life or a client. Work, the household, a customer you invoice — the boundary
            you would draw if somebody asked what you are responsible for.
          </p>
        </div>
      </div>
      <div class="site-row">
        <h3>Collection</h3>
        <div>
          <p>
            A project or a list inside a hub, with its own buckets, labels and saved views. This is
            the level a board is drawn at.
          </p>
        </div>
      </div>
      <div class="site-row">
        <h3>Task</h3>
        <div>
          <p>
            The unit of work, and the level that carries everything: due date and reminder,
            assignee and members, labels, comments, attachments, a cover, custom fields, and a
            recurrence rule that applies to the whole subtree.
          </p>
        </div>
      </div>
      <div class="site-row">
        <h3>Work package</h3>
        <div>
          <p>
            A step of a task. Notes, labels, comments and attachments, its own due date and its own
            assignee — a task’s spine, not a second task pretending.
          </p>
        </div>
      </div>
      <div class="site-row">
        <h3>Activity</h3>
        <div>
          <p>
            The smallest tick. One assignee, a due date, a reminder, a compact history — and
            deliberately nothing else, so a checklist stays a checklist.
          </p>
        </div>
      </div>
    </div>

    <div class="callout">
      <p>
        <strong>Why this is not just a deeper tree.</strong> Which fields a level carries is a
        capability profile rather than five hard-coded entities. Setting a field a level does not
        carry is refused with a named error, never ignored in silence — and a sixth level would be
        a profile entry, not a database migration.
      </p>
      <Proof href="{docs}/architecture/domain-model.md" label="domain-model.md §2 · the capability matrix" />
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The contract</p>
      <h2>Three doors, one catalogue</h2>
      <p class="lede">
        A capability exists once, in one registry, and every door opens onto the same room. There is
        no “API version” of a feature that lags behind the interface — a check in the build fails
        when the three ever disagree.
      </p>
    </div>

    <div class="site-cards">
      <div class="site-card">
        <p class="site-card-index">01</p>
        <h3>REST</h3>
        <p>
          A documented HTTP API with a composable query language, idempotency keys, optimistic
          concurrency and machine-readable errors. Written as a specification first and implemented
          from it, never generated out of a UI.
        </p>
        <Proof href="{docs}/architecture/api-guidelines.md" label="api-guidelines.md" />
      </div>
      <div class="site-card">
        <p class="site-card-index">02</p>
        <h3>MCP</h3>
        <p>
          A Model Context Protocol server inside your own installation. An agent gets tools,
          resources and prompts, a scoped credential, and exactly the permissions the credential
          holds — no more than a person with the same role.
        </p>
        <Proof href="{docs}/architecture/ai-first.md" label="ai-first.md §1" />
      </div>
      <div class="site-card">
        <p class="site-card-index">03</p>
        <h3>Automation</h3>
        <p>
          A rule engine with triggers, conditions and a dry run, plus signed webhooks with retries
          and a dead-letter path. Every use case is available as an action because it is the same
          registration.
        </p>
        <Proof href="{docs}/architecture/automation.md" label="automation.md" />
      </div>
    </div>

    <div class="callout callout-quiet">
      <p>
        Every incumbent now ships an MCP server, and most of them are a hosted endpoint in somebody
        else’s cloud — your tasks travel there to be read. Hubtask’s is an adapter inside the
        instance you run. The agent talks to your box, and every one of its actions lands in the
        trail marked as an agent’s.
      </p>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The difference</p>
      <h2>Provably yours</h2>
      <p class="lede">
        Every tool in this category says your data is safe. These are the four sentences that can be
        checked — each one a mechanism with a test behind it, not a value on an About page.
      </p>
    </div>

    <div class="site-cards site-cards-2 site-cards-signature">
      <div class="site-card">
        <h3>Backups that have been restored</h3>
        <p>
          Encrypted archives on a target you choose — local, S3-compatible, SFTP or WebDAV — with
          RRULE schedules and generational retention. The restore drill runs in the pipeline on
          every change, because a backup nobody has restored is a hypothesis.
        </p>
        <Proof href="{docs}/architecture/backup-restore.md" label="backup-restore.md" />
      </div>
      <div class="site-card">
        <h3>A trail that verifies</h3>
        <p>
          Every auditable action lands in a hash chain that stores no content of yours. One call
          checks the chain and names the first broken link, so tampering is found rather than
          suspected — and the application’s database role may not update or delete a row of it.
        </p>
        <Proof href="{docs}/architecture/audit.md" label="audit.md" />
      </div>
      <div class="site-card">
        <h3>Deletion with brakes</h3>
        <p>
          A retention rule says what it would delete before it deletes anything, keeps a grace
          period, can be stopped, and is overruled by a legal hold. Nothing disappears quietly, and
          writing such a rule needs the owner’s authority.
        </p>
        <Proof href="{docs}/architecture/data-retention.md" label="data-retention.md" />
      </div>
      <div class="site-card">
        <h3>GDPR as use cases</h3>
        <p>
          Access, export and erasure are operations with a statutory deadline the system watches,
          served by the product rather than by a support ticket. Every personal field is in a
          catalogue with the path that deletes it.
        </p>
        <Proof href="{docs}/privacy/data-catalog.md" label="data-catalog.md" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Operations</p>
      <h2>One image, one database, nobody to ask</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          PostgreSQL is the only thing Hubtask requires. It is also the queue, the event outbox, the
          full-text index and the tenant boundary — one dependency to back up, patch and understand
          instead of five.
        </p>
        <p>
          Compose for one box, Helm for a fleet, the same multi-architecture image for both. The
          tenant boundary is enforced by the database through row level security rather than by a
          <code>WHERE</code> clause somebody has to remember, and the health endpoint reports what is
          degraded instead of the process falling over.
        </p>
        <Proof href="{docs}/architecture/deployment.md" label="deployment.md" />
      </div>
      <div>
        <pre class="site-shell"><code><span class="c"># One box, everything on it</span>
docker compose up -d

<span class="c"># A fleet, the same image</span>
helm upgrade --install hubtask ./k8s

<span class="c"># Ask an installation what it can do</span>
curl /api/v1/meta/capabilities</code></pre>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Who runs it</p>
      <h2>Four people this was built for</h2>
    </div>
    <div class="site-cards">
      <div class="site-card">
        <h3>You, on your own hardware</h3>
        <p>
          Free, complete, with no feature held back and nothing phoning home. The licence converts
          to Apache-2.0 on a schedule, so an abandoned project is still yours.
        </p>
        <a href="/use-cases/#individual">How that looks</a>
      </div>
      <div class="site-card">
        <h3>An agency with several clients</h3>
        <p>
          One instance, one workspace per client, a boundary the database enforces — and a backup
          and an export per workspace rather than one undifferentiated dump.
        </p>
        <a href="/use-cases/#provider">How that looks</a>
      </div>
      <div class="site-card">
        <h3>A team with obligations</h3>
        <p>
          An audit trail that verifies, retention with a legal hold, data subject requests with a
          deadline, and an accessibility standard the clients are measured against.
        </p>
        <a href="/security/">What it gives you</a>
      </div>
      <div class="site-card">
        <h3>Somebody automating their work</h3>
        <p>
          A real API, a rule engine, signed webhooks, a CLI that speaks the published contract, and
          an MCP server that runs where your data already is.
        </p>
        <a href="/developers/">Read the contract</a>
      </div>
    </div>
  </div>
</section>

<section class="section closing">
  <div class="wrap">
    <p class="kicker">Where to start</p>
    <h2>Take it and run it</h2>
    <p class="lede">
      Nothing to sign up for, nobody to talk to, no trial that expires. Pull the image, point it at a
      PostgreSQL, and read the architecture while it starts.
    </p>
    <p class="cta-row">
      <a class="cta" href="/download/">Get Hubtask</a>
      <a class="cta-quiet" href="https://github.com/Jersyfi/hubtask">Source on GitHub</a>
    </p>
  </div>
</section>
