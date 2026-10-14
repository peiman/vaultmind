# Commands

> **Note:** This page documents the **ckeletin-go framework's** sample/dev commands (`ping`, `config`, `dev`, …), not VaultMind's. For VaultMind's actual commands (`ask`, `search`, `note`, `index`, `doctor`, `hooks`, `version`, …) see **[AGENT_USAGE.md](AGENT_USAGE.md)** and `vaultmind --help`.

## VaultMind `note propose` and `note accept`

These two write a draft and promote it after a person reviews it. The rest of the VaultMind command reference stays in [AGENT_USAGE.md](AGENT_USAGE.md).

```bash
vaultmind note propose --question "How does the index stay current?" --finding "A write re-indexes the note before the command returns."
vaultmind note accept <id>
```

`propose` retrieves the way `ask` does. A close match (the strong relevance band, by default) becomes a proposal draft next to that note. Anything less close becomes a new draft, expanded through the note template. `accept` flips a new draft to the schema's published status, or appends a proposal onto its target and removes the proposal file. `--json` returns the v1 envelope.

## `ping` Command

Sample command demonstrating Cobra, Viper, Zerolog, and Bubble Tea integration:

```bash
./myapp ping
./myapp ping --message "Hello!" --color cyan
./myapp ping --ui
```

## `doctor` Command

Check your development environment:
```bash
task doctor
```

## `config validate` Command

Validate configuration files:
```bash
./myapp config validate
./myapp config validate --file /path/to/config.yaml
```

## `check` Command (Dev Build Only)

Run comprehensive quality checks with beautiful TUI output:
```bash
./myapp check
./myapp check --category quality
./myapp check --fail-fast --verbose
```

Categories: Development Environment, Code Quality, Architecture Validation, Dependencies, Tests.

## `dev` Command Group (Dev Build Only)

```bash
./myapp dev config    # Inspect configuration
./myapp dev doctor    # Check environment health
./myapp dev progress  # Show development progress
```

See [ADR-012](../.ckeletin/docs/adr/012-dev-commands-build-tags.md) for build tag separation.

## Adding New Commands

```bash
task generate:command name=mycommand
```

This creates the command file, metadata, and config options following the ultra-thin pattern. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the full walkthrough.
