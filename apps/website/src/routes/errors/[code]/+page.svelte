<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  const { data } = $props();
  const entry = $derived(data.entry);
</script>

<svelte:head>
  <title>{entry.code} — Hubtask errors</title>
  <meta name="description" content="{entry.status} {entry.code}: {entry.meaning}" />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker"><a href="/errors/">Errors</a> · HTTP {entry.status}</p>
    <h1><code>{entry.code}</code> — {entry.title}</h1>
    <p class="lede">{entry.meaning}</p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="narrow">
      <h2>What to do</h2>
      <p>{entry.action}</p>
      <p>
        {#if entry.retry}
          Repeating the same request can succeed once the cause has passed. Send it with the same
          <code>Idempotency-Key</code>, so that it takes effect once.
        {:else}
          Repeating the same request unchanged gives the same answer.
        {/if}
      </p>
      <h2>Reading the answer</h2>
      <p>
        <code>detail_code</code> names the concrete case and <code>params</code> the values its message takes;
        both are listed in the <a href="https://github.com/Jersyfi/hubtask/blob/main/locales/en.json">message
        catalogue</a>. Quote <code>request_id</code> when you ask for help: it is the one handle that connects the
        answer to the server's log and to the workspace's audit trail. See also
        <a href="/developers/api/conventions/#problems">the problem document</a>.
      </p>
    </div>
  </div>
</section>
