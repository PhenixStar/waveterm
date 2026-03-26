// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wconfig"
)

// ANSI color codes
const (
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiReset  = "\033[0m"
)

// Finding represents a single validation result
type Finding struct {
	Severity string `json:"severity"` // ERROR | WARN | INFO
	File     string `json:"file"`
	Path     string `json:"path"`
	RuleID   string `json:"rule_id"`
	Message  string `json:"message"`
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate Wave Terminal configuration files",
	Long:  "Validate settings.json, widgets.json, and connections.json for type errors, invalid enums, and bad ranges.",
	RunE:  configValidateRun,
}

var configValidateJsonFlag bool
var configValidateFile string

func init() {
	configValidateCmd.Flags().BoolVar(&configValidateJsonFlag, "json", false, "output results as JSON")
	configValidateCmd.Flags().StringVar(&configValidateFile, "file", "", "validate only a specific file (settings.json, widgets.json, connections.json)")
	rootCmd.AddCommand(configValidateCmd)
}

func configValidateRun(cmd *cobra.Command, args []string) error {
	var findings []Finding

	runFile := func(name string, fn func(map[string]any) []Finding) {
		raw, cerrs := wconfig.ReadWaveHomeConfigFile(name)
		for _, ce := range cerrs {
			findings = append(findings, Finding{
				Severity: "ERROR",
				File:     name,
				Path:     "",
				RuleID:   "SYN",
				Message:  ce.Err,
			})
		}
		if raw != nil {
			findings = append(findings, fn(map[string]any(raw))...)
		}
	}

	switch configValidateFile {
	case "settings.json":
		runFile("settings.json", validateSettings)
	case "widgets.json":
		runFile("widgets.json", validateWidgets)
	case "connections.json":
		runFile("connections.json", validateConnections)
	case "":
		runFile("settings.json", validateSettings)
		runFile("widgets.json", validateWidgets)
		runFile("connections.json", validateConnections)
	default:
		return fmt.Errorf("unknown file %q — must be settings.json, widgets.json, or connections.json", configValidateFile)
	}

	if configValidateJsonFlag {
		enc := json.NewEncoder(WrappedStdout)
		enc.SetIndent("", "  ")
		enc.Encode(findings)
	} else {
		printFindings(findings)
	}

	// exit 1 if any ERRORs
	for _, f := range findings {
		if f.Severity == "ERROR" {
			WshExitCode = 1
			return nil
		}
	}
	return nil
}

func printFindings(findings []Finding) {
	isTty := getIsTty()
	errCount, warnCount, infoCount := 0, 0, 0

	for _, f := range findings {
		loc := f.File
		if f.Path != "" {
			loc = f.File + " > " + f.Path
		}
		var prefix string
		switch f.Severity {
		case "ERROR":
			errCount++
			if isTty {
				prefix = ansiRed + "[ERROR]" + ansiReset
			} else {
				prefix = "[ERROR]"
			}
		case "WARN":
			warnCount++
			if isTty {
				prefix = ansiYellow + "[WARN] " + ansiReset
			} else {
				prefix = "[WARN] "
			}
		default:
			infoCount++
			if isTty {
				prefix = ansiBlue + "[INFO] " + ansiReset
			} else {
				prefix = "[INFO] "
			}
		}
		WriteStdout("%s %s: %s\n", prefix, loc, f.Message)
	}

	if len(findings) == 0 {
		WriteStdout("No issues found.\n")
		return
	}
	WriteStdout("\nSummary: %d error(s), %d warning(s), %d info\n", errCount, warnCount, infoCount)
}
