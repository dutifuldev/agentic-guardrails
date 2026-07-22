---
title: Required Next Work
author: Bob <dutifulbob@gmail.com>
date: 2026-05-17
status: completed
---

# Required Next Work

Slophammer is past the first release-hardening pass. The remaining required
work is about making the product contract hard to misunderstand and preparing
the Go implementation for release.

## Release Direction

Go and TypeScript are releasable implementations.

The Go release should use the `go/` submodule tag shape:

```sh
git tag go/v0.1.0
git push origin go/v0.1.0
```

Users install the released Go command with:

```sh
go install github.com/osolmaz/slophammer/go/cmd/slophammer-go@v0.1.0
```

TypeScript is released as the `slophammer-ts` npm package. TypeScript releases
use the `typescript/vX.Y.Z` tag shape:

```sh
git tag typescript/v0.1.1
git push origin typescript/v0.1.1
```

The package remains package-checked in CI so the artifact stays clean. The
`typescript/v0.1.1` tag was the first release published by npm trusted
publishing with provenance.

## Required Tasks

These are the required next tasks before treating Slophammer as ready for a Go
release.

1. [x] Strict `slophammer.yml` validation.

   Unknown keys must fail. Invalid threshold values must fail. Invalid
   dependency boundary declarations must fail with clear messages. Agents should
   not be able to invent config fields that silently do nothing.

2. [x] Machine-readable rule catalog.

   JSON output exists on the existing rules commands:

   ```sh
   slophammer-go rules --format json
   slophammer-ts rules --format json
   ```

   The JSON must come from the same rule definitions used by the checker.

3. [x] Adoption before/after fixture.

   `fixtures/repos/adoption-before` is the messy input.
   `fixtures/repos/adoption-after` is the matching clean after-state.
   The example should demonstrate the agent entrypoint workflow end to end:
   first findings, files added or changed, final clean run.

4. [x] Go release workflow.

   The dry-run release workflow for the Go implementation verifies
   the `go/vX.Y.Z` tag shape, runs the full Go and conformance checks, and proves
   the documented `go install` command works for the tagged version. The first
   workflow should not publish TypeScript.

5. [x] TypeScript npm release workflow.

   The TypeScript release workflow verifies `typescript/vX.Y.Z` tags, checks
   the tag against `typescript/package.json`, runs the full TypeScript package
   gate and shared conformance, and publishes `slophammer-ts` with npm trusted
   publishing provenance.

## Not Required For The Go Release

These are useful later, but they do not block the Go release:

- a future `slophammer` npm umbrella package or default installer
- better SARIF metadata
- a `fixtures` command
- dogfooding reports from external repositories
- Python checker implementation
- broad cross-language checking from every implementation
