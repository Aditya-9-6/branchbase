# BranchBase Risk Gate 🛡️

> Automated schema migration risk classification with typed decision intelligence and offline fallback.

BranchBase Risk Gate (`branchbase risk`) assesses changed SQL migrations against repository policy. The asynchronous Git hook uses the result before it prewarms a branch database; `branchbase risk check` provides the synchronous exit status for local scripts and CI. The Git `post-checkout` hook runs after checkout and cannot cancel the checkout itself.

---

## Key Principles

1. **Deterministic Decisions, Configurable Actions:** The classifier determines the technical risk and confidence, while `.branchbase/risk-policy.yml` decides whether to allow, warn, confirm, or block.
2. **Offline-First Resilience:** Works offline using a lightweight statement splitter and deterministic rules. It is not a full dialect AST parser. SQL statements it cannot classify are escalated to HIGH for manual review. Remote decision models (TypeSafe Jev / OpenRouter) enrich analysis when network and keys are available.
3. **Local Analysis Cache:** Results are cached by `SHA256(migration_sql + table_schema_signature)` in `.branchbase/cache/risk_cache.json`; cache hits avoid repeating classification for the same SQL and schema signature.
4. **Foreign Key Awareness:** When the configured active database is reachable, BranchBase introspects its catalog for inbound foreign keys. If metadata is unavailable, a dropped column is at least HIGH because FK impact is unknown.

---

## Configuration (`.branchbase/risk-policy.yml`)

```yaml
version: 1

# Actions: allow | warn | confirm | block
on_risk:
  LOW: allow
  MEDIUM: warn
  HIGH: confirm
  CRITICAL: block

# Auto-escalate if confidence is below threshold
confidence_threshold: 0.75

# Fallback when AI provider is unreachable
on_jev_unavailable: warn

# Immediate critical rules
always_critical:
  - "DROP TABLE"
  - "TRUNCATE"
  - "DROP DATABASE"
  - "DROP SCHEMA"

exclude_paths:
  - "migrations/seeds/**"
  - "migrations/test_data/**"

report_format: pretty
save_report: .branchbase/last-risk-report.json
```

---

## CLI Usage

### Analyze a Migration File
```bash
# Terminal human-readable report
branchbase risk analyze migrations/003_drop_email.sql

# Structured JSON output
branchbase risk analyze migrations/003_drop_email.sql --json

# Force offline evaluation
branchbase risk analyze migrations/003_drop_email.sql --offline
```

### Check Pending Migrations
```bash
branchbase risk check --branch feature/drop-email
```

### Exit Codes
- `0`: Migration permitted (LOW / MEDIUM / confirmed HIGH).
- `1`: Migration blocked (CRITICAL or user rejected confirmation).
- `2`: Configuration or CLI syntax error.
- `3`: Internal or provider error (when `on_jev_unavailable: block`).
