# ADR-0104 — A WOPI host for a document server, and one file viewer

**Status:** accepted · **Date:** 2026-11-14 · **Decided:** 2026-11-12, by the owner (the
concept [work-surface](../backlog/work-surface/README.md) E-14, F-1…F-5); the launch, no
converter without a server and the private-network exception on 2026-11-14, by the owner

**Rule lives in:** [security.md](../architecture/security.md) §8 (the WOPI access token), §9 (the
viewer's policy, where a target is minted, the editor's frame), §10 (the document server through
GuardedClient, its one private-network exception);
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
  whether the person reads the file and may change an entry, a board or a page holding it (the
  page's writers are the knowledge module's answer, asked through a port).
* **A save is a new version**: the file keeps its identity, and the bytes it had become an
  object of their own, named by `media_version` and kept while the file lives.
* **Without a document server** an office file is a card with its type, its size and the
  download - Hubtask has no converter of its own; with one it is converted to PDF by the server's
  own conversion, through GuardedClient, for the viewer.
* **The editor launches embedded.** `POST /media/{id}:edit` answers the token and the editor's
  address from the server's discovery (only on its own origin); the viewer posts the token in a
  form into a frame. The interface's policy, computed per workspace from the stored choice when
  the document loads, adds exactly that origin to `frame-src` and `form-action` while one is
  entered, and nothing once it is removed. The token never travels in a URL.
* **The entered server may sit on a private network.** The outbound guard allows a private
  address for that one host only; a redirect, a rebind or any other private target stays refused.
  An installation switch turns the exception off (default on for self-hosting).

## Alternatives

* *A new tab with the token in its address.* No policy change, but a two-hour write token in a
  tab's history, the server's log and a Referer; not chosen.
* *The installation's policy listing every workspace's server.* One policy for all, but every
  workspace's document server could be framed by every other's pages; per workspace was chosen.
* *A guard open to private networks for the document server's client.* Simpler, but a redirect or
  a rebind from the server could reach anything on the network; per host was chosen.

## Further alternatives

* *A converter on the server* (LibreOffice headless). A new dependency in the image; asked of the
  owner separately.
* *Storing the tokens.* Revocable, but a write per open and a secret table; a two-hour signed
  token bounded to one file was preferred.
* *A new object per save, the references moved.* Every entry, page and board would have to follow
  the new id; keeping the id and moving the old bytes aside changes nothing that references it.
