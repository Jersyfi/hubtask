<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's fixture for QrCode: a provisioning URI of the shape the server writes, with
  // a secret that is nobody's, and the shortest and the longest input the encoder takes.

  import QrCode from './QrCode.svelte';
  import Stack from './Stack.svelte';
  import { encode } from './qr.ts';

  interface Props {
    mode?: 'provisioning' | 'smallest' | 'largest';
  }

  const { mode = 'provisioning' }: Props = $props();

  const text = $derived(
    mode === 'smallest'
      ? 'A'
      : mode === 'largest'
        ? 'otpauth://totp/'.padEnd(331, 'x')
        : 'otpauth://totp/Hubtask:ada%40example.org?algorithm=SHA1&digits=6&issuer=Hubtask&period=30&secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP',
  );
  const matrix = $derived(encode(text));
</script>

<Stack gap="150">
  <QrCode {matrix} label="A code an authenticator scans to enrol" />
  <p class="note">Version {matrix.version}, {matrix.size} × {matrix.size} modules, mask {matrix.mask}.</p>
</Stack>

<style>
  .note { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
</style>
