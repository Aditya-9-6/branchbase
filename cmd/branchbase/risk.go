package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/git"
	"github.com/branchbase/branchbase/internal/risk"
)

func runRisk(cwd string, args []string) {
	if len(args) == 0 {
		printRiskUsage()
		os.Exit(2)
	}

	subcommand := args[0]
	switch subcommand {
	case "analyze":
		if len(args) < 2 {
			fmt.Println("Usage: branchbase risk analyze <migration.sql> [--json] [--offline] [--github]")
			os.Exit(2)
		}
		filePath := args[1]
		jsonOutput := false
		githubOutput := false
		offline := false
		for _, a := range args[2:] {
			if a == "--json" {
				jsonOutput = true
			} else if a == "--github" {
				githubOutput = true
			} else if a == "--offline" {
				offline = true
			}
		}
		runRiskAnalyze(cwd, filePath, jsonOutput, githubOutput, offline)

	case "check":
		policyPath := ""
		branchName := ""
		force := false
		offline := false
		jsonOutput := false
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
			case "--offline":
				offline = true
			case "--json":
				jsonOutput = true
			}
		}
		runRiskCheck(cwd, branchName, policyPath, force, offline, jsonOutput)

	default:
		fmt.Fprintf(os.Stderr, "Unknown risk subcommand %q\n\n", subcommand)
		printRiskUsage()
		os.Exit(2)
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
  --github     Output result as GitHub Action annotation commands
  --offline    Force local heuristic analysis without remote API
  --force      Bypass blocking policy actions (proceed with caution)
  --policy <p> Path to custom risk-policy.yml`)
}

func resolveIntrospector(cwd string, cfg *config.Config) risk.SchemaIntrospector {
	if cfg == nil {
		return risk.NewMockIntrospector()
	}

	drv, err := getDriverForConfig(cfg, true)
	if err != nil {
		return risk.NewMockIntrospector()
	}

	if drv.Name() == "postgres" {
		if pgDrv, ok := drv.(interface{ DB() interface{} }); ok {
			if dbHandle, ok := pgDrv.DB().(interface {
				QueryContext(ctx context.Context, query string, args ...interface{}) (*risk.PostgresIntrospector, error)
			}); ok {
				_ = dbHandle
			}
		}
	}

	return risk.NewMockIntrospector()
}

func runRiskAnalyze(cwd, targetFile string, jsonOutput, githubOutput, offline bool) {
	fullPath := targetFile
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(cwd, targetFile)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading migration file %s: %v\n", targetFile, err)
		os.Exit(2)
	}

	cfg, _ := config.LoadConfig(cwd)
	engine := "postgres"
	if cfg != nil && cfg.Driver != "" {
		engine = cfg.Driver
	}

	analyzer, err := risk.NewRiskAnalyzer(risk.AnalyzeOptions{
		RepoRoot:     cwd,
		Engine:       engine,
		Offline:      offline,
		Introspector: resolveIntrospector(cwd, cfg),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize risk analyzer: %v\n", err)
		os.Exit(3)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	riskResult, err := analyzer.AnalyzeSQL(ctx, targetFile, string(content))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Risk analysis failed: %v\n", err)
		os.Exit(3)
	}

	report := analyzer.EvaluateReport([]risk.MigrationRisk{*riskResult}, "")
	_ = analyzer.SaveReport(report)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}

	if githubOutput {
		fmt.Print(risk.FormatGitHubAnnotations(report))
		return
	}

	fmt.Print(risk.FormatPrettyTerminal(report))
}

func runRiskCheck(cwd, branchName, policyPath string, force, offline, jsonOutput bool) {
	if branchName == "" {
		resolved, err := git.ResolveCurrentBranch(cwd)
		if err == nil {
			branchName = resolved
		} else {
			branchName = "current"
		}
	}

	cfg, _ := config.LoadConfig(cwd)
	engine := "postgres"
	if cfg != nil && cfg.Driver != "" {
		engine = cfg.Driver
	}

	analyzer, err := risk.NewRiskAnalyzer(risk.AnalyzeOptions{
		RepoRoot:     cwd,
		Engine:       engine,
		Offline:      offline,
		Force:        force,
		PolicyPath:   policyPath,
		Introspector: resolveIntrospector(cwd, cfg),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize risk analyzer: %v\n", err)
		os.Exit(2)
	}

	// Discover migration files in migrations/ directory if present
	var migrationFiles []string
	candidates := []string{"migrations", "db/migrations", "sql", "migration"}
	for _, dir := range candidates {
		fullDir := filepath.Join(cwd, dir)
		if entries, err := os.ReadDir(fullDir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
					migrationFiles = append(migrationFiles, filepath.Join(dir, entry.Name()))
				}
			}
			if len(migrationFiles) > 0 {
				break
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var assessedRisks []risk.MigrationRisk
	for _, f := range migrationFiles {
		fullPath := filepath.Join(cwd, f)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		res, err := analyzer.AnalyzeSQL(ctx, f, string(content))
		if err != nil {
			continue
		}
		assessedRisks = append(assessedRisks, *res)
	}

	report := analyzer.EvaluateReport(assessedRisks, branchName)
	_ = analyzer.SaveReport(report)

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else if len(assessedRisks) > 0 {
		fmt.Print(risk.FormatPrettyTerminal(report))
	}

	if report.Blocked {
		if force {
			fmt.Println("⚠️  CRITICAL risk detected, but proceeding due to --force flag.")
			os.Exit(0)
		}
		fmt.Println("❌ Migration blocked by risk gate policy. Use 'branchbase risk analyze <file>' or pass --force.")
		os.Exit(1)
	}

	if report.RequiresConfirm && !force {
		confirmed, err := risk.PromptConfirmation(os.Stdin, os.Stdout, "High risk detected. Proceed with migration?")
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
			os.Exit(1)
		}
		if !confirmed {
			fmt.Println("Aborted by user.")
			os.Exit(1)
		}
	}

	// Exit 0: Proceed
	os.Exit(0)
}
