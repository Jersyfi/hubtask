<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { page } from '$app/state';
  import '@hubtask/design-system/fonts.css';
  import '@hubtask/design-system/tokens.css';
  import '../site.css';
  import { Icon } from '@hubtask/design-system/components';
  import Mark from '$lib/Mark.svelte';

  let { children } = $props();

  const nav = [
    { href: '/product/', label: 'Product' },
    { href: '/use-cases/', label: 'Use cases' },
    { href: '/security/', label: 'Security' },
    { href: '/developers/', label: 'Developers' },
    { href: '/self-hosting/', label: 'Self-hosting' },
    { href: '/licence/', label: 'Licence' },
  ];

  // `aria-current` is resolved at prerender time - there is no client-side router here, so each
  // document carries the answer for its own path rather than computing it in a browser.
  const current = $derived(page.url.pathname);
</script>

<a class="skip" href="#main">Skip to content</a>

<header class="masthead">
  <div class="wrap">
    <a class="brand" href="/">
      <Mark />
      Hubtask
    </a>

    <nav class="topnav" aria-label="Sections">
      {#each nav as item (item.href)}
        <a href={item.href} aria-current={current === item.href ? 'page' : undefined}>{item.label}</a>
      {/each}
    </nav>

    <!-- One glyph, showing the mode the page is in, and clicking it advances to the next:
         auto -> light -> dark -> auto. Three radios carry the state and no script is involved.

         The trick that makes a cycle possible in CSS is that the three faces are stacked and only
         the one matching the current state is visible - and *that* face's `for` names the **next**
         radio. So what is painted says where you are, and pressing it says where you go.

         The accessible name comes from `aria-label` on each input rather than from the label
         element, which points elsewhere on purpose. A screen reader therefore meets an ordinary
         three-option radio group and can go straight to any mode with the arrow keys, while the
         pointer gets the single glyph. Neither is told the other one's story.

         One press per three does not change the page colours - on a dark system `Auto` and `Dark`
         render the same, and no ordering of three states over two renderings avoids that. The
         glyph changing, with the movement below, is what makes that press legible anyway: the
         control answers even when the canvas does not. -->
    <div class="theme" role="radiogroup" aria-label="Colour mode">
      <input type="radio" name="mode" id="mode-auto" aria-label="Follow the system" checked />
      <input type="radio" name="mode" id="mode-light" aria-label="Light" />
      <input type="radio" name="mode" id="mode-dark" aria-label="Dark" />

      <label class="theme-face" data-face="auto" for="mode-light" title="Following the system — switch to light">
        <span class="theme-halo" aria-hidden="true"></span>
        <span class="theme-glyph"><Icon name="sun-moon" /></span>
      </label>
      <label class="theme-face" data-face="light" for="mode-dark" title="Light — switch to dark">
        <span class="theme-halo" aria-hidden="true"></span>
        <span class="theme-glyph"><Icon name="sun" /></span>
      </label>
      <label class="theme-face" data-face="dark" for="mode-auto" title="Dark — switch to following the system">
        <span class="theme-halo" aria-hidden="true"></span>
        <span class="theme-glyph"><Icon name="moon-star" /></span>
      </label>
    </div>
  </div>
</header>

<main id="main">
  {@render children()}
</main>

<footer class="colophon">
  <div class="wrap">
    <div class="colophon-grid">
      <div class="colophon-brand">
        <Mark />
        <p>
          Task management in five levels. Self-hostable, API first, agent ready — and built so the
          promises about your data can be checked rather than believed.
        </p>
      </div>

      <nav aria-label="Product">
        <h4>Product</h4>
        <a href="/product/">What it does</a>
        <a href="/use-cases/">Who it is for</a>
        <a href="/roadmap/">Roadmap</a>
        <a href="/download/">Download</a>
      </nav>

      <nav aria-label="Build">
        <h4>Build</h4>
        <a href="/developers/">API, MCP and CLI</a>
        <a href="/self-hosting/">Run it yourself</a>
        <a href="/security/">Security and privacy</a>
        <a href="/accessibility/">Accessibility</a>
      </nav>

      <nav aria-label="Project">
        <h4>Project</h4>
        <a href="https://github.com/Jersyfi/hubtask">Source on GitHub</a>
        <a href="https://github.com/Jersyfi/hubtask/tree/main/docs">Documentation</a>
        <a href="/licence/">Licence and editions</a>
        <a href="https://github.com/sponsors/Jersyfi">Sponsor the work</a>
      </nav>

      <!-- The legal pages are German because the site is German-operated (§ 5 DDG); their link
           labels stay German everywhere, because "Impressum" is the word a German visitor scans
           for. -->
      <nav aria-label="Legal" lang="de">
        <h4 lang="en">Legal</h4>
        <a href="/impressum/">Impressum</a>
        <a href="/datenschutz/">Datenschutz</a>
      </nav>
    </div>

    <div class="colophon-note">
      <p>
        © 2026 Jérôme Bastian Winkel · Business Source License 1.1, converting to Apache-2.0 three
        years after each version is published. Source available, not open source — and the
        difference is stated rather than blurred.
      </p>
      <nav aria-label="Contact">
        <a href="mailto:info@hubtask.eu">info@hubtask.eu</a>
      </nav>
    </div>
  </div>
</footer>
