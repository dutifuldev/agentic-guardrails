# slophammer-py

The Python implementation of [Slophammer](https://github.com/osolmaz/slophammer),
a repository quality checker for agent-assisted projects. It shares the rule
contract, config file, report formats, and conformance fixtures with the Go,
TypeScript, and Rust implementations.

```sh
uvx slophammer-py agents init . --dry-run
uvx slophammer-py agents check .
uvx slophammer-py check .
uvx slophammer-py dry .
```

See the repository root for the rule reference and configuration docs.
