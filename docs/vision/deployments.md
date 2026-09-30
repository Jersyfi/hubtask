# Deployments — the seven shapes Hubtask is run in

The same image runs for one person and for a platform with thousands of customers. What differs
is **who uses it, who runs it, and how many of them are the same person**. A use case names the
deployments it must work in (`deployments:`); a use case that works in `D6` and confuses `D1` has
failed.

The shapes are ordered by size. [P-10](./principles.md#p-10-the-smallest-deployment-comes-first)
says the smallest comes first.

| ID | Shape | Tenancy | Uses it | Runs the workspace | Runs the installation | Same human? |
|---|---|---|---|---|---|---|
| `D1` | **A private person** | single | one person, several devices | the person | the person | all three |
| `D2` | **A family or household** | single | 2–8 people, some of them children or not technical | a parent | a parent (or a technical relative) | usually |
| `D3` | **A club, a freelancer team, a small office** | single | 5–30 people | one or two active members | one of them, or a volunteer | often |
| `D4` | **A company running it itself** | single or multi | 20–500 employees, external guests | IT administrator | a platform team | no |
| `D5` | **A provider for consumers (B2C)** | multi | thousands of individuals, each in their own workspace | each customer, for themselves | the provider | no |
| `D6` | **A provider for companies (B2B)** | multi | customer companies, each one a workspace | each customer's own administrator | the provider | no |
| `D7` | **A managed service provider** | several installations | several customers | the customer or the MSP | the MSP, reproducibly, per customer | no |

## What each shape needs to be true

### D1 — a private person

* `docker compose up`, open the browser, create the workspace and the first account there — no
  database shell, no token pasted from a log into a terminal.
* Every feature, no limits, nothing phones home.
* Never has to learn the words *installation*, *operator*, *tenant* or *plan*. Where the product
  has an installation level, it appears as one more section of the administration, labelled for
  what it does ("applies to every workspace you create later").
* Forgetting the password without a mail server is survivable: recovery codes and a documented
  local recovery.

### D2 — a family

* Shared hubs (groceries, holidays) and **private** hubs that another adult cannot open just
  because they are an administrator too.
* Children limited to their own hub, with a view simple enough for them.
* People without their own email address — a child, a grandparent — can still have an account,
  and an invitation can be handed over as a link or a code when the household has no mail server.
* The technical parent sets the rules once; the others never see a settings screen.

### D3 — a club or small team

* Invitations, roles, groups, automatic assignment, reminders by mail.
* A second factor required of the people who can change the structure.
* Optional sign-in with Google or Microsoft for members who prefer it — without giving a stranger
  with a Google account a way in.

### D4 — a company running it itself

* Sign-in through the company's own directory (Entra ID, Google Workspace, Keycloak), and people
  who arrive that way land with a role, not in an empty product.
* Password and session rules that meet the company's policy (length, history, expiry, idle
  timeout), set once.
* An auditor who can verify the trail and read configuration without seeing content.
* Backups to the company's own target; data subject requests answered in the product.

### D5 — a provider for consumers

* A purchase or sign-up platform creates the workspace through the API, with a service account —
  not with a token belonging to an employee.
* The provider's imprint, privacy notice and terms apply to everybody and cannot be replaced by a
  customer; accepting the terms is recorded.
* Sign-in with public providers (Google, Microsoft, Apple) that admit only people who were invited
  or who bought.
* Plans with limits and features; a downgrade stops, never deletes.
* A provided AI model the customer can use without configuring anything, counted against a budget.
* Operators see numbers and states, never a customer's content.

### D6 — a provider for companies

* Each customer company configures as much as possible itself: its own directory, its own legal
  texts, its own rules — within the locks the provider sets.
* The provider may offer its own sign-in providers and AI models to every customer, and may let a
  customer bring its own.
* Later: the customer's own domain.

### D7 — a managed service provider

* Every installation reproducible from a file (`HUBTASK_INSTANCE_FILE`) and the environment,
  including who its operators are.
* Nothing that needs a person at a browser to bring an installation up.

## A note on the licence

`D5`–`D7`, offered to third parties, are the provider case the licence reserves until Licensing
Start ([ADR-0059](../adr/ADR-0059-licensing-phases-and-licensing-start.md)). The product is built
for them now; offering it commercially waits for that date. A use case may name `D5`–`D7` without
contradicting the licence — it describes what the software can do, not who may sell it.
