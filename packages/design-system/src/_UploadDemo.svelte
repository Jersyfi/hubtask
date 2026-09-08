<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The caller's half of the three-step upload, faked: a file is chosen, a progress number moves,
  // and nothing leaves the browser. The component moves no bytes in production either — this demo
  // is doing what a real caller does, which is why it can be a demo at all.

  import UploadField from './UploadField.svelte';

  const { mode = 'idle' }: { mode?: 'idle' | 'chosen' | 'uploading' | 'over' | 'unavailable' } =
    $props();

  const chosen = { name: 'kitchen-plan-revision-3.pdf', size: 2_411_008 };
  const huge = { name: 'site-survey-raw.zip', size: 268_435_456 };

  let file = $state<{ name: string; size: number } | null>(null);

  $effect(() => {
    file = mode === 'chosen' || mode === 'uploading' ? chosen : mode === 'over' ? huge : null;
  });
</script>

<UploadField
  label="Attachment"
  hint="PDF, PNG or JPEG, up to 25 MB."
  chooseLabel="Choose a file"
  dropLabel="or drop one here"
  cancelLabel="Cancel the upload"
  accept=".pdf,.png,.jpg,.jpeg"
  {file}
  sizeLabel={file ? (mode === 'over' ? '256 MB of 25 MB' : '2.4 MB of 25 MB') : undefined}
  overLimitLabel={mode === 'over'
    ? 'This file is larger than this installation accepts. Choose one under 25 MB.'
    : undefined}
  progress={mode === 'uploading' ? 40 : undefined}
  progressLabel={mode === 'uploading' ? '40% uploaded' : undefined}
  disabledReason={mode === 'unavailable'
    ? 'This installation stores no attachments, so nothing can be uploaded here.'
    : undefined}
  onChoose={(next) => {
    file = { name: next.name, size: next.size };
  }}
/>
