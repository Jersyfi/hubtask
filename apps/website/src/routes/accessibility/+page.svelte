<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Proof from '$lib/Proof.svelte';
  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs';
</script>

<svelte:head>
  <title>Accessibility — how it is built, and what is measured</title>
  <meta
    name="description"
    content="Hubtask is built to WCAG 2.2 level AA: contrast measured on every test run, a visible focus ring, no control disabled without a reason, and every component checked against forty per cent longer text."
  />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker">Accessibility</p>
    <h1>Operable by keyboard, or it is a defect</h1>
    <p class="lede">
      Hubtask is built to WCAG 2.2 level AA — the standard the European Accessibility Act expects.
      This page says what that means in practice and what is actually measured rather than intended.
      The formal accessibility statement is published with the first stable release, once conformance
      has been demonstrated for a release rather than asserted for a plan.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">In practice</p>
      <h2>What was built rather than reviewed</h2>
    </div>
    <ul class="ticks ticks-2">
      <li>
        <strong>Contrast is measured, not eyeballed</strong>
        Every colour pair the design system declares is measured in both light and dark on every
        test run: 4.5:1 for text, 3:1 for a control boundary and the focus ring. A new colour with
        no role assigned fails the suite rather than being skipped.
      </li>
      <li>
        <strong>Focus is always visible</strong>
        A two-pixel ring with an offset, on every interactive element. The workbench can walk the
        focus order of a component and report it, so “it is keyboard operable” is something somebody
        looked at rather than assumed.
      </li>
      <li>
        <strong>Overlays behave</strong>
        Focus is trapped in a dialog and returned to what opened it; menus work with arrows, Home,
        End and type-ahead; Escape closes exactly one layer; a notification is announced without
        stealing focus, because a save confirmation arrives while somebody is typing.
      </li>
      <li>
        <strong>Nothing is disabled without a reason</strong>
        There is no bare “disabled” anywhere in the component library — switching a control off
        requires giving the reason, so a control you cannot use cannot come apart from why.
      </li>
      <li>
        <strong>Text can grow by forty per cent</strong>
        Every component is checked against a pseudo-locale that lengthens every string. Clipping is
        treated as a defect, not as a rendering.
      </li>
      <li>
        <strong>Reduced motion is a user preference</strong>
        Not only the operating system’s: the product carries its own switch, because a preference
        only the OS can set is not one the reader controls.
      </li>
      <li>
        <strong>Right to left is not an afterthought</strong>
        Layout uses start and end rather than left and right throughout, and every component is
        reviewed in both directions.
      </li>
      <li>
        <strong>Colour never carries meaning alone</strong>
        Every status also has text or an icon — which matters for colour vision deficiency, and
        equally for print and greyscale documentation.
      </li>
    </ul>
    <Proof href="{docs}/design/design-system.md" label="design-system.md §6 · the six rules" />
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="section-head">
      <p class="kicker">Scope and limits</p>
      <h2>What this statement covers, and what it does not</h2>
    </div>
    <div class="two-up">
      <div>
        <h3>What this covers</h3>
        <p>
          The web interface an installation serves, and this website. The installed desktop and
          mobile applications are being built and will be held to the same standard, with a screen
          reader pass and a keyboard pass part of the release rather than a follow-up.
        </p>
        <h3>This website</h3>
        <p>
          The page you are reading is plain HTML with no scripting at all, one skip link, a visible
          focus ring, and the same measured contrast. Its colour mode control is three radio buttons
          and works from the keyboard.
        </p>
      </div>
      <div>
        <h3>Known limitations</h3>
        <p>
          Content you enter yourself — an image attachment without a description, a colour used as
          the only distinction in a workspace’s own labels — is outside what the software can
          guarantee. Where the product can help it does: a label is always a colour and a name, never
          a colour alone.
        </p>
        <h3>Reporting a barrier</h3>
        <p>
          If something here or in the application cannot be reached, please say so. A barrier is a
          defect and is handled as one — and a report now, while the interface is being built, is
          worth more than one after it has shipped.
          <a href="mailto:info@hubtask.eu">info@hubtask.eu</a>
        </p>
      </div>
    </div>
  </div>
</section>
