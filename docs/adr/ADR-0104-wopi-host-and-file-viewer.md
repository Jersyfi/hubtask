# ADR-0104 — A WOPI host for a document server, and one file viewer

**Status:** accepted, the launch open · **Date:** 2026-11-14 · **Decided:** 2026-11-12, by the
owner (the concept [work-surface](../backlog/work-surface/README.md) E-14, F-1…F-5)

**Rule lives in:** [security.md](../architecture/security.md) §8 (the WOPI access token), §9 (the
viewer's policy, where a target is minted), §10 (the document server through GuardedClient);
the contract's `/media:document-server` (the workspace's document server, set under STRUCTURE);
[design-system.md](../design/design-system.md) (the stage tokens).

## Context

A file could only be downloaded unless it was a picture. People want to read a PDF, a table or a
contract where it is attached, and to change an office file without downloading and uploading it
again. Hubtask edits no file itself, and must not become a renderer of what somebody uploaded.

## Decision

* **One viewer** in the web app for every file, wherever files are shown (entry, page, board,
  post, inventory, portal). Pictures, audio and video decode in the browser from the download
  target; PDF (pdf.js, `isEvalSupported: false`), text, Markdown, CSV and a ZIP's listing are
  fetched as bytes and drawn by Hubtask; nothing is opened as a document.
* **A local download target is minted at the host the request came on**, so a workspace's own
  origin fetches it under `'self'`; no wider policy and no CORS.
* **A WOPI host** answers a document server chosen per workspace (none, Collabora Online,
  ONLYOFFICE, Euro-Office, optionally the largest vendor's browser office): CheckFileInfo,
  GetFile, PutFile and the locks. The access token is signed for one person, one file, read or
  write, two hours, never stored or logged. Every call asks again, in the application layer,
  whether the person reads the file and may change an entry or a board holding it.
* **A save is a new version**: the file keeps its identity, and the bytes it had become an
  object of their own, named by `media_version` and kept while the file lives.
* **Without a document server** an office file is a card; with one it is converted to PDF by the
  server's own conversion, through GuardedClient, for the viewer.

## Open: the launch

Opening the editor needs the browser to hand the token to the server's action URL. Either the
web interface's policy admits the configured server's origin in `frame-src` and `form-action`
(a form POST into a frame), or the token travels in a URL the server opens in a new tab. Both
touch the security policy; the owner decides. Until then *Bearbeiten* stays hidden and
`POST /media/{id}:edit` is the only door to a session.

## Alternatives

* *A converter on the server* (LibreOffice headless). A new dependency in the image; asked of the
  owner separately.
* *Storing the tokens.* Revocable, but a write per open and a secret table; a two-hour signed
  token bounded to one file was preferred.
* *A new object per save, the references moved.* Every entry, page and board would have to follow
  the new id; keeping the id and moving the old bytes aside changes nothing that references it.
