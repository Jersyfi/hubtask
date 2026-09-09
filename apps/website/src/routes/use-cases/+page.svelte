<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>Who Hubtask is for — four ways it is used</title>
  <meta
    name="description"
    content="A private person on one box, an agency running several client workspaces, a team with audit and retention obligations, and somebody automating their work through an API and an MCP server."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Who it is for</p>
    <h1>Four people, one instance</h1>
    <p class="lede">
      Hubtask was shaped around four readers. Each of them arrives with a different worry, and each
      of them is answered by a different part of the same product — not by a different edition.
    </p>
  </div>
</section>

<section class="section" id="individual">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">01 · The private person</p>
      <h2>Your life, on hardware you own</h2>
      <p class="lede">
        “Will this still be mine in five years, and can I get my data out?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          A hub for work, one for the household, one for the flat you are renovating. Under each, the
          lists you actually keep. Recurring bills, a shopping list your partner can see, the
          maintenance schedule for a bike — and the whole thing on a machine in your own home, with
          the full feature set and nothing withheld.
        </p>
        <p>
          <strong>Free, and free with a condition stated out loud.</strong> Private and household use
          is covered by the licence’s use grant, with no feature gate and no licence key. And if the
          project stops tomorrow, every version you are running becomes Apache-2.0 three years after
          it was published. That is written into the licence rather than promised in a blog post.
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
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section" id="provider">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">02 · The small provider</p>
      <h2>Several clients, one instance, a real boundary</h2>
      <p class="lede">
        “Can one installation hold five clients without any chance of them seeing each other?”
      </p>
    </div>
    <div class="two-up">
      <div>
        <p>
          In multi-workspace mode each client is a tenant: its own hubs, its own members, its own
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
          <li><strong>Provisioning and suspension</strong> Creating, suspending and deleting a workspace are operations, not database surgery.</li>
          <li><strong>Quotas and fairness</strong> One noisy workspace does not become everybody’s latency.</li>
          <li><strong>Sign-in you already run</strong> Connect your identity provider; enforce multi-factor per workspace.</li>
          <li><strong>An export a client can take</strong> Leaving is a supported operation, which is what makes staying a choice.</li>
        </ul>
      </div>
    </div>
    <div class="callout">
      <p>
        <strong>Running it commercially needs a paid licence.</strong> This is the case the licence
        reserves, and the terms are being settled before 1.0.
        <a href="/licence/">What is decided and what is not</a>
      </p>
    </div>
  </div>
</section>

<section class="section" id="team">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">03 · The team with obligations</p>
      <h2>When somebody will ask you to prove it</h2>
      <p class="lede">
        “Can I answer an access request, show that the trail was not edited, and point at an
        accessibility statement?”
      </p>
    </div>
    <div class="site-cards site-cards-2 site-cards-signature">
      <div class="site-card">
        <h3>The trail, and its verification</h3>
        <p>
          Auditable actions form a hash chain that stores no content. One call walks it and names
          the first broken link. The application’s own database role holds no update and no delete
          on that table, so an application defect cannot quietly rewrite history.
        </p>
        <Proof href="{docs}/architecture/audit.md" label="audit.md" />
      </div>
      <div class="site-card">
        <h3>Data subject rights</h3>
        <p>
          Access, export, rectification and erasure are use cases with the statutory deadline
          tracked by the system. Every personal field is in a catalogue that names where it lives
          and what deletes it — including the copies in backups and in the trail.
        </p>
        <Proof href="{docs}/architecture/data-protection.md" label="data-protection.md" />
      </div>
      <div class="site-card">
        <h3>Retention and legal hold</h3>
        <p>
          Rules that announce what they would delete, keep a grace period and can be stopped — and a
          legal hold that overrides both them and somebody emptying their own trash.
        </p>
        <Proof href="{docs}/architecture/data-retention.md" label="data-retention.md" />
      </div>
      <div class="site-card">
        <h3>Accessibility</h3>
        <p>
          The clients are built to WCAG 2.2 AA with a published statement — the standard the
          European Accessibility Act expects, demonstrated rather than asserted.
        </p>
        <a href="/accessibility/">The statement</a>
      </div>
    </div>
  </div>
</section>

<section class="section" id="builder">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">04 · The builder</p>
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
          and errors that carry a code and parameters rather than a sentence to parse.
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
