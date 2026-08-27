<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import '@hubtask/design-system/fonts.css';
  import '@hubtask/design-system/tokens.css';
  import '../site.css';

  let { children } = $props();
</script>

{@render children()}

<!-- The legal pages are German because the site is German-operated (§ 5 DDG); their link labels
     stay German everywhere, because "Impressum" is the word a German visitor scans for. -->
<footer>
  <nav lang="de">
    <a href="/impressum/">Impressum</a>
    <span aria-hidden="true">·</span>
    <a href="/datenschutz/">Datenschutz</a>
  </nav>
  <p>© 2026 Jérôme Bastian Winkel · Business Source License 1.1</p>
</footer>
