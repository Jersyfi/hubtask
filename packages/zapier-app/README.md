# The Zapier app

The Zapier app for [Hubtask](https://hubtask.eu), generated from the OpenAPI contract (P-05,
[ADR-0058](../../docs/adr/ADR-0058-connector-packages.md)).

* **Triggers** — one per event type the installation publishes, as REST hooks: activating a Zap
  creates a webhook subscription, deactivating it deletes it, and the sample comes from trigger
  polling.
* **Creates** — create an entry, update one, complete one, add a comment, create a hub or a
  collection; each input is a field of the contract's own body, and every create sends an
  `Idempotency-Key` so that a Zapier retry never creates twice.
* **Searches** — find entries by text (`POST /search`) and by filter (`POST /items:query`).
* **Authentication** — OAuth2 authorization code with PKCE against the installation's own
  provider; the person connecting names their installation's address, and the app's client is
  registered there with `POST /oauth/clients`.

`pnpm build` generates `dist/`, which is the app as it would be pushed with the Zapier CLI: its
own `package.json` names `zapier-platform-core` as the CLI requires, and the workspace's
manifest names none, so this repository's lockfile carries no platform library.
`pnpm typecheck` generates into a temporary directory, loads the app and validates it against a
schema of the platform's definition format; `pnpm test` proves every event type is a trigger,
every create and search exists with a sample, and the request helper does what the contract
expects.

**Not published.** The marketplace is an account, a client registration and a review the owner
runs; until then the package is `private`.
