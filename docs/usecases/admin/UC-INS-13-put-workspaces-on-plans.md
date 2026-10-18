---
id: UC-INS-13
title: Put workspaces on plans with their own limits, features and locks
context: admin
actors: [PE-operator, PE-platform, PE-owner]
deployments: [D5, D6, D7]
serves: [P-03, P-05, P-06, P-07, P-08, P-15]
state: verified
tasks: [PL-01, PL-02, PL-03, PL-04, PL-05, PL-06, PL-09, PL-11]
checked_by: [core/domain/model/plan/Plan_test.go, core/application/service/admin/Plans_test.go, core/application/service/admin/MovePlan_test.go, core/application/service/admin/PlanFile_test.go, test/integration/plan_test.go, test/integration/managed_doors_test.go, presentation/rest/PlanController_test.go, cmd/hubctl/AdminPlan_test.go, infrastructure/instancefile/File_test.go, core/application/service/admin/Reference_test.go, test/integration/tenant_reference_test.go, cmd/hubctl/AdminTenantFind_test.go, core/domain/model/identity/SignInPolicy_test.go, core/domain/model/identity/LegalLinks_test.go, core/application/service/identity/PlanLevel_test.go, test/integration/plan_resolution_test.go, apps/webapp/e2e/signinsettings.test.mjs, core/domain/model/entitlement/Entitlement_test.go, core/application/catalogue/Catalogue_test.go, core/application/service/admin/Entitlements_test.go, core/application/service/automation/PlanRests_test.go, core/application/service/integration/PlanRests_test.go, infrastructure/webhook/PlanRests_test.go, core/application/service/identity/OwnProviders_test.go, core/application/service/meta/NotIncluded_test.go, test/integration/entitlement_test.go, apps/webapp/e2e/entitlements.test.mjs, core/application/service/quota/Quota_test.go, test/integration/plan_limits_test.go, test/integration/plan_rate_test.go, presentation/rest/QuotaController_test.go, apps/webapp/e2e/quotas.test.mjs, docs/evidence/PL-03-2026-10-17.md, core/domain/model/integration/AiSource_test.go, core/application/service/integration/PlanAi_test.go, test/integration/plan_ai_test.go, core/domain/model/identity/AccountLink_test.go, core/domain/model/plan/AccountLink_test.go, core/application/service/admin/AccountLink_test.go, core/application/service/identity/PausedAccountLink_test.go, core/application/service/meta/AccountLink_test.go, test/integration/account_link_test.go, apps/webapp/e2e/accountlink.test.mjs, apps/webapp/e2e/paused.test.mjs, apps/webapp/e2e/instancesettings.test.mjs, core/application/service/identity/PeopleLimit_test.go, test/integration/people_limit_test.go, cmd/server/PeopleWiring_test.go, docs/evidence/PL-04-2026-10-18.md, core/application/service/admin/MoveDryRun_test.go, test/integration/move_dry_run_test.go, apps/webapp/e2e/instanceplans.test.mjs, docs/evidence/PL-06-2026-10-18.md]
---

# Put workspaces on plans with their own limits, features and locks

## Goal

A provider sells plans — "Private", "Family", "Business" — through its own purchase or billing
platform, and Hubtask is prepared for it: the platform puts each workspace on a plan and moves it,
Hubtask holds which plan a workspace is on and applies its limits, locked values and features at
once, and never destroys anything; prices, contracts and invoices stay with the platform.

## Story

The operator defines plans on the installation. The platform provisions a workspace on "Family"
and moves it to "Business" when the customer upgrades; the new limits and features apply at the
next request. A workspace that loses a feature by downgrading keeps its data and configuration —
the rules stop running, the second provider stops signing people in — and gets both back on
upgrading.

## How to check

1. A plan carries values with locks, limits and feature switches; a workspace references its plan
   and nothing is copied into the workspace.
2. Changing a plan's value changes it for every workspace on the plan at the next request, without
   a job that visits workspaces.
3. A workspace's screens say "Set by your plan" where the plan locked a value, and "Not included in
   your plan" where a feature is off — without selling anything. A plan can leave out exactly three
   features: automation rules, webhooks, and the workspace's own sign-in providers.
4. Downgrading refuses new items beyond a limit and stops removed features — rules stop running,
   webhooks stop delivering, the workspace's own providers stop signing people in, and where that
   leaves nobody a way in the password opens; nothing existing is deleted, a value the plan newly
   locks is kept and applies again when the lock goes, and an upgrade brings each feature back.
5. Export, deletion, access requests, language, time zone and accessibility can never be switched
   off by a plan.
6. Moving a workspace to another plan is in the instance journal and in the workspace's own trail;
   changing a plan is in the instance journal.
7. Plans are created, changed, removed and assigned on the installation's screens, with
   `hubctl admin plan`, through the API and in the instance file, with the same refusals; a save
   made on a plan somebody else changed since it was read is refused with `version_conflict`.
8. A workspace can be provisioned on a plan, by every door that provisions; a plan with workspaces
   on it cannot be removed.
9. A purchase platform, with its service account's credential, provisions a workspace on a plan and
   moves it to another through the API, naming the plan by its short name, which never changes and
   is never reused once created; a move repeated with the same idempotency key moves once, and a
   move that names the workspace's version it last saw is refused with `version_conflict` when the
   workspace has changed since, so a late retry of an older downgrade cannot overwrite a newer
   upgrade. A refusal is a code the platform can act on. Hubtask never asks the platform anything.
10. A workspace carries an optional reference of the platform's own (its customer or subscription
    id), set at provisioning and changeable, unique on the installation; the platform reads one
    workspace — status, plan, version, deletion date — by its id, its short name or that reference,
    and lists workspaces filtered by plan, status and reference, page by page, so a provisioning
    call whose answer was lost can be reconciled.
11. Where the operator set an account link for the installation or the plan, owners and
    administrators see "Your provider's account page" beside "Not included in your plan", at a
    reached limit and on the suspended workspace's page; Hubtask writes the label, the operator only
    the address, and nothing shows a price, another plan or an offer to upgrade.

## Where it ends

* No prices, trials or invoices ([NG-billing](../../vision/non-goals.md)); a link to the
  provider's own page (check 11) is not selling.
* A move applies at once; a change at the end of a billing period is the platform's to schedule.
* A feature for one workspace beyond its plan is another plan.
* No per-seat licensing inside Hubtask; a limit on people is a limit like any other (UC-INS-10).
* Plans prepare Hubtask for a provider's platform; Hubtask ships no plan, and an installation
  without one — every self-hosted one — has every feature and its own limits. A single-mode
  installation offers no plans at all.
