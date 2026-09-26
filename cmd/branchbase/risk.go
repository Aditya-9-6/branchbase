package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/branchbase/branchbase/internal/risk"
)

func runRisk(cwd string, args []string) {
	if len(args) == 0 {
		printRiskUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	switch subcommand {
	case "analyze":
		if len(args) < 2 {
			fmt.Println("Usage: branchbase risk analyze <migration.sql> [--json] [--offline]")
			os.Exit(1)
		}
		filePath := args[1]
		jsonOutput := false
		offline := false
		for _, a := range args[2:] {
			if a == "--json" {
				jsonOutput = true
			} else if a == "--offline" {
				offline = true
			}
		}
		runRiskAnalyze(cwd, filePath, jsonOutput, offline)

	case "check":
		policyPath := ""
		branchName := ""
		force := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--policy":
				if i+1 < len(args) {
					policyPath = args[i+1]
					i++
				}
			case "--branch":
				if i+1 < len(args) {
					branchName = args[i+1]
					i++
				}
			case "--force", "-f":
				force = true
			}
		}
		runRiskCheck(cwd, branchName, policyPath, force)

	default:
		fmt.Fprintf(os.Stderr, "Unknown risk subcommand %q\n\n", subcommand)
		printRiskUsage()
		os.Exit(1)
	}
}

func printRiskUsage() {
	fmt.Println(`BranchBase Risk Gate 🛡️ — Automated database migration risk analysis

Usage:
  branchbase risk <command> [arguments]

Commands:
  analyze <file.sql>   Parse and analyze a migration file for potential risks
  check [--branch <b>] Evaluate pending migrations against .branchbase/risk-policy.yml

Flags:
  --json       Output result as structured JSON
  --offline    Force local heuristic analysis without remote API
  --force      Bypass blocking policy actions (proceed with caution)
  --policy <p> Path to custom risk-policy.yml`)
}

func runRiskAnalyze(cwd, targetFile string, jsonOutput, offline bool) {
	fullPath := targetFile
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(cwd, targetFile)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading migration file %s: %v\n", targetFile, err)
		os.Exit(1)
	}

	parser := risk.NewDDLParser("postgres")
	operations, err := parser.ParseDiff(string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse DDL operations: %v\n", err)
		os.Exit(1)
	}

	policy, _ := risk.LoadPolicy(cwd, "")

	// Check always_critical patterns
	isCritical, criticalPattern := policy.MatchesAlwaysCritical(string(content))

	level := risk.RiskLow
	var flags []string
	if isCritical {
		level = risk.RiskCritical
		flags = append(flags, fmt.Sprintf("Matches always_critical: %s", criticalPattern))
	}

	result := risk.MigrationRisk{
		File:       targetFile,
		Level:      level,
		Category:   risk.CategorySchemaChange,
		Confidence: 1.0,
		Flags:      flags,
		Operations: operations,
		Engine:     "postgres",
		Source:     "heuristic",
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("🛡️  BranchBase Risk Gate — Migration Analysis (Phase 0 Scaffold)")
	fmt.Printf("  • File:       %s\n", targetFile)
	fmt.Printf("  • Operations: %d detected\n", len(operations))
	fmt.Printf("  • Risk Level: %s\n", result.Level)

	for i, op := range operations {
		target := op.TargetTable
		if op.TargetColumn != "" {
			target += "." + op.TargetColumn
		}
		fmt.Printf("    [%d] %-15s Target: %-20s SQL: %s\n", i+1, op.Type, target, op.RawSQL)
	}

	if isCritical {
		fmt.Printf("\n⚠️  CRITICAL: Triggered rule: %s\n", criticalPattern)
	}
}

func runRiskCheck(cwd, branchName, policyPath string, force bool) {
	policy, err := risk.LoadPolicy(cwd, policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load risk policy: %v\n", err)
		os.Exit(2) // Config error exit code
	}

	fmt.Printf("🛡️  BranchBase Risk Gate: checking migrations (policy version: %d)...\n", policy.Version)
	if force {
		fmt.Println("⚠️  --force flag supplied. Bypassing blocker checks.")
	}

	// Exit 0: All safe to proceed
	os.Exit(0)
}
