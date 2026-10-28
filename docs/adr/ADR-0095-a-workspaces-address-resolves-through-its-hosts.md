# ADR-0095 — A workspace's address resolves through its hosts

**Status:** proposed, accepted when CD-03 merges · **Date:** 2026-10-27 · **Decided:** the owner's
answers D-1 to D-8 of [decision #1568](https://github.com/Jersyfi/hubtask/issues/1568)
(2026-10-16); the mechanism below at the cut of milestone CD

**Rule lives in:** [multi-tenancy.md](../architecture/multi-tenancy.md) §3 and §3.1,
[identity.md](../architecture/identity.md) §10.1 and §23.3,
[security.md](../architecture/security.md) §4 (T-28),
[deployment.md](../architecture/deployment.md) §6.1

## Context

Decision #1568 settled what a workspace's address is: a host of its own, never a path; a short
name the operator, and where allowed the owner, can change, with the old address free at once; an
own domain the owner sets up without the operator; and a list of workspaces the device keeps.
Milestone SI prepared `tenant_host` (migration 0104) and left it unread: a request finds its
workspace through `resolve_tenant(slug)`, every mail but the invitation links to the
installation's host (#1549), the provider's return forwards only to a host beneath the
installation's own, and nothing reserves a short name (#1560). Changing an address, assigned hosts
and own domains each need a request's host to name its workspace; three separate mechanisms would
disagree about which host is the workspace's.

## Decision

1. **`tenant_host` is the resolver in multi mode.** Every workspace has rows; exactly one is its
   main address (`is_canonical`). A request's host names its workspace through one narrow
   `SECURITY DEFINER` lookup that answers one workspace or none, as `resolve_tenant` does today. A
   subdomain is a row like any other, and single mode keeps its one workspace without rows.
2. **Every address Hubtask writes comes from the main address**: mails, links, the provider's
   return (`oidc:locate`), the passkey's RP ID and the sign-in card. Nothing builds a workspace's
   address from its short name any more.
3. **A change of address replaces the main row in one transaction**, and the old row goes with it,
   so the old host answers like an address no workspace has (D-2). The workspace's passkeys stay
   stored and count as no factor, as identity.md §23.3 already says for a credential of another
   host; no other data changes.
4. **An own domain is proven by DNS and re-proven.** A TXT record carries the row's verification
   mark, a second record points the domain at the installation's own target. A job checks pending
   domains and re-checks verified ones; a domain whose proof is gone turns `BROKEN`, and the
   subdomain becomes the main address again in the same transaction. Lookups go through a port
   with a deadline (rule 7).
5. **The certificate is the ingress's, on demand.** The installation answers the ingress's
   question "may this host have a certificate" on the operations port - yes only for a verified,
   unblocked own domain or a workspace's host. Hubtask holds no certificate and no private key,
   and the question is not reachable from the public network, so it is no lookup for strangers.
6. **Assigned hosts are a row the operator writes without a proof**, in an installation set to
   `HUBTASK_WORKSPACE_HOSTS=assigned`; an installation uses subdomains or assigned hosts, never both.
7. **The device's list is signed.** After a sign-in a workspace sends the browser once to the
   installation's own host with an entry - host and display name - signed by the installation's
   key. The page there keeps the entries in its own storage and accepts one only after the server
   confirms its own signature and that the host is still a main address. The server never answers
   for an entry it did not sign, so the list is no lookup either.
8. **The switches are installation settings**: owners may change the address (default off), the
   cooldown (default 30 days), owners may set up own domains (default off), and the operator's own
   reserved short names, beside the product's built-in list.

## Consequences

* The forward rule of identity.md §10.1 widens from "beneath its own host" to "the host the lookup
  names", and the lookup answers only from `tenant_host`.
* A dangling own domain and a forged `Host` header are named as threat T-28.
* An own domain is not in the workspace's archive, as no host is ("hosts belong to the
  installation").
* Defaults that stay closed mean an installation that switches nothing on sees only the operator's
  change of address and the device's list.

## Alternatives

* **Hubtask obtains certificates itself (ACME in the binary).** Rejected: it needs the private keys
  in the application and a writable store, and every ingress already does this.
* **A plain list in the base host's storage, filled from the URL.** Rejected: any link could plant
  an entry that sends a person to a look-alike host; the signature costs one request.
* **A redirect and a hold on the old address.** Decided against by the owner (D-2).
