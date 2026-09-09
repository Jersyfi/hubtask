<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>Run it yourself — self-hosting Hubtask</title>
  <meta
    name="description"
    content="One container image and PostgreSQL. Compose for one box, Helm for a fleet, backups to a target you choose, service level objectives with alerts and runbooks, and a health endpoint that says what is missing."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Operations</p>
    <h1>One image. One database. Yours.</h1>
    <p class="lede">
      Self-hosting is the default here, not a stripped-down edition of something else. The same
      image runs a single box and a multi-workspace platform, with the full feature set on both.
    </p>
    <pre class="site-shell"><code><span class="c"># The whole thing, on one machine</span>
git clone https://github.com/Jersyfi/hubtask.git &amp;&amp; cd hubtask
cp deploy/docker/.env.example .env      <span class="c"># set the secrets</span>
docker compose -f deploy/docker/compose.yaml up -d

<span class="c"># API:  http://localhost:8080/api/v1</span>
<span class="c"># Meta: http://localhost:8080/api/v1/meta/capabilities</span></code></pre>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">What it needs</p>
      <h2>PostgreSQL, and that is the list</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          There is no Redis to run, no message broker to keep alive, no search cluster to size.
          PostgreSQL is the store, the job queue, the event outbox, the full-text index and the
          tenant boundary — one dependency to back up, patch and understand.
        </p>
        <p>
          Object storage is optional: attachments go to an S3-compatible bucket if you have one, and
          to a volume if you do not. The vector extension is optional too — with it, search gains a
          semantic pass; without it, search is lexical and the installation says so through the
          capability endpoint rather than failing.
        </p>
        <Proof href="{docs}/architecture/deployment.md" label="deployment.md" />
      </div>
      <div>
        <h4>Supported, and proven by a job</h4>
        <div class="table-scroll">
          <table>
            <thead><tr><th scope="col">Runtime</th><th scope="col">Status</th></tr></thead>
            <tbody>
              <tr><th scope="row">Docker Compose, amd64 and arm64</th><td><span class="yes">supported</span></td></tr>
              <tr><th scope="row">Podman Compose</th><td><span class="yes">supported</span></td></tr>
              <tr><th scope="row">Kubernetes 1.28+, amd64</th><td><span class="yes">supported</span></td></tr>
              <tr><th scope="row">Kubernetes, arm64</th><td><span class="part">best effort</span></td></tr>
              <tr><th scope="row">PostgreSQL 16 and 17</th><td><span class="yes">supported</span></td></tr>
              <tr><th scope="row">PostgreSQL 15 and older</th><td><span class="no">unsupported</span></td></tr>
              <tr><th scope="row">A loose binary, no container</th><td><span class="no">unsupported</span></td></tr>
            </tbody>
          </table>
        </div>
        <p><small>
          “Supported” here means a job in the pipeline runs the software on it and a defect there
          blocks a release. “Best effort” means it is expected to work and nobody has automated the
          proof — which is a different promise, and it is written down as one.
        </small></p>
        <Proof href="{docs}/architecture/support-matrix.md" label="support-matrix.md" />
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Backups</p>
      <h2>Your target, your schedule, and a restore that has been rehearsed</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          Archives are encrypted and go where you tell them: a local volume, an S3-compatible bucket
          — AWS, MinIO, Ceph, Wasabi, Backblaze, Hetzner and the rest, the endpoint is yours to name
          — SFTP, or WebDAV including Nextcloud. Schedules are RRULEs, retention is generational,
          and the archive at the target can be listed without downloading it.
        </p>
        <p>
          <strong>Restore is a first-class operation</strong>, down to a single item, including a
          journal of what a destructive restore removed. And the drill runs in the pipeline on every
          change, because the only backup worth having is one somebody has put back.
        </p>
        <Proof href="{docs}/architecture/backup-restore.md" label="backup-restore.md" />
      </div>
      <div>
        <h4>Two details that matter at three in the morning</h4>
        <ul class="ticks">
          <li>
            <strong>No trust on first use over SFTP</strong>
            A target names the host’s public key or its fingerprint, or it is refused. There is no
            way to switch that off — a first connection that accepted whatever answered only has to
            be intercepted once.
          </li>
          <li>
            <strong>A local target cannot leave its volume</strong>
            The path is relative to the backup volume. Whoever configures a target is administering
            the instance, not the machine it sits on.
          </li>
          <li>
            <strong>An unbuilt target type is refused by name</strong>
            “Hubtask cannot talk to SMB yet” rather than a generic error — a further adapter ships
            when it passes the same conformance suite.
          </li>
        </ul>
      </div>
    </div>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Running it</p>
      <h2>It tells you what is wrong before you have to guess</h2>
    </div>
    <div class="site-cards">
      <div class="site-card">
        <h3>Four health levels</h3>
        <p>
          Liveness, startup and readiness for the orchestrator, and a meta endpoint for a person —
          which names what is degraded and what is missing instead of the process falling over.
        </p>
      </div>
      <div class="site-card">
        <h3>Degrade, do not crash</h3>
        <p>
          A missing optional dependency switches a feature off and says so. Timeouts, circuit
          breakers, bulkheads and load shedding are in the baseline rather than added after the
          first outage.
        </p>
      </div>
      <div class="site-card">
        <h3>Metrics, traces, logs</h3>
        <p>
          OpenTelemetry throughout, traces that survive the outbox and the job queue, structured
          logs with your content redacted, and a dashboard in the repository.
        </p>
      </div>
      <div class="site-card">
        <h3>Objectives and alerts</h3>
        <p>
          Eight service level objectives — availability, read and write latency, event delivery,
          reminder punctuality, webhook delivery, automation, and confirmed writes lost, which is
          zero — with an alert catalogue and a runbook per alert.
        </p>
      </div>
      <div class="site-card">
        <h3>Process roles</h3>
        <p>
          One image, several roles: API, worker, scheduler and automation. Run them together on one
          box or separately in a cluster with autoscaling and a disruption budget.
        </p>
      </div>
      <div class="site-card">
        <h3>Upgrades that roll</h3>
        <p>
          Forward-only migrations designed to expand before they contract, so a new version and the
          previous one can run at the same time while the rollout finishes.
        </p>
      </div>
    </div>
    <Proof href="{docs}/architecture/observability-reliability.md" label="observability-reliability.md" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">A fleet</p>
      <h2>The same image, at platform scale</h2>
    </div>
    <div class="two-up">
      <div>
        <p>
          The Helm chart separates the roles, scales the API horizontally, and ships a disruption
          budget and a network policy. Multi-workspace mode adds provisioning, suspension, deletion
          and export per workspace, quotas that keep one workspace from becoming everybody’s
          latency, and point-in-time recovery with a verified restore.
        </p>
        <p>
          Running it for other people commercially needs a paid licence.
          <a href="/licence/">The terms, and what is still open</a>
        </p>
      </div>
      <div>
        <pre class="site-shell"><code>helm upgrade --install hubtask ./k8s \
  -f ./k8s/values.yaml

<span class="c"># Roles as separate deployments</span>
<span class="c">#   api · worker · scheduler · automation</span></code></pre>
      </div>
    </div>
  </div>
</section>
