# Slophammer TypeScript

Standalone TypeScript implementation of the Slophammer repository quality
checker. The user-facing product name is `slophammer-ts`.

The TypeScript implementation is native-first. It can also carry selected
checks for other ecosystems when those checks are covered by the shared specs
and fixtures.

## Commands

```sh
npm install -g slophammer-ts
slophammer-ts check .
slophammer-ts check . --format json
slophammer-ts check . --format sarif
slophammer-ts check . --execute
slophammer-ts check . --only ts.dependency-boundaries-required
slophammer-ts agents init . --dry-run
slophammer-ts agents check .
slophammer-ts boundaries .
slophammer-ts rules
slophammer-ts rules --format json
slophammer-ts dry .
```

Source-tree development uses the local package scripts:

```sh
npm install
npm run check
slophammer-ts check ..
slophammer-ts agents init .. --dry-run
slophammer-ts agents check ..
slophammer-ts rules
slophammer-ts rules --format json
slophammer-ts dry ..
npm run mutate
```

The npm package is released as `slophammer-ts`. The packed artifact contains
runtime `dist/src/**` files and package metadata, and exposes the public
`slophammer-ts` bin.

The `slophammer` npm package name is reserved for a future umbrella package or
default installer. The TypeScript implementation should not claim the
`slophammer` bin in its published package.

Source-tree development can also run the built CLI directly:

```sh
node dist/src/cli/main.js check ..
node dist/src/cli/main.js boundaries ..
node dist/src/cli/main.js dry ..
```

## Recognized Tool Evidence

`slophammer-ts check .` validates the quality concern, not one preferred tool
stack. Production repositories should not need wrapper scripts or JSON filtering
glue to make a real setup look like Slophammer's defaults.

The checker recognizes:

- `tsc --noEmit` and `tsgo --noEmit` for type checking.
- ESLint, Oxlint, and Biome for lint evidence.
- ESLint and Oxlint rules for explicit `any`, unsafe type operations, and
  complexity.
- Prettier, Oxfmt, Dprint, and Biome formatter checks.
- Node's built-in test runner, Vitest, Jest, Mocha, Ava, Uvu, Tap, Playwright,
  and `tsx --test`.
- `c8`, `nyc`, Vitest, and Jest coverage gates when the configured threshold is
  enforced.
- GitHub Actions workflow matrix commands when the workflow actually executes
  `${{ matrix.command }}`.

The implementation is intentionally strict. Source uses `unknown` at dynamic
boundaries, rejects `any`, keeps filesystem/process work outside the rule
engine, and declares StrykerJS as the mutation testing gate.
