# Security Policy

The BranchBase team and community take the security and integrity of developer environments seriously.

## Supported Versions

Security fixes are applied to the active development branch (`main`) and released as patch versions.

| Version | Supported |
| :--- | :--- |
| `0.3.x` | Yes |
| `0.2.x` | No (Superseded) |
| `0.1.x` | No (Superseded) |
| `< 0.1.0` | No |

---

## Reporting a Vulnerability Privately

If you discover a security vulnerability or sensitive data leak in BranchBase, **please do not disclose it publicly** in issues, discussions, or social media.

Please submit a report privately through:
[**GitHub Private Vulnerability Reporting**](https://github.com/oscarbol09/branchbase/security/advisories/new)

Alternatively, contact the lead maintainer directly at:
`omaderabolano@correo.unicordoba.edu.co`

### What to Include in Your Report
To help us triage and resolve the issue quickly, please include:
- A clear description of the vulnerability and its potential impact.
- Detailed step-by-step reproduction steps or a minimal proof of concept (PoC).
- Affected components (e.g., Transparent Proxy, Git Resolver, PostgreSQL Driver).
- Any proposed remediation or fix, if available.

### What to Expect
- **Initial Acknowledgment:** Within 48 hours.
- **Assessment & Triage:** Within 5 business days, including a severity score (CVSS).
- **Remediation & Patch:** Coordinated disclosure with the reporter once a verified patch is ready.

---

## Security Design Principles in BranchBase

1. **Localhost Isolation:** The BranchBase TCP proxy binds to `127.0.0.1` by default. Set `proxy.listen_host` deliberately to expose it on another interface. Startup packets have a deadline and concurrent connections are capped.
2. **Non-Destructive Operations:** The configured default Git branch maps to the base database (including custom branch names), and the driver refuses to delete that database.
3. **No Product Telemetry:** BranchBase does not send telemetry, metrics, or proxy query logs to an external service. If a team configures an external Risk Gate provider (such as Jev/OpenRouter), migration SQL and schema context may be sent to that provider for classification; offline mode avoids those requests. Proxy routing is decided locally from the active Git branch.

---

## Good-Faith Security Research

We welcome security researchers acting in good faith. Please avoid accessing, modifying, or deleting data that does not belong to you, disrupting services, or degrading other users' experience.
