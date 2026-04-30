# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in agent-gogo, please report it responsibly.

**Do not** file a public GitHub issue for security vulnerabilities.

Instead, send a detailed report to [su.keyu@hotmail.com](mailto:su.keyu@hotmail.com) including:

- A description of the vulnerability
- Steps to reproduce the issue
- The affected version or commit
- Any possible mitigations you have identified

We aim to acknowledge reports within 48 hours and provide a substantive response within 7 days.

## Scope

The following are in scope for security reports:

- Vulnerabilities in the Go runtime core (`internal/`)
- SQL injection or data corruption in the SQLite store
- Authentication or authorization bypass in the Web Console
- Arbitrary code execution through tool runtime or shell execution
- Sensitive data exposure through logs or API responses

The following are out of scope:

- Issues in dependencies (report to the upstream project)
- Social engineering attacks
- Denial of service without a concrete exploit
