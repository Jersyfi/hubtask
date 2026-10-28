---
id: UC-INS-15
title: Give a workspace its own domain
context: admin
actors: [PE-owner, PE-operator]
deployments: [D5, D6, D7]
serves: [P-01, P-03, P-04, P-05, P-11, P-12, P-16]
state: specified
tasks: [SI-12, CD-07, CD-08]
checked_by: []
---

# Give a workspace its own domain

## Goal

A workspace's owner reaches the workspace at `tasks.acme.com` without asking the operator: the
owner adds two DNS records copied from the screen, and verification and the certificate follow by
themselves. Once the own domain is the main address, the workspace's subdomain redirects to it and
stays the way back. The operator sees every own domain and may block one.

## Story

*Administration → Address → Use your own domain*: type the domain, copy the two records, wait. The
screen says "waiting", "verified", "active". *Make it the main address* asks what a change of
address asks, and says what it costs; from then on the subdomain redirects to the own domain. If
the records disappear, or the operator blocks the domain, the subdomain becomes the main address
again by itself and the screen says why.

## How to check

1. Where the installation allows own domains, the workspace's owner sees *Use your own domain* in
   the workspace's address settings; where it does not, and in single mode, the control is not
   shown.
2. The screen shows the two records - one pointing the domain at the installation, one carrying
   the proof - with copy buttons, and says that mail is not affected.
3. Verification runs by itself and shows its state (waiting, verified, active, broken); nobody has
   to press anything after adding the records.
4. A domain that is the installation's own host or lies beneath it, another workspace's address,
   or one the operator blocked is refused at the field with a code naming the reason.
5. Once verified, the domain answers over HTTPS with a certificate obtained on demand; the
   operator configures nothing for that one domain.
6. Making the domain the main address asks for the confirmation of a change of address
   (UC-INS-19 checks 4, 5 and 6) and says what it costs: everybody signs in again, and passkeys
   made at the old address stop working.
7. Once the own domain is the main address, the browser on the subdomain is redirected to it, the
   API answers on both, and every link Hubtask sends from then on uses the own domain; links sent
   before keep working through the redirect.
8. The owner can make the subdomain the main address again at any time, at the same cost.
9. If the domain's records disappear, it turns broken, the subdomain becomes the main address by
   itself, and the workspace's administration and the installation's overview show the warning.
10. The operator sees every own domain with its workspace and state in the installation's
    workspaces, can block one with a reason, and a blocked domain stops answering at once; the
    owner sees that the operator blocked it.
11. Adding, verifying, making main, switching back, breaking and blocking are in the workspace's
    trail; the operator's block is also in the instance journal.

## Where it ends

* One own domain per workspace.
* The certificate is issued by the operator's ingress on demand, after it asks the installation
  whether the host is a verified own domain; Hubtask holds no certificate and no private key.
* Mail is not routed by the own domain, and nobody is routed by an address's domain
  ([multi-tenancy.md](../../architecture/multi-tenancy.md) §3.1).
* Changing the subdomain itself is UC-INS-19; a host the operator assigns is UC-INS-20.
* Milestone CD.

## Today

* Check 1: not met — there is no screen for a workspace's own domain, and no installation switch.
* Check 2: not met — there is no screen that shows the records.
* Check 3: not met — there is no verification of a domain.
* Check 4: not met — nothing refuses a domain beneath the installation's host or a blocked one.
* Check 5: not met — nothing answers the ingress whether a host may have a certificate.
* Check 6: not met — there is no confirmation for making a domain the main address.
* Check 7: not met — nothing resolves a workspace through its own domain.
* Check 8: not met — there is no way back to the subdomain.
* Check 9: not met — there is no fallback from a broken domain, and no warning.
* Check 10: not met — the operator sees no own domains and cannot block one.
* Check 11: not met — no address change reaches the trail or the journal.
