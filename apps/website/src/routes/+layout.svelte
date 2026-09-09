<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import { page } from '$app/state';
  import '@hubtask/design-system/fonts.css';
  import '@hubtask/design-system/tokens.css';
  import '../site.css';
  import { Icon, VisuallyHidden } from '@hubtask/design-system/components';
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

    <!-- Three radios and no script. `Auto` is checked, so the control always states what the page
         is doing; src/site.css turns the choice into a theme. The state is per document, because
         remembering it across a navigation would need storage, and storage would need a script
         this site does not load.

         Icons rather than words, and all three visible rather than one that cycles. A cycling
         button has a dead click here: from `Auto` on a light system the next state is `Light`,
         which changes nothing on screen, and the reader has to press again to reach the state
         they wanted. Reordering the cycle only moves the dead click to the dark system. Three
         targets have none, cost one press from any state to any other, and keep a real radio
         group for the keyboard and the accessibility tree. The movement the cycle would have
         carried is in the thumb that slides between them and in the glyph that turns into
         place. -->
    <div class="modes" role="radiogroup" aria-label="Colour mode">
      <span class="modes-thumb" aria-hidden="true"></span>
      <input type="radio" name="mode" id="mode-auto" checked />
      <label for="mode-auto">
        <span class="mode-glyph" data-glyph="auto"><Icon name="monitor" size="sm" /></span>
        <VisuallyHidden>Follow the system</VisuallyHidden>
      </label>
      <input type="radio" name="mode" id="mode-light" />
      <label for="mode-light">
        <span class="mode-glyph" data-glyph="light"><Icon name="sun" size="sm" /></span>
        <VisuallyHidden>Light</VisuallyHidden>
      </label>
      <input type="radio" name="mode" id="mode-dark" />
      <label for="mode-dark">
        <span class="mode-glyph" data-glyph="dark"><Icon name="moon" size="sm" /></span>
        <VisuallyHidden>Dark</VisuallyHidden>
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
