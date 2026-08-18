# AGENTS.md

These instructions apply to this repository.

<!-- slophammer:agents:start -->
## Repository checks

Run these commands before you finish:

```sh
make check
```

## Package areas

- `go`: `go.mod`
- `packages/web`: `package.json`
- `python`: `pyproject.toml`
- `rust`: `Cargo.toml`
- `z`: `package.json`
- `ä`: `package.json`
<!-- slophammer:agents:end -->

## Working rules

- Keep changes small and reviewable.
- Add or update tests when behavior changes.
- Run the repository checks before you finish.
- Do not weaken existing checks to make a change pass.
