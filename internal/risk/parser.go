package risk

import (
	"regexp"
	"strings"
)

// DDLParser extracts structured DDL operations from raw SQL migration contents.
type DDLParser struct {
	Engine string
}

// NewDDLParser creates a DDLParser for the given database engine (e.g. "postgres", "mysql", "sqlite").
func NewDDLParser(engine string) *DDLParser {
	if engine == "" {
		engine = "postgres"
	}
	return &DDLParser{Engine: strings.ToLower(engine)}
}

var (
	reDropTable     = regexp.MustCompile(`(?i)^\s*DROP\s+TABLE(?:\s+IF\s+EXISTS)?\s+([^\s;,]+)`)
	reTruncate      = regexp.MustCompile(`(?i)^\s*TRUNCATE(?:\s+TABLE)?\s+([^\s;,]+)`)
	reDropColumn    = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+DROP\s+(?:COLUMN\s+)?([^\s;,]+)`)
	reAlterColType  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+ALTER\s+(?:COLUMN\s+)?([^\s;]+)\s+TYPE\s+([^;,]+)`)
	reModifyColumn  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+MODIFY\s+(?:COLUMN\s+)?([^\s;,]+)`)
	reDropConstraint= regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+DROP\s+CONSTRAINT\s+([^\s;,]+)`)
	reDropIndex     = regexp.MustCompile(`(?i)^\s*DROP\s+INDEX(?:\s+IF\s+EXISTS)?\s+([^\s;,]+)`)
	reCreateIndex   = regexp.MustCompile(`(?i)^\s*CREATE(?:\s+UNIQUE)?\s+INDEX(?:\s+CONCURRENTLY)?\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s;]+)\s+ON\s+([^\s;(]+)`)
	reAddColumn     = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+ADD\s+(?:COLUMN\s+)?([^\s;,]+)`)
	reCreateTable   = regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([^\s;(]+)`)
	reAddConstraint = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+ADD\s+CONSTRAINT\s+([^\s;,]+)`)
	reRenameColumn  = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+RENAME\s+(?:COLUMN\s+)?([^\s;]+)\s+TO\s+([^\s;,]+)`)
	reRenameTable   = regexp.MustCompile(`(?i)^\s*ALTER\s+TABLE\s+([^\s;]+)\s+RENAME\s+TO\s+([^\s;,]+)`)
)

// StripCommentsAndTransactions cleans SQL content by removing comments and transaction wrappers.
func StripCommentsAndTransactions(sql string) string {
	// Remove block comments /* ... */
	reBlockComments := regexp.MustCompile(`(?s)/\*.*?\*/`)
	sql = reBlockComments.ReplaceAllString(sql, "")

	// Remove line comments -- ...
	reLineComments := regexp.MustCompile(`--[^\r\n]*`)
	sql = reLineComments.ReplaceAllString(sql, "")

	return strings.TrimSpace(sql)
}

// SplitStatements splits a SQL string by semicolon into individual executable statements.
func SplitStatements(sql string) []string {
	cleaned := StripCommentsAndTransactions(sql)
	if cleaned == "" {
		return nil
	}

	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inDollarQuote := false
	var dollarTag string

	runes := []rune(cleaned)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]

		// Handle PostgreSQL Dollar Quotes $$ or $tag$
		if !inSingleQuote && !inDoubleQuote {
			if r == '$' {
				if inDollarQuote {
					// Check if closing tag matches
					tagEnd := i
					for tagEnd < n && runes[tagEnd] != '$' {
						tagEnd++
					}
					// If we find closing delimiter
					if tagEnd < n && runes[tagEnd] == '$' {
						candidateTag := string(runes[i : tagEnd+1])
						if candidateTag == dollarTag {
							inDollarQuote = false
							dollarTag = ""
							current.WriteString(candidateTag)
							i = tagEnd
							continue
						}
					}
				} else {
					// Check opening tag
					tagEnd := i + 1
					for tagEnd < n && runes[tagEnd] != '$' && tagEnd-i < 30 {
						tagEnd++
					}
					if tagEnd < n && runes[tagEnd] == '$' {
						dollarTag = string(runes[i : tagEnd+1])
						inDollarQuote = true
						current.WriteString(dollarTag)
						i = tagEnd
						continue
					}
				}
			}
		}

		if !inDollarQuote {
			if r == '\'' && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			} else if r == '"' && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		}

		if r == ';' && !inSingleQuote && !inDoubleQuote && !inDollarQuote {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" && !isIgnoredStatement(stmt) {
				statements = append(statements, stmt)
			}
			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}

	stmt := strings.TrimSpace(current.String())
	if stmt != "" && !isIgnoredStatement(stmt) {
		statements = append(statements, stmt)
	}

	return statements
}

func isIgnoredStatement(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	switch upper {
	case "BEGIN", "COMMIT", "ROLLBACK", "BEGIN TRANSACTION", "COMMIT TRANSACTION":
		return true
	default:
		return strings.HasPrefix(upper, "SAVEPOINT ")
	}
}

// CleanIdentifier strips quotes or schema prefixes (e.g. `public."users"` -> `users`).
func CleanIdentifier(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'` + "`")
	if idx := strings.LastIndex(raw, "."); idx != -1 {
		raw = raw[idx+1:]
	}
	return strings.Trim(raw, `"'` + "`")
}

// ParseDiff parses all SQL statements and classifies individual DDL operations.
func (p *DDLParser) ParseDiff(sqlContent string) ([]DDLOperation, error) {
	statements := SplitStatements(sqlContent)
	var operations []DDLOperation

	for idx, stmt := range statements {
		op := p.classifyStatement(stmt, idx+1)
		operations = append(operations, op)
	}

	return operations, nil
}

func (p *DDLParser) classifyStatement(stmt string, line int) DDLOperation {
	trimmed := strings.TrimSpace(stmt)

	// 1. DROP TABLE
	if m := reDropTable.FindStringSubmatch(trimmed); len(m) > 1 {
		return DDLOperation{
			Type:        "DROP_TABLE",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 2. TRUNCATE
	if m := reTruncate.FindStringSubmatch(trimmed); len(m) > 1 {
		return DDLOperation{
			Type:        "TRUNCATE",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 3. DROP COLUMN
	if m := reDropColumn.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:         "DROP_COLUMN",
			TargetTable:  CleanIdentifier(m[1]),
			TargetColumn: CleanIdentifier(m[2]),
			RawSQL:       trimmed,
			LineNumber:   line,
		}
	}

	// 4. ALTER COLUMN TYPE
	if m := reAlterColType.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:         "ALTER_TYPE",
			TargetTable:  CleanIdentifier(m[1]),
			TargetColumn: CleanIdentifier(m[2]),
			RawSQL:       trimmed,
			LineNumber:   line,
		}
	}

	// 5. MODIFY COLUMN (MySQL)
	if m := reModifyColumn.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:         "ALTER_TYPE",
			TargetTable:  CleanIdentifier(m[1]),
			TargetColumn: CleanIdentifier(m[2]),
			RawSQL:       trimmed,
			LineNumber:   line,
		}
	}

	// 6. RENAME COLUMN
	if m := reRenameColumn.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:         "RENAME_COLUMN",
			TargetTable:  CleanIdentifier(m[1]),
			TargetColumn: CleanIdentifier(m[2]),
			RawSQL:       trimmed,
			LineNumber:   line,
		}
	}

	// 7. RENAME TABLE
	if m := reRenameTable.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:        "RENAME_TABLE",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 8. DROP CONSTRAINT
	if m := reDropConstraint.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:        "DROP_CONSTRAINT",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 9. DROP INDEX
	if m := reDropIndex.FindStringSubmatch(trimmed); len(m) > 1 {
		return DDLOperation{
			Type:        "DROP_INDEX",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 10. CREATE INDEX
	if m := reCreateIndex.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:        "ADD_INDEX",
			TargetTable: CleanIdentifier(m[2]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 11. ADD CONSTRAINT
	if m := reAddConstraint.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:        "ADD_CONSTRAINT",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// 12. ADD COLUMN
	if m := reAddColumn.FindStringSubmatch(trimmed); len(m) > 2 {
		return DDLOperation{
			Type:         "ADD_COLUMN",
			TargetTable:  CleanIdentifier(m[1]),
			TargetColumn: CleanIdentifier(m[2]),
			RawSQL:       trimmed,
			LineNumber:   line,
		}
	}

	// 13. CREATE TABLE
	if m := reCreateTable.FindStringSubmatch(trimmed); len(m) > 1 {
		return DDLOperation{
			Type:        "CREATE_TABLE",
			TargetTable: CleanIdentifier(m[1]),
			RawSQL:      trimmed,
			LineNumber:  line,
		}
	}

	// Default fallback generic operation
	return DDLOperation{
		Type:       "GENERIC_DDL",
		RawSQL:     trimmed,
		LineNumber: line,
	}
}
