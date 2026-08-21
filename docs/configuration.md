# vaultmind Configuration

This document describes all available configuration options for vaultmind.

## Configuration Sources

Configuration can be provided in multiple ways, in order of precedence:

1. Command-line flags
2. Environment variables (with prefix `VAULTMIND_`)
3. Configuration file (~/.config/vaultmind/config.yaml)
4. Default values

## Configuration Options

| Key | Type | Default | Environment Variable | Description |
|-----|------|---------|---------------------|-------------|
| `app.log_level` | string | `warn` | `VAULTMIND_APP_LOG_LEVEL` | Logging level for the application (trace, debug, info, warn, error, fatal, panic). Used as console level if app.log.console_level is not set. |
| `app.log.console_level` | string | `` | `VAULTMIND_APP_LOG_CONSOLE_LEVEL` | Console log level (trace, debug, info, warn, error, fatal, panic). If empty, uses app.log_level. |
| `app.log.file_enabled` | bool | `false` | `VAULTMIND_APP_LOG_FILE_ENABLED` | Enable file logging to capture detailed logs |
| `app.log.file_path` | string | `./logs/vaultmind.log` | `VAULTMIND_APP_LOG_FILE_PATH` | Path to the log file (created with secure 0600 permissions) |
| `app.log.file_level` | string | `debug` | `VAULTMIND_APP_LOG_FILE_LEVEL` | File log level (trace, debug, info, warn, error, fatal, panic) |
| `app.log.color_enabled` | string | `auto` | `VAULTMIND_APP_LOG_COLOR_ENABLED` | Enable colored console output (auto, true, false). Auto detects TTY. |
| `app.log.file_max_size` | int | `100` | `VAULTMIND_APP_LOG_FILE_MAX_SIZE` | Maximum size in megabytes before log file is rotated |
| `app.log.file_max_backups` | int | `3` | `VAULTMIND_APP_LOG_FILE_MAX_BACKUPS` | Maximum number of old log files to retain |
| `app.log.file_max_age` | int | `28` | `VAULTMIND_APP_LOG_FILE_MAX_AGE` | Maximum number of days to retain old log files |
| `app.log.file_compress` | bool | `false` | `VAULTMIND_APP_LOG_FILE_COMPRESS` | Compress rotated log files with gzip |
| `app.log.sampling_enabled` | bool | `false` | `VAULTMIND_APP_LOG_SAMPLING_ENABLED` | Enable log sampling for high-volume scenarios |
| `app.log.sampling_initial` | int | `100` | `VAULTMIND_APP_LOG_SAMPLING_INITIAL` | Number of messages to log per second before sampling |
| `app.log.sampling_thereafter` | int | `100` | `VAULTMIND_APP_LOG_SAMPLING_THEREAFTER` | Number of messages to log thereafter per second |
| `app.output_format` | string | `text` | `VAULTMIND_APP_OUTPUT_FORMAT` | Output format: text (human-readable) or json (machine-readable) |
| `app.docs.output_format` | string | `markdown` | `VAULTMIND_APP_DOCS_OUTPUT_FORMAT` | Output format for documentation (markdown, yaml) |
| `app.docs.output_file` | string | `` | `VAULTMIND_APP_DOCS_OUTPUT_FILE` | Output file for documentation (defaults to stdout) |
| `app.apply.vault` | string | `.` | `VAULTMIND_APP_APPLY_VAULT` | Path to vault root |
| `app.apply.json` | bool | `false` | `VAULTMIND_APP_APPLY_JSON` | Output in JSON format |
| `app.apply.dry_run` | bool | `false` | `VAULTMIND_APP_APPLY_DRY_RUN` | Preview without executing |
| `app.apply.diff` | bool | `false` | `VAULTMIND_APP_APPLY_DIFF` | Show unified diffs |
| `app.apply.commit` | bool | `false` | `VAULTMIND_APP_APPLY_COMMIT` | Stage and commit all changes |
| `app.arc.candidates.vault` | string | `.` | `VAULTMIND_APP_ARC_CANDIDATES_VAULT` | Path to vault root |
| `app.arc.candidates.json` | bool | `false` | `VAULTMIND_APP_ARC_CANDIDATES_JSON` | Output in JSON format |
| `app.arc.candidates.arcs_vault` | string | `` | `VAULTMIND_APP_ARC_CANDIDATES_ARCS_VAULT` | Vault holding the existing arcs to compare proposals against (default: the scanned vault). Set this when the desk and the arcs live in different vaults |
| `app.ask.vault` | string | `.` | `VAULTMIND_APP_ASK_VAULT` | Path to vault root |
| `app.ask.json` | bool | `false` | `VAULTMIND_APP_ASK_JSON` | Output in JSON format |
| `app.ask.budget` | int | `4000` | `VAULTMIND_APP_ASK_BUDGET` | Token budget for context-pack |
| `app.ask.max_items` | int | `8` | `VAULTMIND_APP_ASK_MAX_ITEMS` | Max context items |
| `app.ask.search_limit` | int | `5` | `VAULTMIND_APP_ASK_SEARCH_LIMIT` | Max search hits |
| `app.ask.explain` | bool | `false` | `VAULTMIND_APP_ASK_EXPLAIN` | Show per-lane RRF contributions for each hit |
| `app.ask.pointers_only` | bool | `false` | `VAULTMIND_APP_ASK_POINTERS_ONLY` | Skip context-pack bodies; render only id+title+type pointers (forces ask-to-read loop instead of letting the preload satisfy curiosity) |
| `app.ask.preview` | bool | `false` | `VAULTMIND_APP_ASK_PREVIEW` | Render a one-line body snippet under each ranked hit; bridges --pointers-only (titles only) and the full context-pack output |
| `app.ask.read` | string | `` | `VAULTMIND_APP_ASK_READ` | Read the body of the named hit inline after the menu — accepts a 1-indexed rank (e.g. --read 2) or an exact id (e.g. --read concept-foo). Single-command shortcut for probe→read when you already know which hit from the titles |
| `app.ask.quiet_on_nomatch` | bool | `false` | `VAULTMIND_APP_ASK_QUIET_ON_NOMATCH` | Print nothing when the top hit is at/below the noise floor (no_match). For ambient recall: inject silence instead of irrelevant pointers when the prompt is off-domain. Also skips the context-pack and access fan-out so off-domain prompts don't reinforce irrelevant notes. |
| `app.ask.excerpt` | int | `0` | `VAULTMIND_APP_ASK_EXCERPT` | Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one (an arc's rule lives there; its opening is story setup), else the opening prose. Applies to the target note and every context item. 0 = off, which also means an over-budget note contributes no text at all while the pack still counts it, so a tight budget yields items with no content |
| `app.check.fail_fast` | bool | `false` | `VAULTMIND_APP_CHECK_FAIL_FAST` | Stop on first failed check |
| `app.check.verbose` | bool | `false` | `VAULTMIND_APP_CHECK_VERBOSE` | Show verbose output including command details |
| `app.check.parallel` | bool | `true` | `VAULTMIND_APP_CHECK_PARALLEL` | Run checks within each category in parallel (disable with --parallel=false) |
| `app.check.category` | string | `` | `VAULTMIND_APP_CHECK_CATEGORY` | Filter to specific categories (comma-separated: environment,quality,architecture,security,dependencies,tests) |
| `app.check.timing` | bool | `true` | `VAULTMIND_APP_CHECK_TIMING` | Show duration for each check in the output |
| `app.dataviewrender.vault` | string | `.` | `VAULTMIND_APP_DATAVIEWRENDER_VAULT` | Path to vault root |
| `app.dataviewrender.json` | bool | `false` | `VAULTMIND_APP_DATAVIEWRENDER_JSON` | Output in JSON format |
| `app.dataviewrender.dry_run` | bool | `false` | `VAULTMIND_APP_DATAVIEWRENDER_DRY_RUN` | Preview without writing |
| `app.dataviewrender.diff` | bool | `false` | `VAULTMIND_APP_DATAVIEWRENDER_DIFF` | Show unified diff |
| `app.dataviewrender.commit` | bool | `false` | `VAULTMIND_APP_DATAVIEWRENDER_COMMIT` | Stage and commit |
| `app.dataviewrender.force` | bool | `false` | `VAULTMIND_APP_DATAVIEWRENDER_FORCE` | Override checksum mismatch |
| `app.dataviewrender.section_key` | string | `` | `VAULTMIND_APP_DATAVIEWRENDER_SECTION_KEY` | Section key to render |
| `app.dataviewlint.vault` | string | `.` | `VAULTMIND_APP_DATAVIEWLINT_VAULT` | Path to vault root |
| `app.dataviewlint.json` | bool | `false` | `VAULTMIND_APP_DATAVIEWLINT_JSON` | Output in JSON format |
| `app.docs_commands.output` | string | `` | `VAULTMIND_APP_DOCS_COMMANDS_OUTPUT` | Output file for the command reference (defaults to stdout) |
| `app.doctor.vault` | string | `.` | `VAULTMIND_APP_DOCTOR_VAULT` | Path to vault root |
| `app.doctor.json` | bool | `false` | `VAULTMIND_APP_DOCTOR_JSON` | Output in JSON format |
| `app.doctor.summary` | bool | `false` | `VAULTMIND_APP_DOCTOR_SUMMARY` | Print summary counts only (suppress per-link details) |
| `app.doctor.all` | bool | `false` | `VAULTMIND_APP_DOCTOR_ALL` | Diagnose every vault discovered under --root (multi-vault health) |
| `app.doctor.root` | string | `.` | `VAULTMIND_APP_DOCTOR_ROOT` | Root directory to discover vaults under when --all is set |
| `app.doctor.mesh_root_pubkey` | string | `` | `VAULTMIND_APP_DOCTOR_MESH_ROOT_PUBKEY` | Pin the Contract-B network root pubkey (base64) for authenticated mesh health; overrides the enroll-persisted anchor |
| `app.doctor.mesh_registry` | string | `` | `VAULTMIND_APP_DOCTOR_MESH_REGISTRY` | Verify an offline Contract-B signed-registry file instead of fetching from the local daemon |
| `app.doctor.mesh_slug` | string | `` | `VAULTMIND_APP_DOCTOR_MESH_SLUG` | Override the agent slug used to resolve your binding in the mesh registry (default: agents.yaml) |
| `app.doctor.mesh_heartbeat` | string | `` | `VAULTMIND_APP_DOCTOR_MESH_HEARTBEAT` | Override the wake-watcher heartbeat file path used for mesh watcher-liveness |
| `app.doctorheal.vault` | string | `.` | `VAULTMIND_APP_DOCTORHEAL_VAULT` | Path to vault root |
| `app.doctorheal.json` | bool | `false` | `VAULTMIND_APP_DOCTORHEAL_JSON` | Output in JSON format |
| `app.doctorheal.dry_run` | bool | `false` | `VAULTMIND_APP_DOCTORHEAL_DRY_RUN` | Preview repairs without writing (default applies) |
| `app.doctorhealwikilinks.vault` | string | `.` | `VAULTMIND_APP_DOCTORHEALWIKILINKS_VAULT` | Path to vault root |
| `app.doctorhealwikilinks.json` | bool | `false` | `VAULTMIND_APP_DOCTORHEALWIKILINKS_JSON` | Output in JSON format |
| `app.doctorhealwikilinks.dry_run` | bool | `false` | `VAULTMIND_APP_DOCTORHEALWIKILINKS_DRY_RUN` | Preview repairs without writing (default applies) |
| `app.experimentreport.experiment` | string | `` | `VAULTMIND_APP_EXPERIMENTREPORT_EXPERIMENT` | Experiment name to report on |
| `app.experimentreport.json` | bool | `false` | `VAULTMIND_APP_EXPERIMENTREPORT_JSON` | Output in JSON format |
| `app.experimentreport.k` | int | `10` | `VAULTMIND_APP_EXPERIMENTREPORT_K` | K value for Hit@K metric |
| `app.experimentsummary.json` | bool | `false` | `VAULTMIND_APP_EXPERIMENTSUMMARY_JSON` | Output in JSON format |
| `app.experimentsummary.top` | int | `10` | `VAULTMIND_APP_EXPERIMENTSUMMARY_TOP` | Number of top-recalled notes to show |
| `app.experimenttrace.session` | string | `` | `VAULTMIND_APP_EXPERIMENTTRACE_SESSION` | Session ID to trace |
| `app.experimenttrace.note` | string | `` | `VAULTMIND_APP_EXPERIMENTTRACE_NOTE` | Note ID to trace across sessions |
| `app.experimenttrace.json` | bool | `false` | `VAULTMIND_APP_EXPERIMENTTRACE_JSON` | Output in JSON format |
| `app.experimentcompare.session` | string | `` | `VAULTMIND_APP_EXPERIMENTCOMPARE_SESSION` | Restrict to a single session ID |
| `app.experimentcompare.caller` | string | `` | `VAULTMIND_APP_EXPERIMENTCOMPARE_CALLER` | Restrict to a single caller label |
| `app.experimentcompare.since` | string | `` | `VAULTMIND_APP_EXPERIMENTCOMPARE_SINCE` | Only events at or after this RFC3339 timestamp |
| `app.experimentcompare.event_type` | string | `` | `VAULTMIND_APP_EXPERIMENTCOMPARE_EVENT_TYPE` | Restrict to one event type (ask|search|context_pack); empty = all three |
| `app.experimentcompare.k` | int | `10` | `VAULTMIND_APP_EXPERIMENTCOMPARE_K` | K value for Jaccard@K (and the cap on list length used for Kendall's tau) |
| `app.experimentcompare.per_event` | bool | `false` | `VAULTMIND_APP_EXPERIMENTCOMPARE_PER_EVENT` | Emit one row per event in addition to aggregates |
| `app.experimentcompare.json` | bool | `false` | `VAULTMIND_APP_EXPERIMENTCOMPARE_JSON` | Output in JSON format |
| `experiments` | map | `map[]` | `VAULTMIND_EXPERIMENTS` | Top-level experiment definitions map |
| `experiments.telemetry` | string | `anonymous` | `VAULTMIND_EXPERIMENTS_TELEMETRY` | What export may emit — anonymous (strips vault paths, query text, note ids, result paths, caller_meta) | full | off (nothing is written locally either) |
| `experiments.outcome_window_sessions` | int | `2` | `VAULTMIND_EXPERIMENTS_OUTCOME_WINDOW_SESSIONS` | Sessions to look back for outcome linkage |
| `experiments.activation.delta` | float | `0.2` | `VAULTMIND_EXPERIMENTS_ACTIVATION_DELTA` | Spreading activation weight (0.0 disables similarity component) |
| `app.export.output` | string | `` | `VAULTMIND_APP_EXPORT_OUTPUT` | Output file path (empty = stdout) |
| `app.export.tier` | string | `` | `VAULTMIND_APP_EXPORT_TIER` | Override telemetry tier (off|anonymous|full); empty = use experiments.telemetry from config |
| `app.export.rollup` | bool | `false` | `VAULTMIND_APP_EXPORT_ROLLUP` | Emit a federated-aggregator-shaped rollup (vault fingerprint + features + variant stats) instead of raw events |
| `app.export.vault` | string | `.` | `VAULTMIND_APP_EXPORT_VAULT` | Vault path (required when --rollup is set; reads index DB and fingerprint) |
| `app.export.preview` | bool | `false` | `VAULTMIND_APP_EXPORT_PREVIEW` | Print a human-readable summary instead of writing the JSON payload (useful for auditing before sharing) |
| `app.frontmatter.vault` | string | `.` | `VAULTMIND_APP_FRONTMATTER_VAULT` | Path to vault root |
| `app.frontmatter.json` | bool | `false` | `VAULTMIND_APP_FRONTMATTER_JSON` | Output in JSON format |
| `app.frontmatter.live` | bool | `false` | `VAULTMIND_APP_FRONTMATTER_LIVE` | Validate raw .md files on disk instead of the indexed database |
| `app.frontmatterfix.vault` | string | `.` | `VAULTMIND_APP_FRONTMATTERFIX_VAULT` | Path to vault root |
| `app.frontmatterfix.json` | bool | `false` | `VAULTMIND_APP_FRONTMATTERFIX_JSON` | Output in JSON format |
| `app.frontmatterfix.apply` | bool | `false` | `VAULTMIND_APP_FRONTMATTERFIX_APPLY` | Apply changes (default: dry-run) |
| `app.frontmatterset.vault` | string | `.` | `VAULTMIND_APP_FRONTMATTERSET_VAULT` | Path to vault root |
| `app.frontmatterset.json` | bool | `false` | `VAULTMIND_APP_FRONTMATTERSET_JSON` | Output in JSON format |
| `app.frontmatterset.dry_run` | bool | `false` | `VAULTMIND_APP_FRONTMATTERSET_DRY_RUN` | Preview changes without writing |
| `app.frontmatterset.diff` | bool | `false` | `VAULTMIND_APP_FRONTMATTERSET_DIFF` | Show unified diff |
| `app.frontmatterset.commit` | bool | `false` | `VAULTMIND_APP_FRONTMATTERSET_COMMIT` | Stage and commit after mutation |
| `app.frontmatterset.allow_extra` | bool | `false` | `VAULTMIND_APP_FRONTMATTERSET_ALLOW_EXTRA` | Allow keys not in type schema |
| `app.frontmatterunset.vault` | string | `.` | `VAULTMIND_APP_FRONTMATTERUNSET_VAULT` | Path to vault root |
| `app.frontmatterunset.json` | bool | `false` | `VAULTMIND_APP_FRONTMATTERUNSET_JSON` | Output in JSON format |
| `app.frontmatterunset.dry_run` | bool | `false` | `VAULTMIND_APP_FRONTMATTERUNSET_DRY_RUN` | Preview changes without writing |
| `app.frontmatterunset.diff` | bool | `false` | `VAULTMIND_APP_FRONTMATTERUNSET_DIFF` | Show unified diff |
| `app.frontmatterunset.commit` | bool | `false` | `VAULTMIND_APP_FRONTMATTERUNSET_COMMIT` | Stage and commit after mutation |
| `app.frontmattermerge.vault` | string | `.` | `VAULTMIND_APP_FRONTMATTERMERGE_VAULT` | Path to vault root |
| `app.frontmattermerge.json` | bool | `false` | `VAULTMIND_APP_FRONTMATTERMERGE_JSON` | Output in JSON format |
| `app.frontmattermerge.dry_run` | bool | `false` | `VAULTMIND_APP_FRONTMATTERMERGE_DRY_RUN` | Preview changes without writing |
| `app.frontmattermerge.diff` | bool | `false` | `VAULTMIND_APP_FRONTMATTERMERGE_DIFF` | Show unified diff |
| `app.frontmattermerge.commit` | bool | `false` | `VAULTMIND_APP_FRONTMATTERMERGE_COMMIT` | Stage and commit after mutation |
| `app.frontmattermerge.allow_extra` | bool | `false` | `VAULTMIND_APP_FRONTMATTERMERGE_ALLOW_EXTRA` | Allow keys not in type schema |
| `app.frontmattermerge.file` | string | `` | `VAULTMIND_APP_FRONTMATTERMERGE_FILE` | YAML file with fields to merge |
| `app.frontmatternormalize.vault` | string | `.` | `VAULTMIND_APP_FRONTMATTERNORMALIZE_VAULT` | Path to vault root |
| `app.frontmatternormalize.json` | bool | `false` | `VAULTMIND_APP_FRONTMATTERNORMALIZE_JSON` | Output in JSON format |
| `app.frontmatternormalize.dry_run` | bool | `false` | `VAULTMIND_APP_FRONTMATTERNORMALIZE_DRY_RUN` | Preview changes without writing |
| `app.frontmatternormalize.diff` | bool | `false` | `VAULTMIND_APP_FRONTMATTERNORMALIZE_DIFF` | Show unified diff |
| `app.frontmatternormalize.commit` | bool | `false` | `VAULTMIND_APP_FRONTMATTERNORMALIZE_COMMIT` | Stage and commit after mutation |
| `app.frontmatternormalize.strip_time` | bool | `false` | `VAULTMIND_APP_FRONTMATTERNORMALIZE_STRIP_TIME` | Force all datetimes to date-only |
| `app.gitstatus.vault` | string | `.` | `VAULTMIND_APP_GITSTATUS_VAULT` | Path to vault root |
| `app.gitstatus.json` | bool | `false` | `VAULTMIND_APP_GITSTATUS_JSON` | Output in JSON format |
| `app.hooksinstall.force` | bool | `false` | `VAULTMIND_APP_HOOKSINSTALL_FORCE` | Overwrite existing hook scripts (default: refuse) |
| `app.hooksinstall.json` | bool | `false` | `VAULTMIND_APP_HOOKSINSTALL_JSON` | Output in JSON format |
| `app.hooksinstall.only` | string | `` | `VAULTMIND_APP_HOOKSINSTALL_ONLY` | Comma-separated subset of canonical scripts to install (default: all). Unknown names rejected at lint time. |
| `app.hooksinstall.vault` | string | `` | `VAULTMIND_APP_HOOKSINSTALL_VAULT` | Vault path to bake into the printed settings.json stanza via VAULTMIND_VAULT (default: the built-in vaultmind-identity convention). |
| `app.hooksinstall.merge` | bool | `false` | `VAULTMIND_APP_HOOKSINSTALL_MERGE` | Additively merge the hook stanza into the project's settings file (never clobbers existing hooks) instead of only printing it. |
| `app.hooksinstall.local` | bool | `false` | `VAULTMIND_APP_HOOKSINSTALL_LOCAL` | With --merge, target .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared). |
| `app.hooksinstall.dryrun` | bool | `false` | `VAULTMIND_APP_HOOKSINSTALL_DRYRUN` | With --merge, print the merged result without writing it (preview/diff). |
| `app.hooksstatus.json` | bool | `false` | `VAULTMIND_APP_HOOKSSTATUS_JSON` | Output in JSON format |
| `app.hooksuninstall.json` | bool | `false` | `VAULTMIND_APP_HOOKSUNINSTALL_JSON` | Output in JSON format |
| `app.hooksuninstall.local` | bool | `false` | `VAULTMIND_APP_HOOKSUNINSTALL_LOCAL` | Target .claude/settings.local.json instead of .claude/settings.json. |
| `app.hooksuninstall.removescripts` | bool | `false` | `VAULTMIND_APP_HOOKSUNINSTALL_REMOVESCRIPTS` | Also delete the installed hook scripts under .claude/scripts/ (default: leave them). |
| `app.identityenrolladd.request` | string | `` | `VAULTMIND_APP_IDENTITYENROLLADD_REQUEST` | Signed enrollment-request JSON file (stdin when empty or "-") |
| `app.identityenrolladd.registry` | string | `` | `VAULTMIND_APP_IDENTITYENROLLADD_REGISTRY` | Current registry file: unsigned wireRegistry OR signed envelope (absent => fresh) |
| `app.identityenrolladd.root_pubkey` | string | `` | `VAULTMIND_APP_IDENTITYENROLLADD_ROOT_PUBKEY` | Base64-std root ed25519 pubkey (required for a signed-envelope --registry; derives the network) |
| `app.identityenrolladd.network_id` | string | `` | `VAULTMIND_APP_IDENTITYENROLLADD_NETWORK_ID` | Admin network id (vmnet1:…); alternative to --root-pubkey (>=1 required; both must agree) |
| `app.identityenrolladd.validity_seconds` | string | `` | `VAULTMIND_APP_IDENTITYENROLLADD_VALIDITY_SECONDS` | Registry+binding issuance window in seconds (default 31536000 = one year) |
| `app.identityenrolladd.origin_daemon` | string | `` | `VAULTMIND_APP_IDENTITYENROLLADD_ORIGIN_DAEMON` | Comma-separated authorized origin daemon ids for the new binding (default none) |
| `app.identityenroll.invite` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_INVITE` | Network invite: a vmenroll1: token or enroll URL (required) |
| `app.identityenroll.display_name` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_DISPLAY_NAME` | Your display name in the network (required; Unicode NFC) |
| `app.identityenroll.slug` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_SLUG` | Your short ASCII handle/slug (required) |
| `app.identityenroll.pubkey` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_PUBKEY` | Your base64-std ed25519 identity pubkey from `identity init` (required) |
| `app.identityenroll.transport_pubkey` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_TRANSPORT_PUBKEY` | Your base64-std 32-byte WireGuard pubkey from `wg pubkey` (required) |
| `app.identityenroll.transport_endpoint` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_TRANSPORT_ENDPOINT` | Optional reachable host:port (IPv6 bracketed); omitted when empty |
| `app.identityenroll.signer_socket` | string | `` | `VAULTMIND_APP_IDENTITYENROLL_SIGNER_SOCKET` | Signer socket path (default: XDG state dir) |
| `app.identityenroll.yes` | bool | `false` | `VAULTMIND_APP_IDENTITYENROLL_YES` | Skip the out-of-band fingerprint confirmation prompt |
| `app.identityinit.signer_key` | string | `` | `VAULTMIND_APP_IDENTITYINIT_SIGNER_KEY` | Sealed signer key path (default: XDG data dir) |
| `app.identityinvite.root_pubkey` | string | `` | `VAULTMIND_APP_IDENTITYINVITE_ROOT_PUBKEY` | Network ROOT public key (base64-std of the 32-byte ed25519 key; required) |
| `app.identityinvite.relay` | string | `` | `VAULTMIND_APP_IDENTITYINVITE_RELAY` | Relay base URL, e.g. https://chat.acme.com (required) |
| `app.identitysign.file` | string | `` | `VAULTMIND_APP_IDENTITYSIGN_FILE` | Read entry JSON from this file instead of stdin |
| `app.identitysign.signer_socket` | string | `` | `VAULTMIND_APP_IDENTITYSIGN_SIGNER_SOCKET` | Signer socket path (default: XDG state dir) |
| `app.identitysignenrollment.file` | string | `` | `VAULTMIND_APP_IDENTITYSIGNENROLLMENT_FILE` | Read enrollment request JSON from this file instead of stdin |
| `app.identitysignenrollment.signer_socket` | string | `` | `VAULTMIND_APP_IDENTITYSIGNENROLLMENT_SIGNER_SOCKET` | Signer socket path (default: XDG state dir) |
| `app.identitysignenvelope.file` | string | `` | `VAULTMIND_APP_IDENTITYSIGNENVELOPE_FILE` | Read envelope JSON from this file instead of stdin |
| `app.identitysignenvelope.signer_socket` | string | `` | `VAULTMIND_APP_IDENTITYSIGNENVELOPE_SIGNER_SOCKET` | Signer socket path (default: XDG state dir) |
| `app.identitysignenvelope.from_pubkey` | string | `` | `VAULTMIND_APP_IDENTITYSIGNENVELOPE_FROM_PUBKEY` | Signer public key (base64) stamped as the from_pubkey hint; not signed |
| `app.identitysignregistry.file` | string | `` | `VAULTMIND_APP_IDENTITYSIGNREGISTRY_FILE` | Read registry JSON from this file instead of stdin |
| `app.identitysignregistry.signer_socket` | string | `` | `VAULTMIND_APP_IDENTITYSIGNREGISTRY_SIGNER_SOCKET` | Signer socket path (default: XDG state dir) |
| `app.identitysigner.signer_key` | string | `` | `VAULTMIND_APP_IDENTITYSIGNER_SIGNER_KEY` | Sealed signer key path (default: XDG data dir) |
| `app.identitysigner.signer_socket` | string | `` | `VAULTMIND_APP_IDENTITYSIGNER_SIGNER_SOCKET` | Signer socket path (default: XDG state dir) |
| `app.index.vault` | string | `.` | `VAULTMIND_APP_INDEX_VAULT` | Path to the vault root directory |
| `app.index.json` | bool | `false` | `VAULTMIND_APP_INDEX_JSON` | Output in JSON format |
| `app.index.full` | bool | `false` | `VAULTMIND_APP_INDEX_FULL` | Force full rebuild instead of incremental index |
| `app.index.embed` | bool | `false` | `VAULTMIND_APP_INDEX_EMBED` | Compute and store embeddings for note bodies |
| `app.index.model` | string | `` | `VAULTMIND_APP_INDEX_MODEL` | Embedding model: minilm (384d, fast) or bge-m3 (1024d, 3-in-1). Empty (default) auto-selects: bge-m3 on ORT-tagged builds, minilm on pure-Go. |
| `app.index.allow_slow_backend` | bool | `false` | `VAULTMIND_APP_INDEX_ALLOW_SLOW_BACKEND` | Allow BGE-M3 indexing on the pure-Go backend (hours for medium vaults) |
| `app.init.print_instructions` | bool | `false` | `VAULTMIND_APP_INIT_PRINT_INSTRUCTIONS` | Print the concise agent-onboarding quick-start and exit (no vault created); add --full for the whole guide |
| `app.init.full` | bool | `false` | `VAULTMIND_APP_INIT_FULL` | With --print-instructions, print the full agent-onboarding guide instead of the concise quick-start |
| `app.init.wire_hooks` | bool | `false` | `VAULTMIND_APP_INIT_WIRE_HOOKS` | After scaffolding, install the Claude Code hook scripts into the current project and merge the wiring into .claude/settings.json (baked to the new vault). Never clobbers existing hooks. |
| `app.init.local` | bool | `false` | `VAULTMIND_APP_INIT_LOCAL` | With --wire-hooks, merge into .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared). |
| `app.init.dry_run` | bool | `false` | `VAULTMIND_APP_INIT_DRY_RUN` | With --wire-hooks, print the would-be settings merge without writing it (preview). |
| `app.init.project_dir` | string | `` | `VAULTMIND_APP_INIT_PROJECT_DIR` | With --wire-hooks, the project to wire hooks into (where .claude/ lives). Defaults to the current directory; set it when the vault and the project root differ. |
| `app.links.vault` | string | `.` | `VAULTMIND_APP_LINKS_VAULT` | Path to vault root |
| `app.links.json` | bool | `false` | `VAULTMIND_APP_LINKS_JSON` | Output in JSON format |
| `app.links.edge_type` | string | `` | `VAULTMIND_APP_LINKS_EDGE_TYPE` | Filter by edge type |
| `app.linksneighbors.vault` | string | `.` | `VAULTMIND_APP_LINKSNEIGHBORS_VAULT` | Path to vault root |
| `app.linksneighbors.json` | bool | `false` | `VAULTMIND_APP_LINKSNEIGHBORS_JSON` | Output in JSON format |
| `app.linksneighbors.depth` | int | `1` | `VAULTMIND_APP_LINKSNEIGHBORS_DEPTH` | Maximum traversal depth |
| `app.linksneighbors.min_confidence` | string | `low` | `VAULTMIND_APP_LINKSNEIGHBORS_MIN_CONFIDENCE` | Minimum edge confidence (low, medium, high) |
| `app.linksneighbors.max_nodes` | int | `200` | `VAULTMIND_APP_LINKSNEIGHBORS_MAX_NODES` | Maximum nodes to return |
| `app.lintfixlinks.vault` | string | `.` | `VAULTMIND_APP_LINTFIXLINKS_VAULT` | Path to vault root |
| `app.lintfixlinks.json` | bool | `false` | `VAULTMIND_APP_LINTFIXLINKS_JSON` | Output in JSON format |
| `app.lintfixlinks.fix` | bool | `false` | `VAULTMIND_APP_LINTFIXLINKS_FIX` | Apply fixes (default is dry-run) |
| `app.memoryrecall.vault` | string | `.` | `VAULTMIND_APP_MEMORYRECALL_VAULT` | Path to vault root |
| `app.memoryrecall.json` | bool | `false` | `VAULTMIND_APP_MEMORYRECALL_JSON` | Output in JSON format |
| `app.memoryrecall.depth` | int | `1` | `VAULTMIND_APP_MEMORYRECALL_DEPTH` | Maximum traversal depth |
| `app.memoryrecall.min_confidence` | string | `high` | `VAULTMIND_APP_MEMORYRECALL_MIN_CONFIDENCE` | Minimum edge confidence (low, medium, high) |
| `app.memoryrecall.max_nodes` | int | `50` | `VAULTMIND_APP_MEMORYRECALL_MAX_NODES` | Maximum nodes to return |
| `app.memoryrelated.vault` | string | `.` | `VAULTMIND_APP_MEMORYRELATED_VAULT` | Path to vault root |
| `app.memoryrelated.json` | bool | `false` | `VAULTMIND_APP_MEMORYRELATED_JSON` | Output in JSON format |
| `app.memoryrelated.mode` | string | `mixed` | `VAULTMIND_APP_MEMORYRELATED_MODE` | Filter mode (explicit, inferred, mixed) |
| `app.memorycontextpack.vault` | string | `.` | `VAULTMIND_APP_MEMORYCONTEXTPACK_VAULT` | Path to vault root |
| `app.memorycontextpack.json` | bool | `false` | `VAULTMIND_APP_MEMORYCONTEXTPACK_JSON` | Output in JSON format |
| `app.memorycontextpack.budget` | int | `4096` | `VAULTMIND_APP_MEMORYCONTEXTPACK_BUDGET` | Token budget |
| `app.memorycontextpack.depth` | int | `1` | `VAULTMIND_APP_MEMORYCONTEXTPACK_DEPTH` | BFS traversal depth (1 = direct neighbors only) |
| `app.memorycontextpack.max_items` | int | `0` | `VAULTMIND_APP_MEMORYCONTEXTPACK_MAX_ITEMS` | Max context items (0 = unlimited) |
| `app.memorycontextpack.slim` | bool | `false` | `VAULTMIND_APP_MEMORYCONTEXTPACK_SLIM` | Slim frontmatter (type, title, status only) |
| `app.memorysummarize.vault` | string | `.` | `VAULTMIND_APP_MEMORYSUMMARIZE_VAULT` | Path to vault root |
| `app.memorysummarize.json` | bool | `false` | `VAULTMIND_APP_MEMORYSUMMARIZE_JSON` | Output in JSON format |
| `app.memorysummarize.ids` | string | `` | `VAULTMIND_APP_MEMORYSUMMARIZE_IDS` | Comma-separated note IDs (alternative to positional args) |
| `app.memorysummarize.include_body` | bool | `false` | `VAULTMIND_APP_MEMORYSUMMARIZE_INCLUDE_BODY` | Include body text excerpts |
| `app.memorysummarize.max_body_len` | int | `0` | `VAULTMIND_APP_MEMORYSUMMARIZE_MAX_BODY_LEN` | Max body chars per note (0 = full) |
| `app.memorylinks.vault` | string | `.` | `VAULTMIND_APP_MEMORYLINKS_VAULT` | Path to vault root |
| `app.memorylinks.json` | bool | `false` | `VAULTMIND_APP_MEMORYLINKS_JSON` | Output in JSON format |
| `app.memorylinks.edge_type` | string | `` | `VAULTMIND_APP_MEMORYLINKS_EDGE_TYPE` | Filter by edge type |
| `app.memorylinks.out` | bool | `false` | `VAULTMIND_APP_MEMORYLINKS_OUT` | Show only outbound edges |
| `app.memorylinks.in` | bool | `false` | `VAULTMIND_APP_MEMORYLINKS_IN` | Show only inbound edges (backlinks) |
| `app.memorylinks.both` | bool | `false` | `VAULTMIND_APP_MEMORYLINKS_BOTH` | Show both inbound and outbound edges (default) |
| `app.memoryneighbors.vault` | string | `.` | `VAULTMIND_APP_MEMORYNEIGHBORS_VAULT` | Path to vault root |
| `app.memoryneighbors.json` | bool | `false` | `VAULTMIND_APP_MEMORYNEIGHBORS_JSON` | Output in JSON format |
| `app.memoryneighbors.depth` | int | `1` | `VAULTMIND_APP_MEMORYNEIGHBORS_DEPTH` | Maximum traversal depth |
| `app.memoryneighbors.min_confidence` | string | `high` | `VAULTMIND_APP_MEMORYNEIGHBORS_MIN_CONFIDENCE` | Minimum edge confidence (low, medium, high) |
| `app.memoryneighbors.max_nodes` | int | `50` | `VAULTMIND_APP_MEMORYNEIGHBORS_MAX_NODES` | Maximum nodes to return |
| `app.memorypack.vault` | string | `.` | `VAULTMIND_APP_MEMORYPACK_VAULT` | Path to vault root |
| `app.memorypack.json` | bool | `false` | `VAULTMIND_APP_MEMORYPACK_JSON` | Output in JSON format |
| `app.memorypack.budget` | int | `4096` | `VAULTMIND_APP_MEMORYPACK_BUDGET` | Token budget |
| `app.memorypack.depth` | int | `1` | `VAULTMIND_APP_MEMORYPACK_DEPTH` | BFS traversal depth (1 = direct neighbors only) |
| `app.memorypack.max_items` | int | `0` | `VAULTMIND_APP_MEMORYPACK_MAX_ITEMS` | Max context items (0 = unlimited) |
| `app.memorypack.excerpt` | int | `0` | `VAULTMIND_APP_MEMORYPACK_EXCERPT` | Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one, else its opening prose. 0 = off, which also means a note larger than the remaining budget contributes no text at all while the pack still counts it |
| `app.memorypack.slim` | bool | `false` | `VAULTMIND_APP_MEMORYPACK_SLIM` | Slim frontmatter (type, title, status only) |
| `app.note.vault` | string | `.` | `VAULTMIND_APP_NOTE_VAULT` | Path to vault root |
| `app.note.json` | bool | `false` | `VAULTMIND_APP_NOTE_JSON` | Output in JSON format |
| `app.note.frontmatter_only` | bool | `false` | `VAULTMIND_APP_NOTE_FRONTMATTER_ONLY` | Omit body, headings, blocks |
| `app.notecreate.vault` | string | `.` | `VAULTMIND_APP_NOTECREATE_VAULT` | Path to vault root |
| `app.notecreate.json` | bool | `false` | `VAULTMIND_APP_NOTECREATE_JSON` | Output in JSON format |
| `app.notecreate.type` | string | `` | `VAULTMIND_APP_NOTECREATE_TYPE` | Note type (required) |
| `app.notecreate.body` | string | `` | `VAULTMIND_APP_NOTECREATE_BODY` | Body text (overrides template body) |
| `app.notecreate.commit` | bool | `false` | `VAULTMIND_APP_NOTECREATE_COMMIT` | Stage and commit |
| `app.ping.output_message` | string | `Pong` | `VAULTMIND_APP_PING_OUTPUT_MESSAGE` | Default message to display for the ping command |
| `app.ping.output_color` | string | `white` | `VAULTMIND_APP_PING_OUTPUT_COLOR` | Text color for ping command output (white, red, green, blue, cyan, yellow, magenta) |
| `app.ping.ui` | bool | `false` | `VAULTMIND_APP_PING_UI` | Enable interactive UI for the ping command |
| `app.resolve.vault` | string | `.` | `VAULTMIND_APP_RESOLVE_VAULT` | Path to the vault root directory |
| `app.resolve.json` | bool | `false` | `VAULTMIND_APP_RESOLVE_JSON` | Output in JSON format |
| `app.schema.vault` | string | `.` | `VAULTMIND_APP_SCHEMA_VAULT` | Path to vault root |
| `app.schema.json` | bool | `false` | `VAULTMIND_APP_SCHEMA_JSON` | Output in JSON format |
| `app.search.vault` | string | `.` | `VAULTMIND_APP_SEARCH_VAULT` | Path to vault root |
| `app.search.json` | bool | `false` | `VAULTMIND_APP_SEARCH_JSON` | Output in JSON format |
| `app.search.limit` | int | `20` | `VAULTMIND_APP_SEARCH_LIMIT` | Maximum results to return |
| `app.search.offset` | int | `0` | `VAULTMIND_APP_SEARCH_OFFSET` | Skip first N results |
| `app.search.type` | string | `` | `VAULTMIND_APP_SEARCH_TYPE` | Filter by note type |
| `app.search.tag` | string | `` | `VAULTMIND_APP_SEARCH_TAG` | Filter by tag |
| `app.search.mode` | string | `keyword` | `VAULTMIND_APP_SEARCH_MODE` | Search mode: keyword, semantic, or hybrid |
| `app.self.vault` | string | `.` | `VAULTMIND_APP_SELF_VAULT` | Path to vault root |
| `app.self.limit` | int | `10` | `VAULTMIND_APP_SELF_LIMIT` | Max rows per section (recent/hot/stale) |
| `app.vaultstatus.vault` | string | `.` | `VAULTMIND_APP_VAULTSTATUS_VAULT` | Path to vault root |
| `app.vaultstatus.json` | bool | `false` | `VAULTMIND_APP_VAULTSTATUS_JSON` | Output in JSON format |

## Example Configuration

### YAML Configuration File (config.yaml)

```yaml
app:
  # Logging level for the application (trace, debug, info, warn, error, fatal, panic). Used as console level if app.log.console_level is not set.
  log_level: debug

  # Output format: text (human-readable) or json (machine-readable)
  output_format: json

  apply:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview without executing
    dry_run: false

    # Show unified diffs
    diff: false

    # Stage and commit all changes
    commit: false

  ask:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Token budget for context-pack
    budget: 4000

    # Max context items
    max_items: 8

    # Max search hits
    search_limit: 5

    # Show per-lane RRF contributions for each hit
    explain: false

    # Skip context-pack bodies; render only id+title+type pointers (forces ask-to-read loop instead of letting the preload satisfy curiosity)
    pointers_only: false

    # Render a one-line body snippet under each ranked hit; bridges --pointers-only (titles only) and the full context-pack output
    preview: false

    # Read the body of the named hit inline after the menu — accepts a 1-indexed rank (e.g. --read 2) or an exact id (e.g. --read concept-foo). Single-command shortcut for probe→read when you already know which hit from the titles
    read: 

    # Print nothing when the top hit is at/below the noise floor (no_match). For ambient recall: inject silence instead of irrelevant pointers when the prompt is off-domain. Also skips the context-pack and access fan-out so off-domain prompts don't reinforce irrelevant notes.
    quiet_on_nomatch: false

    # Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one (an arc's rule lives there; its opening is story setup), else the opening prose. Applies to the target note and every context item. 0 = off, which also means an over-budget note contributes no text at all while the pack still counts it, so a tight budget yields items with no content
    excerpt: 0

  check:
    # Stop on first failed check
    fail_fast: true

    # Show verbose output including command details
    verbose: true

    # Run checks within each category in parallel (disable with --parallel=false)
    parallel: false

    # Filter to specific categories (comma-separated: environment,quality,architecture,security,dependencies,tests)
    category: security,tests

    # Show duration for each check in the output
    timing: false

  dataviewrender:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview without writing
    dry_run: false

    # Show unified diff
    diff: false

    # Stage and commit
    commit: false

    # Override checksum mismatch
    force: false

    # Section key to render
    section_key: 

  memoryneighbors:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Maximum traversal depth
    depth: 1

    # Minimum edge confidence (low, medium, high)
    min_confidence: high

    # Maximum nodes to return
    max_nodes: 50

  notecreate:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Note type (required)
    type: 

    # Body text (overrides template body)
    body: 

    # Stage and commit
    commit: false

  doctorhealwikilinks:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview repairs without writing (default applies)
    dry_run: false

  export:
    # Output file path (empty = stdout)
    output: 

    # Override telemetry tier (off|anonymous|full); empty = use experiments.telemetry from config
    tier: 

    # Emit a federated-aggregator-shaped rollup (vault fingerprint + features + variant stats) instead of raw events
    rollup: false

    # Vault path (required when --rollup is set; reads index DB and fingerprint)
    vault: .

    # Print a human-readable summary instead of writing the JSON payload (useful for auditing before sharing)
    preview: false

  identityinvite:
    # Network ROOT public key (base64-std of the 32-byte ed25519 key; required)
    root_pubkey: 

    # Relay base URL, e.g. https://chat.acme.com (required)
    relay: 

  identitysignregistry:
    # Read registry JSON from this file instead of stdin
    file: 

    # Signer socket path (default: XDG state dir)
    signer_socket: 

  identitysigner:
    # Sealed signer key path (default: XDG data dir)
    signer_key: 

    # Signer socket path (default: XDG state dir)
    signer_socket: 

  resolve:
    # Path to the vault root directory
    vault: .

    # Output in JSON format
    json: false

  self:
    # Path to vault root
    vault: .

    # Max rows per section (recent/hot/stale)
    limit: 10

  arc:
    # Path to vault root
    candidates.vault: .

    # Output in JSON format
    candidates.json: false

    # Vault holding the existing arcs to compare proposals against (default: the scanned vault). Set this when the desk and the arcs live in different vaults
    candidates.arcs_vault: 

  docs_commands:
    # Output file for the command reference (defaults to stdout)
    output: internal/onboard/COMMANDS.md

  experimentreport:
    # Experiment name to report on
    experiment: 

    # Output in JSON format
    json: false

    # K value for Hit@K metric
    k: 10

  experimentcompare:
    # Restrict to a single session ID
    session: 

    # Restrict to a single caller label
    caller: 

    # Only events at or after this RFC3339 timestamp
    since: 

    # Restrict to one event type (ask|search|context_pack); empty = all three
    event_type: 

    # K value for Jaccard@K (and the cap on list length used for Kendall's tau)
    k: 10

    # Emit one row per event in addition to aggregates
    per_event: false

    # Output in JSON format
    json: false

  frontmatter:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Validate raw .md files on disk instead of the indexed database
    live: false

  frontmatterset:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview changes without writing
    dry_run: false

    # Show unified diff
    diff: false

    # Stage and commit after mutation
    commit: false

    # Allow keys not in type schema
    allow_extra: false

  frontmatternormalize:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview changes without writing
    dry_run: false

    # Show unified diff
    diff: false

    # Stage and commit after mutation
    commit: false

    # Force all datetimes to date-only
    strip_time: false

  identityenroll:
    # Network invite: a vmenroll1: token or enroll URL (required)
    invite: 

    # Your display name in the network (required; Unicode NFC)
    display_name: 

    # Your short ASCII handle/slug (required)
    slug: 

    # Your base64-std ed25519 identity pubkey from `identity init` (required)
    pubkey: 

    # Your base64-std 32-byte WireGuard pubkey from `wg pubkey` (required)
    transport_pubkey: 

    # Optional reachable host:port (IPv6 bracketed); omitted when empty
    transport_endpoint: 

    # Signer socket path (default: XDG state dir)
    signer_socket: 

    # Skip the out-of-band fingerprint confirmation prompt
    yes: false

  dataviewlint:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

  identityinit:
    # Sealed signer key path (default: XDG data dir)
    signer_key: 

  identitysignenrollment:
    # Read enrollment request JSON from this file instead of stdin
    file: 

    # Signer socket path (default: XDG state dir)
    signer_socket: 

  identitysignenvelope:
    # Read envelope JSON from this file instead of stdin
    file: 

    # Signer socket path (default: XDG state dir)
    signer_socket: 

    # Signer public key (base64) stamped as the from_pubkey hint; not signed
    from_pubkey: 

  index:
    # Path to the vault root directory
    vault: ./my-vault

    # Output in JSON format
    json: true

    # Force full rebuild instead of incremental index
    full: true

    # Compute and store embeddings for note bodies
    embed: true

    # Embedding model: minilm (384d, fast) or bge-m3 (1024d, 3-in-1). Empty (default) auto-selects: bge-m3 on ORT-tagged builds, minilm on pure-Go.
    model: bge-m3

    # Allow BGE-M3 indexing on the pure-Go backend (hours for medium vaults)
    allow_slow_backend: true

  links:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Filter by edge type
    edge_type: 

  memoryrecall:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Maximum traversal depth
    depth: 1

    # Minimum edge confidence (low, medium, high)
    min_confidence: high

    # Maximum nodes to return
    max_nodes: 50

  memorycontextpack:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Token budget
    budget: 4096

    # BFS traversal depth (1 = direct neighbors only)
    depth: 1

    # Max context items (0 = unlimited)
    max_items: 0

    # Slim frontmatter (type, title, status only)
    slim: false

  log:
    # Console log level (trace, debug, info, warn, error, fatal, panic). If empty, uses app.log_level.
    console_level: info

    # Enable file logging to capture detailed logs
    file_enabled: true

    # Path to the log file (created with secure 0600 permissions)
    file_path: /var/log/vaultmind/app.log

    # File log level (trace, debug, info, warn, error, fatal, panic)
    file_level: debug

    # Enable colored console output (auto, true, false). Auto detects TTY.
    color_enabled: true

    # Maximum size in megabytes before log file is rotated
    file_max_size: 100

    # Maximum number of old log files to retain
    file_max_backups: 3

    # Maximum number of days to retain old log files
    file_max_age: 28

    # Compress rotated log files with gzip
    file_compress: true

    # Enable log sampling for high-volume scenarios
    sampling_enabled: true

    # Number of messages to log per second before sampling
    sampling_initial: 100

    # Number of messages to log thereafter per second
    sampling_thereafter: 100

  hooksinstall:
    # Overwrite existing hook scripts (default: refuse)
    force: false

    # Output in JSON format
    json: false

    # Comma-separated subset of canonical scripts to install (default: all). Unknown names rejected at lint time.
    only: 

    # Vault path to bake into the printed settings.json stanza via VAULTMIND_VAULT (default: the built-in vaultmind-identity convention).
    vault: 

    # Additively merge the hook stanza into the project's settings file (never clobbers existing hooks) instead of only printing it.
    merge: false

    # With --merge, target .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared).
    local: false

    # With --merge, print the merged result without writing it (preview/diff).
    dryrun: false

  hooksstatus:
    # Output in JSON format
    json: false

  memorylinks:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Filter by edge type
    edge_type: 

    # Show only outbound edges
    out: false

    # Show only inbound edges (backlinks)
    in: false

    # Show both inbound and outbound edges (default)
    both: false

  ping:
    # Default message to display for the ping command
    output_message: Hello World!

    # Text color for ping command output (white, red, green, blue, cyan, yellow, magenta)
    output_color: green

    # Enable interactive UI for the ping command
    ui: true

  vaultstatus:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

  frontmatterunset:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview changes without writing
    dry_run: false

    # Show unified diff
    diff: false

    # Stage and commit after mutation
    commit: false

  gitstatus:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

  memoryrelated:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Filter mode (explicit, inferred, mixed)
    mode: mixed

  memorypack:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Token budget
    budget: 4096

    # BFS traversal depth (1 = direct neighbors only)
    depth: 1

    # Max context items (0 = unlimited)
    max_items: 0

    # Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one, else its opening prose. 0 = off, which also means a note larger than the remaining budget contributes no text at all while the pack still counts it
    excerpt: 0

    # Slim frontmatter (type, title, status only)
    slim: false

  search:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Maximum results to return
    limit: 20

    # Skip first N results
    offset: 0

    # Filter by note type
    type: 

    # Filter by tag
    tag: 

    # Search mode: keyword, semantic, or hybrid
    mode: keyword

  identityenrolladd:
    # Signed enrollment-request JSON file (stdin when empty or "-")
    request: 

    # Current registry file: unsigned wireRegistry OR signed envelope (absent => fresh)
    registry: 

    # Base64-std root ed25519 pubkey (required for a signed-envelope --registry; derives the network)
    root_pubkey: 

    # Admin network id (vmnet1:…); alternative to --root-pubkey (>=1 required; both must agree)
    network_id: 

    # Registry+binding issuance window in seconds (default 31536000 = one year)
    validity_seconds: 

    # Comma-separated authorized origin daemon ids for the new binding (default none)
    origin_daemon: 

  doctor:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Print summary counts only (suppress per-link details)
    summary: false

    # Diagnose every vault discovered under --root (multi-vault health)
    all: false

    # Root directory to discover vaults under when --all is set
    root: .

    # Pin the Contract-B network root pubkey (base64) for authenticated mesh health; overrides the enroll-persisted anchor
    mesh_root_pubkey: 

    # Verify an offline Contract-B signed-registry file instead of fetching from the local daemon
    mesh_registry: 

    # Override the agent slug used to resolve your binding in the mesh registry (default: agents.yaml)
    mesh_slug: 

    # Override the wake-watcher heartbeat file path used for mesh watcher-liveness
    mesh_heartbeat: 

  experimentsummary:
    # Output in JSON format
    json: false

    # Number of top-recalled notes to show
    top: 10

  experimenttrace:
    # Session ID to trace
    session: 

    # Note ID to trace across sessions
    note: 

    # Output in JSON format
    json: false

  frontmatterfix:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Apply changes (default: dry-run)
    apply: false

  frontmattermerge:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview changes without writing
    dry_run: false

    # Show unified diff
    diff: false

    # Stage and commit after mutation
    commit: false

    # Allow keys not in type schema
    allow_extra: false

    # YAML file with fields to merge
    file: 

  hooksuninstall:
    # Output in JSON format
    json: false

    # Target .claude/settings.local.json instead of .claude/settings.json.
    local: false

    # Also delete the installed hook scripts under .claude/scripts/ (default: leave them).
    removescripts: false

  identitysign:
    # Read entry JSON from this file instead of stdin
    file: 

    # Signer socket path (default: XDG state dir)
    signer_socket: 

  docs:
    # Output format for documentation (markdown, yaml)
    output_format: yaml

    # Output file for documentation (defaults to stdout)
    output_file: /path/to/output.md

  doctorheal:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Preview repairs without writing (default applies)
    dry_run: false

  init:
    # Print the concise agent-onboarding quick-start and exit (no vault created); add --full for the whole guide
    print_instructions: false

    # With --print-instructions, print the full agent-onboarding guide instead of the concise quick-start
    full: false

    # After scaffolding, install the Claude Code hook scripts into the current project and merge the wiring into .claude/settings.json (baked to the new vault). Never clobbers existing hooks.
    wire_hooks: false

    # With --wire-hooks, merge into .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared).
    local: false

    # With --wire-hooks, print the would-be settings merge without writing it (preview).
    dry_run: false

    # With --wire-hooks, the project to wire hooks into (where .claude/ lives). Defaults to the current directory; set it when the vault and the project root differ.
    project_dir: 

  linksneighbors:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Maximum traversal depth
    depth: 1

    # Minimum edge confidence (low, medium, high)
    min_confidence: low

    # Maximum nodes to return
    max_nodes: 200

  lintfixlinks:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Apply fixes (default is dry-run)
    fix: false

  memorysummarize:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Comma-separated note IDs (alternative to positional args)
    ids: 

    # Include body text excerpts
    include_body: false

    # Max body chars per note (0 = full)
    max_body_len: 0

  note:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

    # Omit body, headings, blocks
    frontmatter_only: false

  schema:
    # Path to vault root
    vault: .

    # Output in JSON format
    json: false

  # Top-level experiment definitions map
  experiments: map[]

experiments:
  # What export may emit — anonymous (strips vault paths, query text, note ids, result paths, caller_meta) | full | off (nothing is written locally either)
  telemetry: anonymous

  # Sessions to look back for outcome linkage
  outcome_window_sessions: 2

  activation:
    # Spreading activation weight (0.0 disables similarity component)
    delta: 0.2

```

### Environment Variables

```bash
# Logging level for the application (trace, debug, info, warn, error, fatal, panic). Used as console level if app.log.console_level is not set.
export VAULTMIND_APP_LOG_LEVEL=debug

# Console log level (trace, debug, info, warn, error, fatal, panic). If empty, uses app.log_level.
export VAULTMIND_APP_LOG_CONSOLE_LEVEL=info

# Enable file logging to capture detailed logs
export VAULTMIND_APP_LOG_FILE_ENABLED=true

# Path to the log file (created with secure 0600 permissions)
export VAULTMIND_APP_LOG_FILE_PATH=/var/log/vaultmind/app.log

# File log level (trace, debug, info, warn, error, fatal, panic)
export VAULTMIND_APP_LOG_FILE_LEVEL=debug

# Enable colored console output (auto, true, false). Auto detects TTY.
export VAULTMIND_APP_LOG_COLOR_ENABLED=true

# Maximum size in megabytes before log file is rotated
export VAULTMIND_APP_LOG_FILE_MAX_SIZE=100

# Maximum number of old log files to retain
export VAULTMIND_APP_LOG_FILE_MAX_BACKUPS=3

# Maximum number of days to retain old log files
export VAULTMIND_APP_LOG_FILE_MAX_AGE=28

# Compress rotated log files with gzip
export VAULTMIND_APP_LOG_FILE_COMPRESS=true

# Enable log sampling for high-volume scenarios
export VAULTMIND_APP_LOG_SAMPLING_ENABLED=true

# Number of messages to log per second before sampling
export VAULTMIND_APP_LOG_SAMPLING_INITIAL=100

# Number of messages to log thereafter per second
export VAULTMIND_APP_LOG_SAMPLING_THEREAFTER=100

# Output format: text (human-readable) or json (machine-readable)
export VAULTMIND_APP_OUTPUT_FORMAT=json

# Output format for documentation (markdown, yaml)
export VAULTMIND_APP_DOCS_OUTPUT_FORMAT=yaml

# Output file for documentation (defaults to stdout)
export VAULTMIND_APP_DOCS_OUTPUT_FILE=/path/to/output.md

# Path to vault root
export VAULTMIND_APP_APPLY_VAULT=.

# Output in JSON format
export VAULTMIND_APP_APPLY_JSON=false

# Preview without executing
export VAULTMIND_APP_APPLY_DRY_RUN=false

# Show unified diffs
export VAULTMIND_APP_APPLY_DIFF=false

# Stage and commit all changes
export VAULTMIND_APP_APPLY_COMMIT=false

# Path to vault root
export VAULTMIND_APP_ARC_CANDIDATES_VAULT=.

# Output in JSON format
export VAULTMIND_APP_ARC_CANDIDATES_JSON=false

# Vault holding the existing arcs to compare proposals against (default: the scanned vault). Set this when the desk and the arcs live in different vaults
export VAULTMIND_APP_ARC_CANDIDATES_ARCS_VAULT=

# Path to vault root
export VAULTMIND_APP_ASK_VAULT=.

# Output in JSON format
export VAULTMIND_APP_ASK_JSON=false

# Token budget for context-pack
export VAULTMIND_APP_ASK_BUDGET=4000

# Max context items
export VAULTMIND_APP_ASK_MAX_ITEMS=8

# Max search hits
export VAULTMIND_APP_ASK_SEARCH_LIMIT=5

# Show per-lane RRF contributions for each hit
export VAULTMIND_APP_ASK_EXPLAIN=false

# Skip context-pack bodies; render only id+title+type pointers (forces ask-to-read loop instead of letting the preload satisfy curiosity)
export VAULTMIND_APP_ASK_POINTERS_ONLY=false

# Render a one-line body snippet under each ranked hit; bridges --pointers-only (titles only) and the full context-pack output
export VAULTMIND_APP_ASK_PREVIEW=false

# Read the body of the named hit inline after the menu — accepts a 1-indexed rank (e.g. --read 2) or an exact id (e.g. --read concept-foo). Single-command shortcut for probe→read when you already know which hit from the titles
export VAULTMIND_APP_ASK_READ=

# Print nothing when the top hit is at/below the noise floor (no_match). For ambient recall: inject silence instead of irrelevant pointers when the prompt is off-domain. Also skips the context-pack and access fan-out so off-domain prompts don't reinforce irrelevant notes.
export VAULTMIND_APP_ASK_QUIET_ON_NOMATCH=false

# Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one (an arc's rule lives there; its opening is story setup), else the opening prose. Applies to the target note and every context item. 0 = off, which also means an over-budget note contributes no text at all while the pack still counts it, so a tight budget yields items with no content
export VAULTMIND_APP_ASK_EXCERPT=0

# Stop on first failed check
export VAULTMIND_APP_CHECK_FAIL_FAST=true

# Show verbose output including command details
export VAULTMIND_APP_CHECK_VERBOSE=true

# Run checks within each category in parallel (disable with --parallel=false)
export VAULTMIND_APP_CHECK_PARALLEL=false

# Filter to specific categories (comma-separated: environment,quality,architecture,security,dependencies,tests)
export VAULTMIND_APP_CHECK_CATEGORY=security,tests

# Show duration for each check in the output
export VAULTMIND_APP_CHECK_TIMING=false

# Path to vault root
export VAULTMIND_APP_DATAVIEWRENDER_VAULT=.

# Output in JSON format
export VAULTMIND_APP_DATAVIEWRENDER_JSON=false

# Preview without writing
export VAULTMIND_APP_DATAVIEWRENDER_DRY_RUN=false

# Show unified diff
export VAULTMIND_APP_DATAVIEWRENDER_DIFF=false

# Stage and commit
export VAULTMIND_APP_DATAVIEWRENDER_COMMIT=false

# Override checksum mismatch
export VAULTMIND_APP_DATAVIEWRENDER_FORCE=false

# Section key to render
export VAULTMIND_APP_DATAVIEWRENDER_SECTION_KEY=

# Path to vault root
export VAULTMIND_APP_DATAVIEWLINT_VAULT=.

# Output in JSON format
export VAULTMIND_APP_DATAVIEWLINT_JSON=false

# Output file for the command reference (defaults to stdout)
export VAULTMIND_APP_DOCS_COMMANDS_OUTPUT=internal/onboard/COMMANDS.md

# Path to vault root
export VAULTMIND_APP_DOCTOR_VAULT=.

# Output in JSON format
export VAULTMIND_APP_DOCTOR_JSON=false

# Print summary counts only (suppress per-link details)
export VAULTMIND_APP_DOCTOR_SUMMARY=false

# Diagnose every vault discovered under --root (multi-vault health)
export VAULTMIND_APP_DOCTOR_ALL=false

# Root directory to discover vaults under when --all is set
export VAULTMIND_APP_DOCTOR_ROOT=.

# Pin the Contract-B network root pubkey (base64) for authenticated mesh health; overrides the enroll-persisted anchor
export VAULTMIND_APP_DOCTOR_MESH_ROOT_PUBKEY=

# Verify an offline Contract-B signed-registry file instead of fetching from the local daemon
export VAULTMIND_APP_DOCTOR_MESH_REGISTRY=

# Override the agent slug used to resolve your binding in the mesh registry (default: agents.yaml)
export VAULTMIND_APP_DOCTOR_MESH_SLUG=

# Override the wake-watcher heartbeat file path used for mesh watcher-liveness
export VAULTMIND_APP_DOCTOR_MESH_HEARTBEAT=

# Path to vault root
export VAULTMIND_APP_DOCTORHEAL_VAULT=.

# Output in JSON format
export VAULTMIND_APP_DOCTORHEAL_JSON=false

# Preview repairs without writing (default applies)
export VAULTMIND_APP_DOCTORHEAL_DRY_RUN=false

# Path to vault root
export VAULTMIND_APP_DOCTORHEALWIKILINKS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_DOCTORHEALWIKILINKS_JSON=false

# Preview repairs without writing (default applies)
export VAULTMIND_APP_DOCTORHEALWIKILINKS_DRY_RUN=false

# Experiment name to report on
export VAULTMIND_APP_EXPERIMENTREPORT_EXPERIMENT=

# Output in JSON format
export VAULTMIND_APP_EXPERIMENTREPORT_JSON=false

# K value for Hit@K metric
export VAULTMIND_APP_EXPERIMENTREPORT_K=10

# Output in JSON format
export VAULTMIND_APP_EXPERIMENTSUMMARY_JSON=false

# Number of top-recalled notes to show
export VAULTMIND_APP_EXPERIMENTSUMMARY_TOP=10

# Session ID to trace
export VAULTMIND_APP_EXPERIMENTTRACE_SESSION=

# Note ID to trace across sessions
export VAULTMIND_APP_EXPERIMENTTRACE_NOTE=

# Output in JSON format
export VAULTMIND_APP_EXPERIMENTTRACE_JSON=false

# Restrict to a single session ID
export VAULTMIND_APP_EXPERIMENTCOMPARE_SESSION=

# Restrict to a single caller label
export VAULTMIND_APP_EXPERIMENTCOMPARE_CALLER=

# Only events at or after this RFC3339 timestamp
export VAULTMIND_APP_EXPERIMENTCOMPARE_SINCE=

# Restrict to one event type (ask|search|context_pack); empty = all three
export VAULTMIND_APP_EXPERIMENTCOMPARE_EVENT_TYPE=

# K value for Jaccard@K (and the cap on list length used for Kendall's tau)
export VAULTMIND_APP_EXPERIMENTCOMPARE_K=10

# Emit one row per event in addition to aggregates
export VAULTMIND_APP_EXPERIMENTCOMPARE_PER_EVENT=false

# Output in JSON format
export VAULTMIND_APP_EXPERIMENTCOMPARE_JSON=false

# Top-level experiment definitions map
export VAULTMIND_EXPERIMENTS=map[]

# What export may emit — anonymous (strips vault paths, query text, note ids, result paths, caller_meta) | full | off (nothing is written locally either)
export VAULTMIND_EXPERIMENTS_TELEMETRY=anonymous

# Sessions to look back for outcome linkage
export VAULTMIND_EXPERIMENTS_OUTCOME_WINDOW_SESSIONS=2

# Spreading activation weight (0.0 disables similarity component)
export VAULTMIND_EXPERIMENTS_ACTIVATION_DELTA=0.2

# Output file path (empty = stdout)
export VAULTMIND_APP_EXPORT_OUTPUT=

# Override telemetry tier (off|anonymous|full); empty = use experiments.telemetry from config
export VAULTMIND_APP_EXPORT_TIER=

# Emit a federated-aggregator-shaped rollup (vault fingerprint + features + variant stats) instead of raw events
export VAULTMIND_APP_EXPORT_ROLLUP=false

# Vault path (required when --rollup is set; reads index DB and fingerprint)
export VAULTMIND_APP_EXPORT_VAULT=.

# Print a human-readable summary instead of writing the JSON payload (useful for auditing before sharing)
export VAULTMIND_APP_EXPORT_PREVIEW=false

# Path to vault root
export VAULTMIND_APP_FRONTMATTER_VAULT=.

# Output in JSON format
export VAULTMIND_APP_FRONTMATTER_JSON=false

# Validate raw .md files on disk instead of the indexed database
export VAULTMIND_APP_FRONTMATTER_LIVE=false

# Path to vault root
export VAULTMIND_APP_FRONTMATTERFIX_VAULT=.

# Output in JSON format
export VAULTMIND_APP_FRONTMATTERFIX_JSON=false

# Apply changes (default: dry-run)
export VAULTMIND_APP_FRONTMATTERFIX_APPLY=false

# Path to vault root
export VAULTMIND_APP_FRONTMATTERSET_VAULT=.

# Output in JSON format
export VAULTMIND_APP_FRONTMATTERSET_JSON=false

# Preview changes without writing
export VAULTMIND_APP_FRONTMATTERSET_DRY_RUN=false

# Show unified diff
export VAULTMIND_APP_FRONTMATTERSET_DIFF=false

# Stage and commit after mutation
export VAULTMIND_APP_FRONTMATTERSET_COMMIT=false

# Allow keys not in type schema
export VAULTMIND_APP_FRONTMATTERSET_ALLOW_EXTRA=false

# Path to vault root
export VAULTMIND_APP_FRONTMATTERUNSET_VAULT=.

# Output in JSON format
export VAULTMIND_APP_FRONTMATTERUNSET_JSON=false

# Preview changes without writing
export VAULTMIND_APP_FRONTMATTERUNSET_DRY_RUN=false

# Show unified diff
export VAULTMIND_APP_FRONTMATTERUNSET_DIFF=false

# Stage and commit after mutation
export VAULTMIND_APP_FRONTMATTERUNSET_COMMIT=false

# Path to vault root
export VAULTMIND_APP_FRONTMATTERMERGE_VAULT=.

# Output in JSON format
export VAULTMIND_APP_FRONTMATTERMERGE_JSON=false

# Preview changes without writing
export VAULTMIND_APP_FRONTMATTERMERGE_DRY_RUN=false

# Show unified diff
export VAULTMIND_APP_FRONTMATTERMERGE_DIFF=false

# Stage and commit after mutation
export VAULTMIND_APP_FRONTMATTERMERGE_COMMIT=false

# Allow keys not in type schema
export VAULTMIND_APP_FRONTMATTERMERGE_ALLOW_EXTRA=false

# YAML file with fields to merge
export VAULTMIND_APP_FRONTMATTERMERGE_FILE=

# Path to vault root
export VAULTMIND_APP_FRONTMATTERNORMALIZE_VAULT=.

# Output in JSON format
export VAULTMIND_APP_FRONTMATTERNORMALIZE_JSON=false

# Preview changes without writing
export VAULTMIND_APP_FRONTMATTERNORMALIZE_DRY_RUN=false

# Show unified diff
export VAULTMIND_APP_FRONTMATTERNORMALIZE_DIFF=false

# Stage and commit after mutation
export VAULTMIND_APP_FRONTMATTERNORMALIZE_COMMIT=false

# Force all datetimes to date-only
export VAULTMIND_APP_FRONTMATTERNORMALIZE_STRIP_TIME=false

# Path to vault root
export VAULTMIND_APP_GITSTATUS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_GITSTATUS_JSON=false

# Overwrite existing hook scripts (default: refuse)
export VAULTMIND_APP_HOOKSINSTALL_FORCE=false

# Output in JSON format
export VAULTMIND_APP_HOOKSINSTALL_JSON=false

# Comma-separated subset of canonical scripts to install (default: all). Unknown names rejected at lint time.
export VAULTMIND_APP_HOOKSINSTALL_ONLY=

# Vault path to bake into the printed settings.json stanza via VAULTMIND_VAULT (default: the built-in vaultmind-identity convention).
export VAULTMIND_APP_HOOKSINSTALL_VAULT=

# Additively merge the hook stanza into the project's settings file (never clobbers existing hooks) instead of only printing it.
export VAULTMIND_APP_HOOKSINSTALL_MERGE=false

# With --merge, target .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared).
export VAULTMIND_APP_HOOKSINSTALL_LOCAL=false

# With --merge, print the merged result without writing it (preview/diff).
export VAULTMIND_APP_HOOKSINSTALL_DRYRUN=false

# Output in JSON format
export VAULTMIND_APP_HOOKSSTATUS_JSON=false

# Output in JSON format
export VAULTMIND_APP_HOOKSUNINSTALL_JSON=false

# Target .claude/settings.local.json instead of .claude/settings.json.
export VAULTMIND_APP_HOOKSUNINSTALL_LOCAL=false

# Also delete the installed hook scripts under .claude/scripts/ (default: leave them).
export VAULTMIND_APP_HOOKSUNINSTALL_REMOVESCRIPTS=false

# Signed enrollment-request JSON file (stdin when empty or "-")
export VAULTMIND_APP_IDENTITYENROLLADD_REQUEST=

# Current registry file: unsigned wireRegistry OR signed envelope (absent => fresh)
export VAULTMIND_APP_IDENTITYENROLLADD_REGISTRY=

# Base64-std root ed25519 pubkey (required for a signed-envelope --registry; derives the network)
export VAULTMIND_APP_IDENTITYENROLLADD_ROOT_PUBKEY=

# Admin network id (vmnet1:…); alternative to --root-pubkey (>=1 required; both must agree)
export VAULTMIND_APP_IDENTITYENROLLADD_NETWORK_ID=

# Registry+binding issuance window in seconds (default 31536000 = one year)
export VAULTMIND_APP_IDENTITYENROLLADD_VALIDITY_SECONDS=

# Comma-separated authorized origin daemon ids for the new binding (default none)
export VAULTMIND_APP_IDENTITYENROLLADD_ORIGIN_DAEMON=

# Network invite: a vmenroll1: token or enroll URL (required)
export VAULTMIND_APP_IDENTITYENROLL_INVITE=

# Your display name in the network (required; Unicode NFC)
export VAULTMIND_APP_IDENTITYENROLL_DISPLAY_NAME=

# Your short ASCII handle/slug (required)
export VAULTMIND_APP_IDENTITYENROLL_SLUG=

# Your base64-std ed25519 identity pubkey from `identity init` (required)
export VAULTMIND_APP_IDENTITYENROLL_PUBKEY=

# Your base64-std 32-byte WireGuard pubkey from `wg pubkey` (required)
export VAULTMIND_APP_IDENTITYENROLL_TRANSPORT_PUBKEY=

# Optional reachable host:port (IPv6 bracketed); omitted when empty
export VAULTMIND_APP_IDENTITYENROLL_TRANSPORT_ENDPOINT=

# Signer socket path (default: XDG state dir)
export VAULTMIND_APP_IDENTITYENROLL_SIGNER_SOCKET=

# Skip the out-of-band fingerprint confirmation prompt
export VAULTMIND_APP_IDENTITYENROLL_YES=false

# Sealed signer key path (default: XDG data dir)
export VAULTMIND_APP_IDENTITYINIT_SIGNER_KEY=

# Network ROOT public key (base64-std of the 32-byte ed25519 key; required)
export VAULTMIND_APP_IDENTITYINVITE_ROOT_PUBKEY=

# Relay base URL, e.g. https://chat.acme.com (required)
export VAULTMIND_APP_IDENTITYINVITE_RELAY=

# Read entry JSON from this file instead of stdin
export VAULTMIND_APP_IDENTITYSIGN_FILE=

# Signer socket path (default: XDG state dir)
export VAULTMIND_APP_IDENTITYSIGN_SIGNER_SOCKET=

# Read enrollment request JSON from this file instead of stdin
export VAULTMIND_APP_IDENTITYSIGNENROLLMENT_FILE=

# Signer socket path (default: XDG state dir)
export VAULTMIND_APP_IDENTITYSIGNENROLLMENT_SIGNER_SOCKET=

# Read envelope JSON from this file instead of stdin
export VAULTMIND_APP_IDENTITYSIGNENVELOPE_FILE=

# Signer socket path (default: XDG state dir)
export VAULTMIND_APP_IDENTITYSIGNENVELOPE_SIGNER_SOCKET=

# Signer public key (base64) stamped as the from_pubkey hint; not signed
export VAULTMIND_APP_IDENTITYSIGNENVELOPE_FROM_PUBKEY=

# Read registry JSON from this file instead of stdin
export VAULTMIND_APP_IDENTITYSIGNREGISTRY_FILE=

# Signer socket path (default: XDG state dir)
export VAULTMIND_APP_IDENTITYSIGNREGISTRY_SIGNER_SOCKET=

# Sealed signer key path (default: XDG data dir)
export VAULTMIND_APP_IDENTITYSIGNER_SIGNER_KEY=

# Signer socket path (default: XDG state dir)
export VAULTMIND_APP_IDENTITYSIGNER_SIGNER_SOCKET=

# Path to the vault root directory
export VAULTMIND_APP_INDEX_VAULT=./my-vault

# Output in JSON format
export VAULTMIND_APP_INDEX_JSON=true

# Force full rebuild instead of incremental index
export VAULTMIND_APP_INDEX_FULL=true

# Compute and store embeddings for note bodies
export VAULTMIND_APP_INDEX_EMBED=true

# Embedding model: minilm (384d, fast) or bge-m3 (1024d, 3-in-1). Empty (default) auto-selects: bge-m3 on ORT-tagged builds, minilm on pure-Go.
export VAULTMIND_APP_INDEX_MODEL=bge-m3

# Allow BGE-M3 indexing on the pure-Go backend (hours for medium vaults)
export VAULTMIND_APP_INDEX_ALLOW_SLOW_BACKEND=true

# Print the concise agent-onboarding quick-start and exit (no vault created); add --full for the whole guide
export VAULTMIND_APP_INIT_PRINT_INSTRUCTIONS=false

# With --print-instructions, print the full agent-onboarding guide instead of the concise quick-start
export VAULTMIND_APP_INIT_FULL=false

# After scaffolding, install the Claude Code hook scripts into the current project and merge the wiring into .claude/settings.json (baked to the new vault). Never clobbers existing hooks.
export VAULTMIND_APP_INIT_WIRE_HOOKS=false

# With --wire-hooks, merge into .claude/settings.local.json (gitignored, personal) instead of .claude/settings.json (committed, team-shared).
export VAULTMIND_APP_INIT_LOCAL=false

# With --wire-hooks, print the would-be settings merge without writing it (preview).
export VAULTMIND_APP_INIT_DRY_RUN=false

# With --wire-hooks, the project to wire hooks into (where .claude/ lives). Defaults to the current directory; set it when the vault and the project root differ.
export VAULTMIND_APP_INIT_PROJECT_DIR=

# Path to vault root
export VAULTMIND_APP_LINKS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_LINKS_JSON=false

# Filter by edge type
export VAULTMIND_APP_LINKS_EDGE_TYPE=

# Path to vault root
export VAULTMIND_APP_LINKSNEIGHBORS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_LINKSNEIGHBORS_JSON=false

# Maximum traversal depth
export VAULTMIND_APP_LINKSNEIGHBORS_DEPTH=1

# Minimum edge confidence (low, medium, high)
export VAULTMIND_APP_LINKSNEIGHBORS_MIN_CONFIDENCE=low

# Maximum nodes to return
export VAULTMIND_APP_LINKSNEIGHBORS_MAX_NODES=200

# Path to vault root
export VAULTMIND_APP_LINTFIXLINKS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_LINTFIXLINKS_JSON=false

# Apply fixes (default is dry-run)
export VAULTMIND_APP_LINTFIXLINKS_FIX=false

# Path to vault root
export VAULTMIND_APP_MEMORYRECALL_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYRECALL_JSON=false

# Maximum traversal depth
export VAULTMIND_APP_MEMORYRECALL_DEPTH=1

# Minimum edge confidence (low, medium, high)
export VAULTMIND_APP_MEMORYRECALL_MIN_CONFIDENCE=high

# Maximum nodes to return
export VAULTMIND_APP_MEMORYRECALL_MAX_NODES=50

# Path to vault root
export VAULTMIND_APP_MEMORYRELATED_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYRELATED_JSON=false

# Filter mode (explicit, inferred, mixed)
export VAULTMIND_APP_MEMORYRELATED_MODE=mixed

# Path to vault root
export VAULTMIND_APP_MEMORYCONTEXTPACK_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYCONTEXTPACK_JSON=false

# Token budget
export VAULTMIND_APP_MEMORYCONTEXTPACK_BUDGET=4096

# BFS traversal depth (1 = direct neighbors only)
export VAULTMIND_APP_MEMORYCONTEXTPACK_DEPTH=1

# Max context items (0 = unlimited)
export VAULTMIND_APP_MEMORYCONTEXTPACK_MAX_ITEMS=0

# Slim frontmatter (type, title, status only)
export VAULTMIND_APP_MEMORYCONTEXTPACK_SLIM=false

# Path to vault root
export VAULTMIND_APP_MEMORYSUMMARIZE_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYSUMMARIZE_JSON=false

# Comma-separated note IDs (alternative to positional args)
export VAULTMIND_APP_MEMORYSUMMARIZE_IDS=

# Include body text excerpts
export VAULTMIND_APP_MEMORYSUMMARIZE_INCLUDE_BODY=false

# Max body chars per note (0 = full)
export VAULTMIND_APP_MEMORYSUMMARIZE_MAX_BODY_LEN=0

# Path to vault root
export VAULTMIND_APP_MEMORYLINKS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYLINKS_JSON=false

# Filter by edge type
export VAULTMIND_APP_MEMORYLINKS_EDGE_TYPE=

# Show only outbound edges
export VAULTMIND_APP_MEMORYLINKS_OUT=false

# Show only inbound edges (backlinks)
export VAULTMIND_APP_MEMORYLINKS_IN=false

# Show both inbound and outbound edges (default)
export VAULTMIND_APP_MEMORYLINKS_BOTH=false

# Path to vault root
export VAULTMIND_APP_MEMORYNEIGHBORS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYNEIGHBORS_JSON=false

# Maximum traversal depth
export VAULTMIND_APP_MEMORYNEIGHBORS_DEPTH=1

# Minimum edge confidence (low, medium, high)
export VAULTMIND_APP_MEMORYNEIGHBORS_MIN_CONFIDENCE=high

# Maximum nodes to return
export VAULTMIND_APP_MEMORYNEIGHBORS_MAX_NODES=50

# Path to vault root
export VAULTMIND_APP_MEMORYPACK_VAULT=.

# Output in JSON format
export VAULTMIND_APP_MEMORYPACK_JSON=false

# Token budget
export VAULTMIND_APP_MEMORYPACK_BUDGET=4096

# BFS traversal depth (1 = direct neighbors only)
export VAULTMIND_APP_MEMORYPACK_DEPTH=1

# Max context items (0 = unlimited)
export VAULTMIND_APP_MEMORYPACK_MAX_ITEMS=0

# Cap each note's contribution at N tokens, preferring its decision-bearing passage — the Principle section where a note has one, else its opening prose. 0 = off, which also means a note larger than the remaining budget contributes no text at all while the pack still counts it
export VAULTMIND_APP_MEMORYPACK_EXCERPT=0

# Slim frontmatter (type, title, status only)
export VAULTMIND_APP_MEMORYPACK_SLIM=false

# Path to vault root
export VAULTMIND_APP_NOTE_VAULT=.

# Output in JSON format
export VAULTMIND_APP_NOTE_JSON=false

# Omit body, headings, blocks
export VAULTMIND_APP_NOTE_FRONTMATTER_ONLY=false

# Path to vault root
export VAULTMIND_APP_NOTECREATE_VAULT=.

# Output in JSON format
export VAULTMIND_APP_NOTECREATE_JSON=false

# Note type (required)
export VAULTMIND_APP_NOTECREATE_TYPE=

# Body text (overrides template body)
export VAULTMIND_APP_NOTECREATE_BODY=

# Stage and commit
export VAULTMIND_APP_NOTECREATE_COMMIT=false

# Default message to display for the ping command
export VAULTMIND_APP_PING_OUTPUT_MESSAGE=Hello World!

# Text color for ping command output (white, red, green, blue, cyan, yellow, magenta)
export VAULTMIND_APP_PING_OUTPUT_COLOR=green

# Enable interactive UI for the ping command
export VAULTMIND_APP_PING_UI=true

# Path to the vault root directory
export VAULTMIND_APP_RESOLVE_VAULT=.

# Output in JSON format
export VAULTMIND_APP_RESOLVE_JSON=false

# Path to vault root
export VAULTMIND_APP_SCHEMA_VAULT=.

# Output in JSON format
export VAULTMIND_APP_SCHEMA_JSON=false

# Path to vault root
export VAULTMIND_APP_SEARCH_VAULT=.

# Output in JSON format
export VAULTMIND_APP_SEARCH_JSON=false

# Maximum results to return
export VAULTMIND_APP_SEARCH_LIMIT=20

# Skip first N results
export VAULTMIND_APP_SEARCH_OFFSET=0

# Filter by note type
export VAULTMIND_APP_SEARCH_TYPE=

# Filter by tag
export VAULTMIND_APP_SEARCH_TAG=

# Search mode: keyword, semantic, or hybrid
export VAULTMIND_APP_SEARCH_MODE=keyword

# Path to vault root
export VAULTMIND_APP_SELF_VAULT=.

# Max rows per section (recent/hot/stale)
export VAULTMIND_APP_SELF_LIMIT=10

# Path to vault root
export VAULTMIND_APP_VAULTSTATUS_VAULT=.

# Output in JSON format
export VAULTMIND_APP_VAULTSTATUS_JSON=false

```
