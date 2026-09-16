# n8n-nodes-hubtask

The n8n community node for [Hubtask](https://hubtask.eu), generated from the OpenAPI contract
(P-04, [ADR-0058](../../docs/adr/ADR-0058-connector-packages.md)).

Two nodes and one credential:

* **Hubtask** — every operation of the contract, one resource per tag, declarative: the
  description in `dist/nodes/Hubtask/Hubtask.node.json` is generated whole from
  `api/openapi.yaml`, and there is no code per operation.
* **Hubtask Trigger** — a webhook subscription over the REST hooks pattern: activating a workflow
  creates the subscription, deactivating it deletes it, and every delivery's signature is
  verified against the secret the subscription answered once. A delivery is read from the
  request's raw bytes: it arrives as `application/cloudevents+json`, which n8n's own body parser
  leaves unread, and the signature is over those bytes.
* **Hubtask API** — the installation's API root and a personal access token.

`pnpm build` generates `dist/`, which is the package as it would be published: its own
`package.json` names `n8n-workflow` as the peer dependency n8n itself supplies, and the
workspace's manifest names none, so this repository's lockfile carries no platform library.
`pnpm typecheck` validates the generated description against a schema of n8n's declarative
format; `pnpm test` proves every operation and every event type of the contract is reachable.

**Not published.** Publication to the community node registry is an account and a review the
owner runs; until then the package is `private`.
