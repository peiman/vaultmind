# Security Policy

## Supported versions

VaultMind is pre-1.0. Security fixes land on `main` and ship in the next patch
release; only the latest release is supported. Upgrade with
`go install github.com/peiman/vaultmind@latest` or the latest prebuilt archive.

## Reporting a vulnerability

**Please do not open a public issue for a vulnerability.**

Report it privately through GitHub:
[**Report a vulnerability**](https://github.com/peiman/vaultmind/security/advisories/new)
(the repository's *Security* tab → *Report a vulnerability*). Only the
maintainer sees it.

Useful to include: the version (`vaultmind version`), the command or hook
involved, a minimal vault or input that reproduces it, and what an attacker
gains.

You will get an acknowledgement within a few days. Fixes ship as a patch
release with an advisory that credits you, unless you'd rather stay anonymous.

## What VaultMind trusts, and what it doesn't

VaultMind is a local CLI: one binary, run by you, over folders of Markdown.
The cases that matter:

- **A vault you did not write** (a cloned repo, a collaborator's vault, an
  imported docs folder) is untrusted input. Reads and writes stay inside the
  vault root, and symlinks are never followed out of a vault: `index`,
  `doctor`, the mutation commands and `import` refuse them. A way to make
  VaultMind read or write outside a vault from vault content is a
  vulnerability.
- **Hooks** installed by `vaultmind hooks install` run on your agent's
  events. They query your vaults and never approve a tool call on the agent's
  behalf. A hook that grants a permission, or runs vault content as a
  command, is a vulnerability.
- **Network.** VaultMind sends nothing about you or your vaults. The only
  network calls are `doctor`'s daily version check (opt out with
  `VAULTMIND_NO_UPDATE_CHECK=1`) and the embedding-model download that
  `index --embed` makes the first time it needs a model. See the README's
  "The one network call" and "The local usage log".

Known gap: the embedding-model download is not yet checksum-verified or pinned
to a revision ([#192](https://github.com/peiman/vaultmind/issues/192)).

## Verifying a release

Release archives are signed keylessly with Sigstore from this repository's
GitHub Actions. To verify a prebuilt ORT archive:

```bash
cosign verify-blob \
  --bundle vaultmind_vX.Y.Z_darwin_arm64_ort.tar.gz.sigstore.json \
  --certificate-identity-regexp '^https://github.com/peiman/vaultmind/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  vaultmind_vX.Y.Z_darwin_arm64_ort.tar.gz
```

The other archives are listed in `checksums.txt`, signed the same way
(`checksums.txt.sigstore.json`): verify the checksums file, then check the
archive against it. Each release also carries an SPDX SBOM.

## Automated checks

`task check` runs secret scanning (gitleaks), static analysis (semgrep),
Go vulnerability scanning (govulncheck), a grype scan of the SBOM, and
dependency integrity and licence checks. CI adds a Trivy scan; fuzz tests run
weekly; Dependabot proposes dependency updates.
