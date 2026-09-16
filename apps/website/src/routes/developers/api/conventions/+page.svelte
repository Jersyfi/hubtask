<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What every operation shares, written once. The facts here are the contract's own - the
  // security scheme, the problem document, the two header parameters - rendered from the
  // document where the document carries them, and stated in prose where it only implies them
  // (api-guidelines.md is the authority for that prose).
  import { Callout, CodeBlock, ParameterTable } from '@hubtask/design-system/components';

  import { reference } from '$lib/api/document.ts';

  const docs = 'https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/api-guidelines.md';
  const headings = { name: 'Name', type: 'Type', required: 'Required', description: 'Description' };
  const words = { required: 'required', optional: 'optional', deprecated: 'deprecated' };

  const problem = reference.schemas.find((schema) => schema.name === 'Problem');

  const problemExample = `{
  "type": "about:blank",
  "title": "Unprocessable Entity",
  "status": 422,
  "code": "validation_failed",
  "detail_code": "items.title_too_long",
  "params": { "max": 500 },
  "field_errors": [
    { "path": "title", "code": "items.title_too_long", "params": { "max": 500 } }
  ],
  "request_id": "018f6e2a-2b7c-7c1e-9b1a-3d5e6f7a8b9c"
}`;

  const pageExample = `GET ${reference.baseUrl}/items?collection_id=…&limit=50
→ { "items": [ … ], "next_cursor": "eyJ…", "has_more": true }

GET ${reference.baseUrl}/items?collection_id=…&limit=50&cursor=eyJ…`;
</script>

<svelte:head>
  <title>Conventions — API reference</title>
  <meta name="description" content="What every Hubtask API request shares: bearer authentication, problem documents with stable codes, idempotency keys, optimistic concurrency with ETags, cursor pagination and action suffixes." />
</svelte:head>

<section class="hero hero-sub">
  <div class="wrap">
    <p class="kicker"><a href="/developers/api/">API reference</a> · conventions</p>
    <h1>What every request shares</h1>
    <p class="lede">
      Six rules, and every operation page assumes them. The long form is
      <a href={docs}>api-guidelines.md</a>; this is the short one, with the shapes the contract declares.
    </p>
  </div>
</section>

<section class="section">
  <div class="wrap">
    <div class="site-api-operations">
      <section class="site-api-schema" id="authentication">
        <h2><a href="#authentication">Authentication</a></h2>
        <p>
          One scheme, <code>bearerAuth</code>: an <code>Authorization: Bearer</code> header carrying an OIDC access
          token, a personal access token (<code>hbt_pat_…</code>) or a service account token. Which of the three it is
          makes no difference to an operation — a token is refused precisely what the person or account behind it would
          be refused. The few routes without it say so on their own page.
        </p>
        <CodeBlock label="A bearer" language="http" code={`GET ${reference.baseUrl}/accounts/me\nAuthorization: Bearer hbt_pat_…`} />
      </section>

      <section class="site-api-schema" id="problems">
        <h2><a href="#problems">A refusal is a problem document</a></h2>
        <p>
          Every error is RFC 9457 with a stable <code>code</code> that is part of the contract, a
          <code>detail_code</code> that names a message in the catalogue, the parameters that message takes, and one
          entry per field where a field was wrong. There is no English in it: the client owns every sentence.
        </p>
        {#if problem}
          <ParameterTable label="Problem" isLabelHidden {headings} {words} rows={problem.rows} />
        {/if}
        <CodeBlock label="A 422" language="json" code={problemExample} />
      </section>

      <section class="site-api-schema" id="idempotency">
        <h2><a href="#idempotency">Idempotency</a></h2>
        <p>
          Every mutation takes an <code>Idempotency-Key</code>, a UUID the client chooses. The same request under the
          same key takes effect once and is answered from the stored response for 24 hours — which is what makes a
          retry after a lost connection safe. Two answers are deliberately not kept: a <code>5xx</code>, and a refusal
          that asks for a step-up proof, because neither is an outcome of the request.
        </p>
        <Callout tone="warning" title="Send one on every write">
          <p>A client that retries without a key can create twice. The header is optional in the contract so that a one-off script works; a client is expected to send it.</p>
        </Callout>
      </section>

      <section class="site-api-schema" id="concurrency">
        <h2><a href="#concurrency">Concurrency</a></h2>
        <p>
          Every resource answers an <code>ETag</code> and carries a <code>version</code>. A partial write sends the
          ETag it last read as <code>If-Match</code>; a write against a state somebody else has moved on is refused
          with <code>412</code> and <code>version_conflict</code>, and the client reads again rather than overwriting
          what it never saw. Partial writes are <code>application/merge-patch+json</code>: an absent key changes
          nothing, <code>null</code> clears.
        </p>
      </section>

      <section class="site-api-schema" id="pagination">
        <h2><a href="#pagination">Pagination is a cursor</a></h2>
        <p>
          A listing answers a page and a <code>next_cursor</code>; the next request sends it back. There are no page
          numbers anywhere, and a cursor is opaque and signed — a client can neither construct one nor read one. A
          listing whose reach is checked per record can answer a short page that is not the last: walk on until
          <code>has_more</code> is false.
        </p>
        <CodeBlock label="Two pages" language="http" code={pageExample} />
      </section>

      <section class="site-api-schema" id="actions">
        <h2><a href="#actions">Actions are a suffix</a></h2>
        <p>
          Something that is neither a read nor a field change is a <code>POST</code> to the resource with a colon
          suffix: <code>/items/&lbrace;itemId&rbrace;:complete</code>, <code>/backups/&lbrace;backupId&rbrace;:verify</code>. An action that
          cannot be bounded answers <code>202</code> and a job, and <code>/jobs/&lbrace;jobId&rbrace;</code> is where its
          progress and its result are read.
        </p>
        <Callout title="Tolerate what you do not know">
          <p>A field this reference does not list may appear in a response from a newer installation. A client must ignore it rather than fail — that is a binding requirement on every first-party client, and the reason a server upgrade does not break an older one.</p>
        </Callout>
      </section>
    </div>
  </div>
</section>
