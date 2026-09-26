# BranchBase Risk Gate 🛡️

> Automated schema migration risk classification with typed decision intelligence and offline fallback.

BranchBase Risk Gate (`branchbase risk`) protects local and preview database branches from accidental, destructive schema migrations before they are applied during `git checkout` or `branchbase switch`.

---

## Key Principles

1. **Deterministic Decisions, Configurable Actions:** The classifier determines the technical risk and confidence, while `.branchbase/risk-policy.yml` decides whether to allow, warn, confirm, or block.
2. **Offline-First Resilience:** Works completely offline using deterministic AST/regex heuristics. Remote decision models (TypeSafe Jev / OpenRouter) enrich analysis with calibrated probabilities when network and keys are available.
3. **Zero-Latency Cache:** Results are cached by `SHA256(migration_sql + table_schema_signature)` in `.branchbase/cache/risk_cache.json`, ensuring subsequent checkouts run in sub-millisecond time.
4. **Foreign Key Awareness:** Introspects the active database catalog to verify if altered/dropped columns break inbound foreign keys across other tables.

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
