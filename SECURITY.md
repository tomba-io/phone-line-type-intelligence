# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x     | Yes                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public GitHub issue.
2. Use [GitHub Security Advisories](https://github.com/tomba-io/phone-line-type-intelligence/security/advisories/new) to report privately.
3. Alternatively, email **security@tomba.io** with details.

We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for confirmed vulnerabilities.

## Scope

The following are considered security issues:

- Panics or crashes on untrusted input (denial of service)
- Memory safety violations
- Injection vulnerabilities in CLI commands
- Unintended data exposure

The following are **not** security issues:

- Data accuracy (misclassified numbers due to porting or stale data) -- see ACCURACY.md
- Geographic blocking of upstream data sources (NANPA, IFT)

## Built-in Audit

This project includes a security audit command:

```bash
lti audit --verbose
```

It validates table integrity, input sanitisation, concurrent safety, and data checksums.
