<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Levels from '$lib/Levels.svelte';
  import Proof from '$lib/Proof.svelte';

  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>What Hubtask does — the product</title>
  <meta
    name="description"
    content="Five levels with capability profiles, four layouts, a composable query language, recurrence across time zones, templates, calendar feeds and CalDAV, imports, sign-in on your own rules, automation rules, an inbox for anything unsorted, backups, an installation level for operators, and offline work that does not lose concurrent edits."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">The product</p>
    <h1>Everything it does, and where each thing lives</h1>
    <p class="lede">
      Hubtask is one model with a lot of surface. This page walks the surface — what you can put on
      a task, how you look at a list, who gets in and how, what happens while you are asleep, what
      it does when the network is gone, and how an installation is run. What is decided and still
      being built is marked <span class="site-chip site-chip-amber">Planned</span>.
    </p>
    <Levels />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The hierarchy</p>
      <h2>What each level carries</h2>
      <p>
        The five levels are not five entities. They are one item with a <strong>capability
        profile</strong>, and the profile decides what the level accepts. A field the level does not
        carry is refused with a named error rather than silently dropped — so a client, a script and
        an agent all find out the same thing at the same moment.
      </p>
    </div>

    <div class="table-scroll">
      <table>
        <caption class="visually-quiet">The capability matrix, as the domain model defines it</caption>
        <thead>
          <tr>
            <th scope="col">Capability</th>
            <th scope="col">Task</th>
            <th scope="col">Work package</th>
            <th scope="col">Activity</th>
          </tr>
        </thead>
        <tbody>
          <tr><th scope="row">Completion</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td></tr>
          <tr><th scope="row">Due date</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td></tr>
          <tr><th scope="row">Reminders</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td></tr>
          <tr><th scope="row">Assignment</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="part">one assignee</span></td></tr>
          <tr><th scope="row">Several members</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">Notes, labels, comments</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">Attachments</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">Custom fields</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">Bucket on a board</th><td><span class="yes">yes</span></td><td><span class="no">no</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">Cover (colour or image)</th><td><span class="yes">yes</span></td><td><span class="no">no</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">Recurrence</th><td><span class="yes">yes</span></td><td><span class="no">no</span></td><td><span class="no">no</span></td></tr>
          <tr><th scope="row">History</th><td><span class="yes">yes</span></td><td><span class="yes">yes</span></td><td><span class="part">compact</span></td></tr>
        </tbody>
      </table>
    </div>
    <Proof href="{docs}/architecture/domain-model.md" label="domain-model.md §2" />

    

    <div class="callout">
      <p>
        <strong>A workspace may narrow a profile, never widen it past the system boundary.</strong>
        If your team decides activities carry no due date, that is a setting. If somebody wants a
        sixth level, that is a profile entry and a permitted-child-type change — no schema
        migration, no API break.
      </p>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Looking at work</p>
      <h2>Four layouts and a query language</h2>
    </div>

    <div class="site-cards">
      <div class="site-card">
        <h3>List</h3>
        <p>Collapsed or expanded, with the subtree visible where you want it and folded where you do not.</p>
      </div>
      <div class="site-card">
        <h3>Kanban</h3>
        <p>
          Buckets under a collection, with a work-in-progress limit and a done column that are
          <em>announced</em> rather than enforced — the server never refuses a card for crossing a
          limit somebody set as a reminder to themselves.
        </p>
      </div>
      <div class="site-card">
        <h3>Timeline</h3>
        <p>
          A span where an item has a start and a due date, a point where it only has a due date, and
          a list beside the axis for everything undated — because hiding the undated would be a
          filter nobody chose.
        </p>
      </div>
      <div class="site-card">
        <h3>Saved views</h3>
        <p>
          A query, a layout hint and a share setting, stored and re-openable. Exportable as CSV,
          JSON or ICS.
        </p>
      </div>
    </div>

    <div class="two-up section-gap">
      <div>
        <h3>The query language</h3>
        <p>
          Filtering is a composable expression over fields the installation actually has —
          including the custom fields you defined — with comparisons the server declares rather than
          a client guessing. It is the same language behind a saved view, an export, an automation
          condition and an agent’s search.
        </p>
        <p>
          No byte of a request ever becomes SQL text. That is not a habit; it is a gate in the
          build.
        </p>
        <Proof href="{docs}/architecture/api-guidelines.md" label="api-guidelines.md · the query DSL" />
      </div>
      <div>
        <h3>Search</h3>
        <p>
          Full-text search across titles, notes and comments, in the language of the content.
          Where the database offers the vector extension, a semantic pass joins it and the two are
          ranked into one page — the words winning over the meaning, and one cursor for both.
        </p>
        <p>
          Where the extension is absent, search is lexical, complete, and says so through the
          capability manifest rather than failing.
        </p>
        <Proof href="{docs}/architecture/ai-first.md" label="ai-first.md §2 · hybrid search" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Time</p>
      <h2>Dates that survive a time zone change</h2>
    </div>
    <ul class="ticks ticks-2">
      <li>
        <strong>All-day and timed due dates</strong>
        A day is a day and a moment is a moment; they are different things and are stored
        differently. An item may carry its own time zone.
      </li>
      <li>
        <strong>Recurrence by RFC&nbsp;5545</strong>
        Real RRULEs, repeating either on schedule or on completion, with the transition across
        daylight saving covered by tests rather than by hope.
      </li>
      <li>
        <strong>Reminders, relative or absolute</strong>
        “Half an hour before it is due” or a fixed moment, several per item, delivered when they
        are due — with punctuality as a measured objective, not a promise.
      </li>
      <li>
        <strong>Templates</strong>
        A whole tree of tasks, work packages and activities with relative dates, stamped out in one
        call and anchored to a date you name.
      </li>
      <li>
        <strong>Calendar feeds</strong>
        A revocable ICS URL for any saved view, readable by any calendar application. The URL is
        the credential, it is shown once, and it can be withdrawn.
      </li>
      <li>
        <strong>CalDAV, both ways</strong>
        Tick a task off or move its date in the calendar app you already use, and the change goes
        through the same rules as one made in Hubtask.
      </li>
      <li>
        <strong>Live updates</strong>
        A server-sent event stream, so a second window and a second device do not disagree about
        what is done.
      </li>
    </ul>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Working with other people</p>
      <h2>Collaboration and content</h2>
    </div>
    <div class="site-rows">
      <div class="site-row">
        <h3>Members and assignment</h3>
        <div><p>
          An assignee or a set of members, depending on the level. Automatic assignment by policy
          where you want the system to choose, and a named refusal when no candidate can take it —
          never a silent drop.
        </p></div>
      </div>
      <div class="site-row">
        <h3>Comments</h3>
        <div><p>
          Threaded, editable, with “removed” as a visible state of its own rather than a hole in
          the conversation.
        </p></div>
      </div>
      <div class="site-row">
        <h3>Attachments and covers</h3>
        <div><p>
          Files go to object storage or the local volume by a presigned upload the server never
          carries the bytes for, and are usable only after a confirmation step that reads them back.
          A cover is a colour from the palette or an image.
        </p></div>
      </div>
      <div class="site-row">
        <h3>Custom fields</h3>
        <div><p>
          Eight kinds of field, defined per collection, filterable in the query language like any
          built-in field.
        </p></div>
      </div>
      <div class="site-row">
        <h3>Roles</h3>
        <div><p>
          Seven roles across four scopes — workspace, hub, collection, item — with an auditor role
          that reads the trail and the configuration and nothing else. Permission is resolved in one
          place, never in an adapter and never in a repository.
        </p>
        <Proof href="{docs}/architecture/domain-model.md" label="domain-model.md §3 · the role matrix" /></div>
      </div>
      <div class="site-row">
        <h3>History</h3>
        <div><p>
          Every change on an item, with the actor and what changed — and for activities, a compact
          form, because a tick does not need a paragraph.
        </p></div>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Who gets in</p>
      <h2>Signing in, on your own rules</h2>
    </div>
    <div class="site-rows">
      <div class="site-row">
        <h3>The password rule</h3>
        <div><p>
          Eighteen switches in three groups — the password, the second factor, the session — set by
          the installation with a lock where it decides, and tightened by a workspace where it may.
          Shipped at what NIST advises: length over composition, common passwords refused, no forced
          expiry. The rules show under the field as you type, in your language.
        </p>
        <Proof href="{docs}/adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md" label="ADR-0068" /></div>
      </div>
      <div class="site-row">
        <h3>A second factor, and the way back</h3>
        <div><p>
          An authenticator app, required of administrators or of everybody. Ten recovery codes you
          can copy and replace. A forgotten password is reset by mail, and the reset still asks for
          the second factor. Every session is listed and can be ended.
        </p></div>
      </div>
      <div class="site-row">
        <h3>Google, Microsoft, your own directory</h3>
        <div><p>
          Several providers per workspace, each with its own button. A provider admits by directory,
          never by the text of an address, and connecting it to an account that already exists asks
          for that account’s own proof first. An installation can offer providers to every workspace
          without showing any of them a secret.
        </p>
        <Proof href="{docs}/adr/ADR-0071-provider-admission.md" label="ADR-0071" /></div>
      </div>
      <div class="site-row">
        <h3>Tokens and apps</h3>
        <div><p>
          Personal access tokens with scopes and an expiry, service accounts for machines, and
          third-party apps a person lets act for them through OAuth — and takes back.
        </p></div>
      </div>
      <div class="site-row">
        <h3>Next</h3>
        <div><p>
          <span class="site-chip site-chip-amber">Planned</span> Passkeys. Terms of use agreed at the invitation and again when they change.
          Accounts for a child or a grandparent without a mail address. Private hubs another
          administrator cannot open. People arriving from a directory landing with a role.
        </p></div>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">In, out, and back</p>
      <h2>Nothing is stuck, nothing is lost</h2>
    </div>
    <ul class="ticks ticks-2">
      <li>
        <strong>Imports</strong>
        From a spreadsheet, Trello, Google Tasks or Microsoft To Do, with a report of what came in
        and what was refused and why. Running the same import twice creates nothing twice.
      </li>
      <li>
        <strong>Trash and archive</strong>
        A deleted hub, collection or task waits in the trash, thirty days by default; finished work is archived
        rather than deleted, and both come back with their history.
      </li>
      <li>
        <strong>Retention you can see coming</strong>
        A rule shows what it would remove before it acts, warns the people whose work it touches,
        and a legal hold stops it.
      </li>
      <li>
        <strong>Backups that are checked</strong>
        Encrypted, to your own target, on a schedule with generations, and opened by the code that
        would restore them. Restore the whole workspace or bring one hub back.
      </li>
      <li>
        <strong>Mail when it matters</strong>
        When work lands on you, before something is due, when something you set up stops working —
        and each person chooses which mails they get. <span class="site-chip site-chip-amber">Planned</span> Channels other than the mailbox,
        and rules that tell people something.
      </li>
      <li>
        <strong>Small things that save time</strong>
        Duplicate a task with its subtree, change many entries at once, see what is on you when you
        arrive, a short tour on the first visit, and a small celebration when work gets done —
        switchable per account.
      </li>
    </ul>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">While you are away</p>
      <h2>Automation, intake and agents</h2>
    </div>
    <div class="site-cards site-cards-2">
      <div class="site-card">
        <h3>Rules</h3>
        <p>
          Triggers on what happens, conditions in a real expression language, and actions drawn
          from the same catalogue a person uses. A dry run shows what a rule would do before it is
          allowed to do it, and loop and throttle protection stop a rule from feeding itself.
        </p>
        <Proof href="{docs}/architecture/automation.md" label="automation.md" />
      </div>
      <div class="site-card">
        <h3>The inbox for anything unsorted</h3>
        <p>
          Quick capture, an inbound webhook and an e-mail address that can be rotated — everything
          arrives in one place and becomes work when somebody converts it. Optionally with a
          suggestion attached, which changes nothing until it is accepted.
        </p>
      </div>
      <div class="site-card">
        <h3>Webhooks out</h3>
        <p>
          Signed subscriptions with retries and a dead-letter path, carrying published CloudEvents
          schemas, a secret that can be replaced without dropping a delivery, and a replay of what
          a server missed. Connectors for n8n and Zapier are generated from the same contract,
          built and tested, and published with the convergence milestone.
        </p>
      </div>
      <div class="site-card">
        <h3>AI, entirely optional</h3>
        <p>
          Suggestions for the inbox and for breaking a task down, summaries, semantic search — with
          the provider behind a port, configurable per workspace, and Ollama on your own machine as
          a first-class choice. Switched off, none of it is there, and the rest of the product is
          unchanged.
        </p>
        <p>
          <span class="site-chip site-chip-amber">Planned</span> A model the provider offers, switched on with one click and counted against a
          budget; and each person keeping their own content out, unless the workspace names a
          legal basis for AI as part of everybody’s work.
        </p>
        <Proof href="{docs}/architecture/ai-first.md" label="ai-first.md §2" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Away from the network</p>
      <h2>Offline, without losing what somebody else did</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          The web app keeps a copy of your workspace in the browser and goes on working when the
          connection drops: changes wait in a queue and are sent when it returns, and the devices
          you used are listed and can be forgotten. The data model has carried what this needs since
          the first migration — a change log, tombstones, a clock per field change and fractional
          indices. <span class="site-chip site-chip-amber">Planned</span> The installed desktop and mobile apps, and reminders, recurrence and
          templates changed while offline.
        </p>
        <p>
          <strong>No client merges.</strong> Merging happens on the server, per field, with
          conflict-preserving rules — last write wins where that is right, an order-preserving set
          where it is not, a fractional index for position. Two people editing the same task at the
          same time must not produce a lost edit and a shrug, and that is a property of the merge
          rather than of any one client.
        </p>
        <Proof href="{docs}/architecture/offline-sync.md" label="offline-sync.md §4" />
      </div>
      <div>
        <h3>Multilingual by construction</h3>
        <p>
          The backend never emits display text — it emits a message code and parameters, and the
          client renders it. That is what makes a new language a catalogue file rather than a
          release, and it is why right-to-left and a forty-per-cent-longer German string are
          requirements on every client rather than a later project.
        </p>
        <Proof href="{docs}/architecture/i18n-l10n.md" label="i18n-l10n.md" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Running the installation</p>
      <h2>One level above every workspace</h2>
      <p class="lede">
        For whoever runs Hubtask for others — and, on a private installation, simply one more
        section of the administration.
      </p>
    </div>
    <div class="site-cards site-cards-2">
      <div class="site-card">
        <h3>Operators, and an hour at a time</h3>
        <p>
          A register of the people and machines who may run the installation, separate from every
          workspace’s administrators. A person raises their own session for one hour after a fresh
          proof, and every start and end is in the installation’s journal.
        </p>
        <Proof href="{docs}/adr/ADR-0070-the-instance-layer.md" label="ADR-0070" />
      </div>
      <div class="site-card">
        <h3>Defaults with a lock</h3>
        <p>
          Sign-in rules, legal texts, limits and language defaults set once for every workspace —
          each with a lock that decides whether a workspace may change it, and a screen that says
          which level decided. From the dashboard, from <code>hubctl</code>, or from a file.
        </p>
      </div>
      <div class="site-card">
        <h3>The dashboard</h3>
        <p>
          Workspaces and their state — create, suspend, resume, delete, export — limits per
          workspace, the health report, the journal, and key rotation with a census of what moved.
          Numbers and states, never a workspace’s content.
        </p>
      </div>
      <div class="site-card">
        <h3>Next</h3>
        <p>
          <span class="site-chip site-chip-amber">Planned</span> The first start in the browser with a one-time code. AI models the installation
          offers, with a budget per source. Plans with their own limits, features and locks.
          Events that tell a purchase platform what happened. A workspace’s own domain.
        </p>
        <a href="/roadmap/">The order they come in</a>
      </div>
    </div>
  </div>
</section>
