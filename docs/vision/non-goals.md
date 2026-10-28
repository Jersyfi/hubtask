# Non-goals

What Hubtask deliberately is not. A use case whose *Where it ends* section is thin is often
missing one of these; a session about to build one of them should stop and ask.

| ID | Hubtask does not … | Because | Instead |
|---|---|---|---|
| `NG-billing` | know prices, contracts, trials, invoices, coupons or dunning | [P-15](./principles.md#p-15-hubtask-holds-state-the-platform-holds-the-business): a self-hosted product that needs a business back-end is not self-hosted | A platform pushes state in (create, plan, suspend) and reads usage out |
| `NG-phone-home` | contact any service the operator did not configure — no telemetry, no licence check, no update ping, no fonts from a CDN | [P-09](./principles.md#p-09-yours-to-run) | Everything in the image; outbound calls only through the guarded client to targets the operator named |
| `NG-operator-reads-content` | let an operator of the installation read a workspace's tasks, comments or files | [P-01](./principles.md#p-01-workspaces-never-see-each-other) | Counts, states, limits, the health report, the instance journal |
| `NG-global-identity` | give one person one account across workspaces | The tenant boundary is the product; a person in two workspaces is two accounts | Each workspace at its own address; a workspace switcher may list the person's own accounts on one device |
| `NG-e2e-encryption` | encrypt content end to end | Search, automation, rules and AI need to read content on the server | Encryption at rest, sealed secrets, TLS |
| `NG-weaker-recovery` | reset a password around an active second factor, or let an administrator open somebody's account | [P-02](./principles.md#p-02-an-account-is-opened-only-by-its-own-strongest-proof) | Recovery codes; an administrator removes and re-invites. **One exception:** a *managed* account without a mail address gets a new start password from an administrator, because nothing else could recover it ([ADR-0074](../adr/ADR-0074-managed-accounts.md)) |
| `NG-magic-link` | sign people in by a mailbox link alone | Mailbox control is weaker than password plus second factor ([P-02](./principles.md#p-02-an-account-is-opened-only-by-its-own-strongest-proof)) | Reset links (which keep the second factor), passkeys (planned) |
| `NG-sms` | send codes by SMS | No carrier from a self-hosted product, and NIST rates it down | Authenticator apps, passkeys (planned) |
| `NG-security-questions` | ask for password hints or security questions | They are guessable and unrecoverable | Recovery codes |
| `NG-ai-required` | make any feature depend on AI | [P-14](./principles.md#p-14-ai-is-optional-and-nobody-consents-in-anothers-place) | AI suggests; people decide |
| `NG-ai-consent-by-default` | let a plan, an operator or a default consent to AI processing for a workspace | [P-14](./principles.md#p-14-ai-is-optional-and-nobody-consents-in-anothers-place) | The workspace consents itself; a provider may only *offer* a model |
| `NG-page-numbers` | show page numbers or a pager | The owner's product rule (2026-09-27): lists load on when the reader asks; no pager, no page numbers | Continuous lists, search, filters |
| `NG-regex-rules` | accept administrator-written regular expressions for password rules | A regex is a denial-of-service vector and its refusal has no sentence that names the fix | The eighteen named switches of [ADR-0068](../adr/ADR-0068-sign-in-policy-and-the-password-lifetime.md) |
| `NG-saml-before-1` | speak SAML or SCIM before 1.0 | OpenID Connect covers every directory named so far ([security.md](../architecture/security.md) §15) | OIDC with directory admission ([ADR-0071](../adr/ADR-0071-provider-admission.md)) |
| `NG-second-app` | ship a second frontend for operators or administrators | One frame, one catalogue, one bundle ([ADR-0070](../adr/ADR-0070-the-instance-layer.md) §5) | Route areas of the same web app |
| `NG-chat` | ship team chat, channels or direct messages | A chat product is its own discipline and every organisation already has one; a second one splits the conversation | Comments with threads and mentions on every entry and page; notifications into the organisation's chat ([UC-NOT-06](../usecases/notification/UC-NOT-06-be-told-somewhere-other-than-mail.md)) |
| `NG-meetings` | host video or audio calls | The same reason; and it is infrastructure Hubtask cannot run on a small box | A meeting-notes page template whose action items become tasks |
| `NG-invoicing` | write invoices, quotes or payment reminders | [P-15](./principles.md#p-15-hubtask-holds-state-the-platform-holds-the-business), `NG-billing` | Time and cost are recorded and exported (CSV, API) to the accounting system |
| `NG-in-process-plugins` | run third-party code inside the server process | Every guarantee — the tenant boundary, the trail, the gates — ends where foreign code runs in-process | Apps out of process: the API, webhooks, scoped credentials, declared UI extension points rendered by the web app |
| `NG-spreadsheet` | offer a formula grid or a general spreadsheet engine | It is a product of its own, and formulas over user content cannot be bounded like CEL conditions | Computed fields from a fixed list (sum, count, roll-up, date difference); export to a spreadsheet |
| `NG-crm` | become a sales CRM (pipelines with forecasting, deals, quotes) | Breadth without depth; the market is served | Records (types without completion) cover a contact list or an asset register; a CRM connects through the API |
