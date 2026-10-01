<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>Who Hubtask is for — seven ways it is run</title>
  <meta
    name="description"
    content="A private person, a family, a club, a company running it itself, a provider for consumers, a provider for companies, and a managed service provider: what each of them gets today, and what is planned."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Who it is for</p>
    <h1>Seven ways to run it, one product</h1>
    <p class="lede">
      From one person on a box at home to a provider with thousands of workspaces, every shape runs
      the same product. There is no edition and no feature held back. Each section says what works
      today; what is decided and still being built is marked <span class="site-chip site-chip-amber">Planned</span>.
    </p>
  </div>
</section>

<section class="section" id="individual">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">01 · A private person</p>
      <h2>Your life, on hardware you own</h2>
      <p class="lede">
        “Will this still be mine in five years, and can I get my data out?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          A hub for work, one for the household, one for the flat you are renovating. Under each, the
          lists you actually keep. Recurring bills, a shopping list, the maintenance schedule for a
          bike — and the whole thing on a machine in your own home, with the full feature set and
          nothing withheld.
        </p>
        <p>
          <strong>Free, and free with the condition stated out loud.</strong> Private and household use
          is covered by the licence’s use grant, with no feature gate and no licence key — and the
          project makes no promise about maintenance until it sells something. If it stops tomorrow,
          every version you are running becomes Apache-2.0 three years after it was published. That
          is written into the licence rather than promised in a blog post.
        </p>
        <a href="/licence/">Read the licence terms</a>
      </div>
      <div>
        <h4>What matters here</h4>
        <ul class="ticks">
          <li><strong>Nothing phones home</strong> No telemetry, no licence check, no font loaded from a foreign domain.</li>
          <li><strong>Backups you configure</strong> Your own target, your own schedule, encrypted, with a restore that has been drilled.</li>
          <li><strong>Export whenever</strong> CSV, JSON and ICS from any view; a full workspace export as a use case.</li>
          <li><strong>One container and a database</strong> The homelab standard, not a stack of five services.</li>
          <li><strong>A forgotten password is survivable</strong> Recovery codes you can copy and replace, without a mail server.</li>
          <li><strong>Set up in the browser</strong> <span class="site-chip site-chip-amber">Planned</span> The first start prints a one-time code, and the browser creates your workspace and account — no terminal, no database shell.</li>
          <li><strong>Passkeys</strong> <span class="site-chip site-chip-amber">Planned</span> Signing in without a password, as its own milestone.</li>
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section" id="family">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">02 · A family</p>
      <h2>Shared where it helps, private where it should be</h2>
      <p class="lede">
        “Can we share the groceries and the holidays without everybody reading everything?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          One installation for the household: a hub for the groceries, one for the holidays, one each
          for the things that are nobody else’s business. A person can be a member of one hub and see
          nothing else, which is how a child gets their own corner. Reminders, recurring chores and
          templates for the weekly routine work the same for everybody.
        </p>
        <p>
          <strong>The technical parent sets the rules once.</strong> Who may sign in how, who needs a
          second factor, what is kept and for how long — set in the administration, and the others
          never see a settings screen.
        </p>
      </div>
      <div>
        <h4>What matters here</h4>
        <ul class="ticks">
          <li><strong>A hub per person or topic</strong> Membership on one hub reaches that hub and nothing above it.</li>
          <li><strong>Private hubs</strong> <span class="site-chip site-chip-amber">Planned</span> A hub another adult cannot open just because they are an administrator too, with an emergency access that tells every member at once.</li>
          <li><strong>Accounts without a mail address</strong> <span class="site-chip site-chip-amber">Planned</span> A child or a grandparent signs in with a name and a start password Hubtask makes, and changes it at the first sign-in.</li>
          <li><strong>Invitations without mail</strong> <span class="site-chip site-chip-amber">Planned</span> Handed over as a link or a code when the household runs no mail server.</li>
        </ul>
        <Proof href="{docs}/adr/ADR-0073-private-hubs.md" label="ADR-0073 · private hubs" />
      </div>
    </div>
  </div>
</section>

<section class="section" id="club">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">03 · A club or a small team</p>
      <h2>Everybody knows what is theirs</h2>
      <p class="lede">
        “Can we hand out the jobs, remind people, and keep strangers out?”
      </p>
    </div>
    <div class="site-cards site-cards-2">
      <div class="site-card">
        <h3>Invitations, roles and groups</h3>
        <p>
          Invite people with a role on the whole workspace or on one hub, group them, and let a
          collection hand out new work automatically — a fixed person, round robin, least loaded,
          or somebody from a named group, with a named refusal when nobody can take it.
        </p>
      </div>
      <div class="site-card">
        <h3>Reminders that arrive</h3>
        <p>
          A mail when work lands on somebody, reminders before something is due, and each person
          chooses which mails they get.
        </p>
      </div>
      <div class="site-card">
        <h3>A second factor for those who can change things</h3>
        <p>
          Require one of owners and administrators, or of everybody. Recovery codes can be copied
          and replaced, and a reset never walks past a second factor.
        </p>
      </div>
      <div class="site-card">
        <h3>Google or Microsoft, for those who prefer it</h3>
        <p>
          Members may sign in with the account they already have — and a stranger with a Google
          account gets nothing, because the provider admits only the people this club invited.
        </p>
        <Proof href="{docs}/adr/ADR-0071-provider-admission.md" label="ADR-0071 · who a provider admits" />
      </div>
    </div>
  </div>
</section>

<section class="section" id="team">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">04 · A company running it itself</p>
      <h2>When somebody will ask you to prove it</h2>
      <p class="lede">
        “Can our directory sign people in, can I answer an access request, and can I show that the
        trail was not edited?”
      </p>
    </div>
    <div class="site-cards site-cards-2 site-cards-signature">
      <div class="site-card">
        <h3>Your directory, your rules</h3>
        <p>
          Sign-in through Entra ID, Google Workspace, Keycloak or any OpenID Connect provider,
          admitted by directory rather than by the text of an address. Password and session rules
          to the company’s policy — length, character classes, history, expiry, idle timeout — set
          once, with a lock where the installation decides. Connecting an existing account to the
          directory asks for that account’s own proof first.
        </p>
        <p>
          <span class="site-chip site-chip-amber">Planned</span> People arriving through the
          directory land with a role you chose, not in an empty workspace.
        </p>
        <Proof href="{docs}/adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md" label="ADR-0068 · the sign-in rule" />
      </div>
      <div class="site-card">
        <h3>The trail, and its verification</h3>
        <p>
          Auditable actions form a hash chain that stores no content. One call walks it and names
          the first broken link. The application’s own database role holds no update and no delete
          on that table, so an application defect cannot quietly rewrite history. An auditor role
          reads the trail and the configuration and never the content.
        </p>
        <Proof href="{docs}/architecture/audit.md" label="audit.md" />
      </div>
      <div class="site-card">
        <h3>Data subject rights</h3>
        <p>
          Access, export, rectification, restriction and erasure are use cases with the statutory
          deadline tracked by the system. Every personal field is in a catalogue that names where it
          lives and what deletes it — including the copies in backups and in the trail.
        </p>
        <p>
          <span class="site-chip site-chip-amber">Planned</span> A deadline extended once, with
          the reason; and an erasure that keeps exactly what a legal hold covers, and says so.
        </p>
        <Proof href="{docs}/architecture/data-protection.md" label="data-protection.md" />
      </div>
      <div class="site-card">
        <h3>Retention, legal hold, backups</h3>
        <p>
          Rules that announce what they would delete, keep a grace period and can be stopped — and a
          legal hold that overrides both them and somebody emptying their own trash. Backups go to
          the company’s own target, encrypted, and restore the whole workspace or one hub.
        </p>
        <Proof href="{docs}/architecture/data-retention.md" label="data-retention.md" />
      </div>
      <div class="site-card">
        <h3>AI on your terms</h3>
        <p>
          Off until the workspace consents, with the processor and where it sits always named. Your
          own model or a local one, never a default.
        </p>
        <p>
          <span class="site-chip site-chip-amber">Planned</span> Each person may keep their own
          content out — or the company makes AI part of everybody’s work and names the legal basis,
          which everybody then sees.
        </p>
        <Proof href="{docs}/vision/principles.md#p-14-ai-is-optional-and-nobody-consents-in-anothers-place" label="P-14 · nobody consents in another's place" />
      </div>
      <div class="site-card">
        <h3>Accessibility</h3>
        <p>
          Built to WCAG 2.2 AA — the standard the European Accessibility Act expects — with contrast
          measured on every test run rather than reviewed. The formal statement is published with
          the first stable release.
        </p>
        <a href="/accessibility/">How it is built</a>
      </div>
    </div>
  </div>
</section>

<section class="section" id="consumers">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">05 · A provider for consumers</p>
      <h2>Thousands of customers, one set of rules</h2>
      <p class="lede">
        “Can my shop create a workspace when somebody buys, and can my legal texts apply to all of
        them?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          Every customer gets a workspace of their own, behind the same database boundary as every
          other. Your purchase or sign-up platform creates it through the API, suspends it when a
          payment fails and resumes it when it arrives — with an idempotency key, so a webhook that
          arrives twice creates nothing twice. Hubtask holds the state; your platform holds the
          contract, the price and the invoice.
        </p>
        <p>
          <strong>Your imprint and privacy notice, on every sign-in page, locked.</strong> The
          installation sets a value and decides whether a workspace may change it. You see numbers
          and states for every workspace, never a customer’s content.
        </p>
      </div>
      <div>
        <h4>What matters here</h4>
        <ul class="ticks">
          <li><strong>A credential for the platform</strong> A service account provisions workspaces, not a token belonging to an employee. On the operator screen <span class="site-chip site-chip-amber">Planned</span>; through the API today.</li>
          <li><strong>Google and Microsoft sign-in</strong> Offered to every workspace by the installation, admitting only people who were invited.</li>
          <li><strong>Limits per workspace</strong> Visible before they hurt; a workspace at its limit is refused the next thing, never loses the last.</li>
          <li><strong>Terms agreed and re-agreed</strong> <span class="site-chip site-chip-amber">Planned</span> With the version and the moment on the account.</li>
          <li><strong>AI included</strong> <span class="site-chip site-chip-amber">Planned</span> A model the provider offers, one click for the customer, counted against a budget.</li>
          <li><strong>Plans</strong> <span class="site-chip site-chip-amber">Planned</span> Limits, features and locks per plan; a downgrade stops, never deletes. Its own milestone.</li>
          <li><strong>Sign in with Apple</strong> <span class="site-chip site-chip-amber">Planned</span> Needs a signed client secret and its own decision.</li>
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section" id="provider">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">06 · A provider for companies</p>
      <h2>Several clients, one instance, a real boundary</h2>
      <p class="lede">
        “Can one installation hold five clients without any chance of them seeing each other — and
        let each of them run their own rules?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          In multi-workspace mode each client is a workspace: its own hubs, its own members, its own
          labels, its own retention rules — and its own backup and its own export, rather than one
          undifferentiated dump you would have to cut up by hand if a client left.
        </p>
        <p>
          <strong>The boundary is in the database.</strong> Row level security enforces it, every
          query runs inside a transaction that has declared which tenant it is acting for, and a
          cross-tenant test exists for every repository method — a gate in the build fails when one
          is missing. It is not a <code>WHERE</code> clause somebody has to remember on a Friday.
        </p>
        <Proof href="{docs}/architecture/multi-tenancy.md" label="multi-tenancy.md" />
      </div>
      <div>
        <h4>What matters here</h4>
        <ul class="ticks">
          <li><strong>Each client configures itself</strong> Its own directory, legal texts and sign-in rules — within the defaults and locks you set for every workspace.</li>
          <li><strong>Providers you offer</strong> Sign-in providers configured once by the installation and switched on by each client, without seeing a secret.</li>
          <li><strong>Provisioning and suspension</strong> Creating, suspending and deleting a workspace are operations, not database surgery.</li>
          <li><strong>Quotas and fairness</strong> One noisy workspace does not become everybody’s latency.</li>
          <li><strong>An export a client can take</strong> Leaving is a supported operation, which is what makes staying a choice.</li>
          <li><strong>AI you offer, or theirs</strong> <span class="site-chip site-chip-amber">Planned</span> Offer your own model, let a client bring theirs, or require a minimum jurisdiction.</li>
          <li><strong>The client’s own domain</strong> <span class="site-chip site-chip-amber">Planned</span> Two DNS records to copy, the rest automatic — after passkeys and plans.</li>
        </ul>
      </div>
    </div>
    <div class="callout">
      <p>
        <strong>Running it commercially is free until Licensing Start</strong> — a date announced at
        least sixty days ahead, after which organisations of five persons or more need a licence for
        new versions. What you run before that day keeps its terms.
        <a href="/licence/">What is decided and what is not</a>
      </p>
    </div>
  </div>
</section>

<section class="section" id="msp">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">07 · A managed service provider</p>
      <h2>Every installation from a file, none by hand</h2>
      <p class="lede">
        “Can I bring up the fortieth installation exactly like the first, without a person at a
        browser?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          An installation’s defaults and locks — sign-in rules, legal texts, limits, language defaults — can
          live in one file:
          written once on the first start, or enforced on every start with the screens refusing to
          change it. Container images and a Helm chart, migrations that run as their own job and are
          safe during a rolling update, and a health report and metrics on a port of their own.
        </p>
        <Proof href="{docs}/architecture/deployment.md" label="deployment.md" />
      </div>
      <div>
        <h4>What matters here</h4>
        <ul class="ticks">
          <li><strong>The installation as code</strong> One file, seeded or enforced, and the health report says which applies.</li>
          <li><strong>An operator register</strong> The people and machines who may run the installation, separate from every workspace’s administrators, with an hour of elevated access after a fresh proof.</li>
          <li><strong>Key rotation with a census</strong> Rotate the installation’s keys and see which secrets moved.</li>
          <li><strong>Operators from the file</strong> <span class="site-chip site-chip-amber">Planned</span> Who runs the installation, named in the file and the environment, and a command in the server binary to get back into an installation nobody can run.</li>
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section" id="builder">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">And across all seven · The builder</p>
      <h2>A contract, not a screen with an export button</h2>
      <p class="lede">
        “Is the API real, or is it whatever the interface happened to need?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          The specification is written first and the code is generated from it; the interface is one
          client among several. Everything a person can do, a token can do — with idempotency keys
          so a retry does not duplicate, optimistic concurrency so a blind write does not clobber,
          and errors that carry a code and parameters rather than a sentence to parse. Client
          libraries for Go, TypeScript and Python are generated from the same document, and the
          n8n and Zapier connectors are built and tested, to be published with the convergence
          milestone.
        </p>
        <p>
          An agent gets the same catalogue over MCP, running inside your installation. It holds a
          scoped credential, it is refused exactly what that credential would be refused over HTTP,
          and every action it takes is in the trail marked as an agent’s.
        </p>
        <a href="/developers/">The developer page</a>
      </div>
      <div>
        <pre class="site-shell"><code><span class="c"># The whole product, from a terminal</span>
hubctl item create --collection "$C" \
  --type TASK --title "Renew the domain"

hubctl due set "$ID" --at 2027-01-31
hubctl recur set "$ID" --rule "FREQ=YEARLY"

<span class="c"># Follow the change stream</span>
hubctl watch

<span class="c"># Or pipe it</span>
hubctl search domain --json | jq '.items[].title'</code></pre>
      </div>
    </div>
  </div>
</section>
