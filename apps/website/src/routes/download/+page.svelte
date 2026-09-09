<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const repo = 'https://github.com/Jersyfi/hubtask';
</script>

<svelte:head>
  <title>Get Hubtask — server, clients and the CLI</title>
  <meta
    name="description"
    content="The container image for the server, signed installers for Windows, macOS and Linux, the mobile apps, and hubctl for every platform. Nothing to sign up for."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Get it</p>
    <h1>Nothing to sign up for</h1>
    <p class="lede">
      No account, no trial, no sales call. Pull the image, run it next to a PostgreSQL, and read the
      architecture while it starts. Everything on this page is free to run for private use.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The server</p>
      <h2>One image, both ways of running it</h2>
    </div>
    <div class="two-up">
      <div>
        <pre class="shell"><code><span class="c"># One box</span>
git clone https://github.com/Jersyfi/hubtask.git
cd hubtask
cp deploy/docker/.env.example .env
docker compose -f deploy/docker/compose.yaml up -d

<span class="c"># A cluster</span>
helm upgrade --install hubtask ./k8s \
  -f ./k8s/values.yaml</code></pre>
      </div>
      <div>
        <h4>What comes with it</h4>
        <ul class="ticks">
          <li><strong>Multi-architecture</strong> amd64 and arm64 from the same tag, so a small ARM box is not a second-class citizen.</li>
          <li><strong>Signed, with an SBOM</strong> Every release publishes a signature and a software bill of materials.</li>
          <li><strong>The web interface included</strong> It ships inside the binary; there is no second thing to deploy.</li>
          <li><strong>The migrator included</strong> Schema changes are applied by a job in the same image, forward only.</li>
        </ul>
        <Proof href="{repo}/releases" label="releases · checksums and signatures" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">The clients</p>
      <h2>Where each one runs</h2>
      <p class="lede">
        The web interface is part of the server. The desktop and mobile applications are shells
        around the same interface, adding local storage, the system keychain and an updater — which
        is what makes offline work real rather than a cache.
      </p>
    </div>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th scope="col">Client</th>
            <th scope="col">Platforms</th>
            <th scope="col">Notes</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <th scope="row">Web</th>
            <td>Any current browser</td>
            <td>Served by the instance itself. Chromium, Firefox and Safari, current and previous major. A best-effort cache offline, and it says so rather than pretending.</td>
          </tr>
          <tr>
            <th scope="row">Desktop</th>
            <td>Windows, macOS, Linux</td>
            <td>Signed installers with an updater. Encrypted local storage, full offline operation, and administration available like on the web.</td>
          </tr>
          <tr>
            <th scope="row">Mobile</th>
            <td>iOS, Android</td>
            <td>The same shell. Workspace administration is deliberately not here — where a screen is missing, the app names the capability and links to the web interface of the server you are signed into.</td>
          </tr>
          <tr>
            <th scope="row"><code>hubctl</code></th>
            <td>Linux, macOS, Windows</td>
            <td>amd64 and arm64. Apple silicon and Windows on ARM built and published; the whole product from a terminal, and a pipe-friendly <code>--json</code>.</td>
          </tr>
        </tbody>
      </table>
    </div>
    <Proof href="{repo}/blob/main/docs/architecture/support-matrix.md" label="support-matrix.md · what is proven by a job" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">First five minutes</p>
      <h2>From nothing to a completed task</h2>
    </div>
    <pre class="shell"><code><span class="c"># Sign in with a personal access token</span>
echo "$TOKEN" | hubctl auth login --url https://tasks.example.org

<span class="c"># A hub, a collection, a task</span>
HUB=$(hubctl container create --type HUB --name "Personal" | awk 'NR==2 &lbrace;print $1&rbrace;')
COL=$(hubctl container create --type COLLECTION --parent "$HUB" --name "Errands" | awk 'NR==2 &lbrace;print $1&rbrace;')
ITEM=$(hubctl item create --collection "$COL" --type TASK --title "Buy milk" | awk 'NR==2 &lbrace;print $1&rbrace;')

<span class="c"># Make it real</span>
hubctl due set "$ITEM" --at 2027-01-10
hubctl remind add "$ITEM" --at -PT30M
hubctl item complete "$ITEM"</code></pre>
    <p class="cta-row">
      <a class="cta" href="https://github.com/Jersyfi/hubtask">Source and releases</a>
      <a class="cta-quiet" href="/self-hosting/">The operations page</a>
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="callout">
      <p>
        <strong>Running it for a business?</strong> That is the case the licence reserves. What a
        commercial licence costs is being settled before 1.0, and it will be published before
        anybody is asked to pay. <a href="/licence/">The terms as they stand</a>
      </p>
    </div>
  </div>
</section>
