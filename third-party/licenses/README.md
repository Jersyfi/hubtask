# The notices that ship with the binary

Every dependency's own licence text, copied verbatim, and the two bundled assets' texts beside
them. This directory is **generated** — run `make licenses` rather than editing it — and it is
copied into the container image at `/licenses` (`deploy/docker/Dockerfile`).

It exists because a list is not a notice. `THIRD-PARTY-LICENSES.md` records *which* dependency
carries *which* licence, which is what the `gate-licenses` build gate reads; the licences
themselves ask for something else. BSD-3-Clause says it in as many words — "redistributions **in
binary form** must reproduce the above copyright notice … in the documentation and/or other
materials provided with the distribution" — and MIT and ISC require the notice "in all copies",
while Apache-2.0 §4 requires the recipient to be given a copy of the licence and any `NOTICE`
file. The image is published to `ghcr.io` on every release, which is a distribution to third
parties, so those materials have to travel with it.

`_bundled/` is the exception to "generated": the typeface and the icon set reach the binary through
the web bundle rather than through the module graph, so `go-licenses` cannot see them. Their texts
are copied from the pnpm packages the lockfile pins — `@fontsource-variable/ibm-plex-sans` and
`lucide-static` — and are checked in rather than regenerated, so that `make licenses` keeps working
in a checkout where Node.js was never installed (`project-structure.md` §2.1).
