# Security Policy

## Reporting a vulnerability

Please do not disclose suspected vulnerabilities in a public issue. Use GitHub's
private vulnerability reporting for `fd1az/nats-tui` so reports, reproduction
details, and fixes remain private until a coordinated release is available.

Include the affected version, operating system, impact, reproduction steps, and
any proposed mitigation. Reports involving credential disclosure, arbitrary
command execution, path traversal, or authorization bypass receive priority.

## Automated controls

Every push to `main`, pull request, and weekly scheduled run executes:

- `govulncheck` for vulnerabilities reachable through the compiled Go call graph;
- `gosec` for Go-specific static security analysis;
- CodeQL's `security-and-quality` query suite;
- dependency review for vulnerable packages introduced by pull requests.

Release archives are accompanied by SHA-256 checksums. The direct shell and
PowerShell installers verify those checksums before replacing an executable.

