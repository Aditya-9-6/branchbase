# How We Built a Jev-Powered Migration Risk Gate for BranchBase

> Bringing typed decision intelligence and sub-millisecond safety gates to Git-driven database branching workflows.

---

## The Problem: Ephemeral Branching Meets Destructive Migrations

At **BranchBase**, our core mission is simple: provide instant, isolated database branches triggered natively by Git workflow events (`git checkout`, `git switch`, `post-merge`). When an engineer switches to `feature/auth-redesign`, BranchBase transparently provisions an isolated copy of the database and redirects the local proxy.

However, as developer adoption grew, a critical edge case emerged: **Destructive schema migrations applied during checkout.**

Consider a developer switching branches:
```bash
git checkout feature/deprecate-email-column
# -> post-checkout triggers local migration runner
# -> executes: ALTER TABLE users DROP COLUMN email;
```

If `users.email` is actively referenced by foreign key constraints across `orders`, `billing`, and `notifications`, applying this migration in a shared staging environment or local branch destroys relational integrity and leaves the developer with broken state.

Traditional linters only inspect static syntax. They lack **runtime schema context** (which tables reference what) and **probabilistic reasoning** about migration intent.

---

## Enter TypeSafe Jev & System One Decision Models

Instead of relying solely on shallow regex or deploying a heavy, slow LLM agent that hallucinates unstructured text, we integrated **TypeSafe Jev** (`typesafe/jev-1.13`) alongside a deterministic offline heuristic engine.

Jev provides **typed decision primitives** (*Choice* and *Noul*) with calibrated probabilities:

```
                  ┌────────────────────────────────────────────────────────┐
                  │                    Git Checkout Hook                   │
                  └───────────────────────────┬────────────────────────────┘
                                              │
                                              ▼
                  ┌────────────────────────────────────────────────────────┐
                  │           BranchBase Risk Gate (`bb risk`)             │
                  └───────────────────────────┬────────────────────────────┘
                                              │
                      ┌───────────────────────┴───────────────────────┐
                      ▼                                               ▼
        [ Composite SHA256 Cache ]                       [ Schema Introspector ]
        SHA256(sql + schema_sig)                        information_schema FKs
                      │                                               │
                      └───────────────────────┬───────────────────────┘
                                              ▼
                                 ┌─────────────────────────┐
                                 │   RiskClassifier Engine │
                                 └────────────┬────────────┘
                                              │
                      ┌───────────────────────┴───────────────────────┐
                      ▼                                               ▼
         [ TypeSafe Jev / OpenRouter ]                   [ Deterministic Heuristic ]
         • Typed System One questions                    • Pure Go AST/token stream
         • Calibrated probability score                  • Zero network dependency
                      │                                               │
                      └───────────────────────┬───────────────────────┘
                                              ▼
                                 ┌─────────────────────────┐
                                 │   .branchbase/policy    │
                                 │  (allow/warn/confirm)   │
                                 └─────────────────────────┘
```

---

## Architectural Deep Dive: 4 Key Engineering Decisions

### 1. Zero-Latency Git Hooks via Composite Hashing
A Git hook must execute in `<150ms` to preserve developer ergonomics. An external API call takes `400ms - 1.5s`.

We solved this with a composite cache key:
$$\text{Key} = \text{SHA256}(\text{migration\_sql} \mathbin{\Vert} \text{"::"} \mathbin{\Vert} \text{table\_schema\_signature})$$

If the table schema has not changed and the migration file hash matches, the evaluation is returned in $<1\text{ ms}$ from `.branchbase/cache/risk_cache.json`.

### 2. Live Catalog Introspection (Grounding Decisions in Reality)
Prompting an AI with only `ALTER TABLE users DROP COLUMN email;` is insufficient. The engine cannot know whether `email` is an isolated attribute or the target of 5 foreign key constraints.

Before classification, BranchBase queries the active database:
```sql
SELECT 
    tc.constraint_name,
    tc.table_name AS from_table,
    kcu.column_name AS from_column,
    ccu.table_name AS to_table,
    ccu.column_name AS to_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu 
    ON tc.constraint_name = kcu.constraint_name 
    AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage ccu 
    ON ccu.constraint_name = tc.constraint_name 
    AND ccu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND ccu.table_name = 'users';
```

The resulting state (`"users (id, name, email) — 3 inbound FK references: orders(user_email), billing(email)"`) grounds the decision in absolute fact.

### 3. TTY-Safe Interactive Confirmation
When a `HIGH` risk migration requires confirmation (`confirm` action in `.branchbase/risk-policy.yml`), naive prompts hang inside non-interactive GUI tools (VS Code Source Control, IntelliJ Git, CI/CD).

We built terminal mode detection into the confirmation prompt:
```go
func PromptConfirmation(r io.Reader, w io.Writer, prompt string) (bool, error) {
    if f, ok := r.(*os.File); ok {
        if (f.Stat().Mode() & os.ModeCharDevice) == 0 {
            return false, ErrNonInteractiveTerminal
        }
    }
    // Safe interactive read ...
}
```

If `stdin` is not a TTY, the risk gate fails safely with a clean error instructing the user to supply `--force` or run from a terminal.

### 4. Pure Go Portability (No CGO Bloat)
Rather than pulling in C-based parsers that complicate cross-compilation matrices for macOS (ARM64/Intel), Linux, and Windows, our heuristic classifier is built in 100% pure Go. It supports PostgreSQL, MySQL, and SQLite dialects out of the box with zero runtime C dependencies.

---

## Policy as Code: `.branchbase/risk-policy.yml`

Risk evaluation is strictly decoupled from repo policy:

```yaml
version: 1

on_risk:
  LOW: allow
  MEDIUM: warn
  HIGH: confirm
  CRITICAL: block

confidence_threshold: 0.75
on_jev_unavailable: warn

always_critical:
  - "DROP TABLE"
  - "TRUNCATE"
  - "DROP DATABASE"

exclude_paths:
  - "migrations/seeds/**"

report_format: pretty
save_report: .branchbase/last-risk-report.json
```

---

## Try It in BranchBase v0.4.0

```bash
# Analyze a migration locally
branchbase risk analyze migrations/003_drop_users.sql

# Evaluate pending migrations against policy
branchbase risk check --branch feature/cleanup

# Start the interactive dashboard with Risk Gate badges
branchbase tui
```

Check out the code on GitHub: [github.com/oscarbol09/branchbase](https://github.com/oscarbol09/branchbase)
