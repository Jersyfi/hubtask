<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Button from './Button.svelte';
  import Inline from './Inline.svelte';
  import Kbd from './Kbd.svelte';
  import Stack from './Stack.svelte';
</script>

<Stack gap="200">
  <Inline gap="050" align="center"><Kbd>⌘K</Kbd> or <Kbd>Ctrl K</Kbd> or <Kbd>/</Kbd> <Kbd>Alt Shift ↑</Kbd></Inline>
  <Inline gap="050" align="center"><Kbd size="sm">↑</Kbd><Kbd size="sm">↓</Kbd> choose · <Kbd size="sm">⏎</Kbd> take · <Kbd size="sm">Esc</Kbd> close</Inline>
  <div><Button tone="primary" size="sm" icon="plus">Capture <Kbd size="sm" isOnFill>⏎</Kbd></Button></div>
</Stack>
