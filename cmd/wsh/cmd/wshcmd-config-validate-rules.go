// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/wavetermdev/waveterm/pkg/wconfig"
)

// buildSettingsEnumMap extracts jsonschema enum tags from SettingsType at init-time.
// Returns map[json-key] -> []allowed-values
func buildSettingsEnumMap() map[string][]string {
	t := reflect.TypeOf(wconfig.SettingsType{})
	result := make(map[string][]string)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		jsonTag := f.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		key := strings.Split(jsonTag, ",")[0]
		schema := f.Tag.Get("jsonschema")
		if schema == "" {
			continue
		}
		var enums []string
		for _, part := range strings.Split(schema, ",") {
			if strings.HasPrefix(part, "enum=") {
				enums = append(enums, strings.TrimPrefix(part, "enum="))
			}
		}
		if len(enums) > 0 {
			result[key] = enums
		}
	}
	return result
}

// buildSettingsKnownKeys returns set of all json keys in SettingsType
func buildSettingsKnownKeys() map[string]bool {
	t := reflect.TypeOf(wconfig.SettingsType{})
	result := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		jsonTag := f.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		key := strings.Split(jsonTag, ",")[0]
		result[key] = true
	}
	return result
}

var settingsEnumMap = buildSettingsEnumMap()
var settingsKnownKeys = buildSettingsKnownKeys()

var validViews = map[string]bool{
	"term": true, "web": true, "sysinfo": true, "preview": true,
	"waveai": true, "waveconfig": true, "codeedit": true,
	"cpuplot": true, "vdom": true, "tips": true, "help": true, "launcher": true,
}

var validControllers = map[string]bool{"shell": true, "cmd": true}

var validSysinfoTypes = map[string]bool{
	"CPU": true, "Mem": true, "CPU + Mem": true, "All CPU": true,
	"GPU": true, "GPU + Mem": true, "All GPU": true, "CPU + GPU": true,
	"Disk": true, "Net": true, "Disk + Net": true, "All Metrics": true,
	"GPU Temp": true, "GPU Power": true, "Dials": true,
}

func validateSettings(raw map[string]any) []Finding {
	var findings []Finding

	checkRange := func(key string, min, max float64, severity, ruleID string) {
		v, ok := raw[key]
		if !ok {
			return
		}
		f, isNum := toFloat64(v)
		if !isNum {
			findings = append(findings, Finding{Severity: "ERROR", File: "settings.json", Path: key, RuleID: ruleID,
				Message: fmt.Sprintf("expected number, got %T", v)})
			return
		}
		if f < min || f > max {
			findings = append(findings, Finding{Severity: severity, File: "settings.json", Path: key, RuleID: ruleID,
				Message: fmt.Sprintf("value %.4g is out of range [%.4g, %.4g]", f, min, max)})
		}
	}

	// Range checks
	checkRange("term:fontsize", 8, 72, "ERROR", "S01")
	checkRange("editor:fontsize", 8, 72, "ERROR", "S02")
	checkRange("ai:fontsize", 8, 72, "ERROR", "S03")
	checkRange("ai:fixedfontsize", 8, 72, "ERROR", "S04")
	checkRange("window:opacity", 0, 1, "ERROR", "S05")
	checkRange("window:magnifiedblockopacity", 0, 1, "ERROR", "S06")
	checkRange("window:zoom", 0.5, 3.0, "WARN", "S07")
	checkRange("term:scrollback", 0, 100000, "WARN", "S13")
	checkRange("window:tilegapsize", 0, 100, "WARN", "S14")

	if v, ok := raw["autoupdate:intervalms"]; ok {
		f, isNum := toFloat64(v)
		if isNum && f < 60000 {
			findings = append(findings, Finding{Severity: "WARN", File: "settings.json", Path: "autoupdate:intervalms", RuleID: "S15",
				Message: fmt.Sprintf("%.4g ms is very low; recommended minimum is 60000 ms", f)})
		}
	}

	// Enum checks from struct tags
	for key, allowed := range settingsEnumMap {
		v, ok := raw[key]
		if !ok {
			continue
		}
		s, isStr := v.(string)
		if !isStr || s == "" {
			continue
		}
		if !containsStr(allowed, s) {
			findings = append(findings, Finding{Severity: "ERROR", File: "settings.json", Path: key, RuleID: "S08",
				Message: fmt.Sprintf("%q is not valid; allowed: %s", s, strings.Join(allowed, ", "))})
		}
	}

	// Unknown key check (S16)
	for key := range raw {
		if !settingsKnownKeys[key] {
			findings = append(findings, Finding{Severity: "INFO", File: "settings.json", Path: key, RuleID: "S16",
				Message: "unknown key (will be ignored at runtime)"})
		}
	}

	return findings
}

func validateWidgets(raw map[string]any) []Finding {
	var findings []Finding
	ordersSeen := map[float64][]string{}

	for widgetName, widgetRaw := range raw {
		widgetMap, ok := widgetRaw.(map[string]any)
		if !ok {
			continue
		}

		// W01: display:order recommended
		order, hasOrder := widgetMap["display:order"]
		if !hasOrder || order == nil {
			findings = append(findings, Finding{Severity: "WARN", File: "widgets.json", Path: widgetName, RuleID: "W01",
				Message: "missing display:order (ordering will be non-deterministic)"})
		} else if f, ok := toFloat64(order); ok {
			ordersSeen[f] = append(ordersSeen[f], widgetName)
		}

		// W08: blockdef required
		blockdefRaw, hasBlockdef := widgetMap["blockdef"]
		if !hasBlockdef || blockdefRaw == nil {
			findings = append(findings, Finding{Severity: "ERROR", File: "widgets.json", Path: widgetName, RuleID: "W08",
				Message: "missing required field blockdef"})
			continue
		}
		blockdefMap, ok := blockdefRaw.(map[string]any)
		if !ok {
			findings = append(findings, Finding{Severity: "ERROR", File: "widgets.json", Path: widgetName + ".blockdef", RuleID: "W08",
				Message: "blockdef must be an object"})
			continue
		}

		// W09: meta required
		metaRaw, hasMeta := blockdefMap["meta"]
		if !hasMeta || metaRaw == nil {
			findings = append(findings, Finding{Severity: "ERROR", File: "widgets.json", Path: widgetName + ".blockdef", RuleID: "W09",
				Message: "missing required field meta"})
			continue
		}
		metaMap, ok := metaRaw.(map[string]any)
		if !ok {
			continue
		}

		basePath := widgetName + ".blockdef.meta"

		// W02: view enum
		if viewRaw, ok := metaMap["view"]; ok {
			if view, ok := viewRaw.(string); ok && view != "" && !validViews[view] {
				findings = append(findings, Finding{Severity: "ERROR", File: "widgets.json", Path: basePath + ".view", RuleID: "W02",
					Message: fmt.Sprintf("%q is not a valid view type", view)})
			}
		}

		// W03: controller enum
		controller := ""
		if ctrlRaw, ok := metaMap["controller"]; ok {
			if c, ok := ctrlRaw.(string); ok {
				controller = c
				if c != "" && !validControllers[c] {
					findings = append(findings, Finding{Severity: "ERROR", File: "widgets.json", Path: basePath + ".controller", RuleID: "W03",
						Message: fmt.Sprintf("%q is not a valid controller; allowed: shell, cmd", c)})
				}
			}
		}

		// W04: sysinfo:type enum
		if stRaw, ok := metaMap["sysinfo:type"]; ok {
			if st, ok := stRaw.(string); ok && st != "" && !validSysinfoTypes[st] {
				findings = append(findings, Finding{Severity: "ERROR", File: "widgets.json", Path: basePath + ".sysinfo:type", RuleID: "W04",
					Message: fmt.Sprintf("%q is not a valid sysinfo type", st)})
			}
		}

		// W05: graph:numpoints positive
		if npRaw, ok := metaMap["graph:numpoints"]; ok {
			if f, ok := toFloat64(npRaw); ok && f <= 0 {
				findings = append(findings, Finding{Severity: "WARN", File: "widgets.json", Path: basePath + ".graph:numpoints", RuleID: "W05",
					Message: "graph:numpoints <= 0 will use default (120)"})
			}
		}

		// W06: cmd:* keys only valid when controller=cmd
		if controller != "cmd" {
			for k := range metaMap {
				if strings.HasPrefix(k, "cmd:") {
					findings = append(findings, Finding{Severity: "WARN", File: "widgets.json", Path: basePath + "." + k, RuleID: "W06",
						Message: fmt.Sprintf("%q key is only meaningful when controller=cmd", k)})
				}
			}
		}
	}

	// W07: duplicate display:order values
	for order, names := range ordersSeen {
		if len(names) > 1 {
			findings = append(findings, Finding{Severity: "WARN", File: "widgets.json", Path: "", RuleID: "W07",
				Message: fmt.Sprintf("duplicate display:order %.4g on widgets: %s", order, strings.Join(names, ", "))})
		}
	}

	return findings
}

func validateConnections(raw map[string]any) []Finding {
	var findings []Finding

	for connName, connRaw := range raw {
		connMap, ok := connRaw.(map[string]any)
		if !ok {
			continue
		}

		// C01: ssh:hostname required
		if _, ok := connMap["ssh:hostname"]; !ok {
			findings = append(findings, Finding{Severity: "ERROR", File: "connections.json", Path: connName, RuleID: "C01",
				Message: "missing required field ssh:hostname"})
		}

		// C02+C03: ssh:port must be numeric string in range 1-65535
		if portRaw, ok := connMap["ssh:port"]; ok && portRaw != nil {
			portStr, isStr := portRaw.(string)
			if !isStr {
				findings = append(findings, Finding{Severity: "ERROR", File: "connections.json", Path: connName + ".ssh:port", RuleID: "C02",
					Message: "ssh:port must be a string"})
			} else {
				portNum, err := strconv.Atoi(portStr)
				if err != nil {
					findings = append(findings, Finding{Severity: "ERROR", File: "connections.json", Path: connName + ".ssh:port", RuleID: "C02",
						Message: fmt.Sprintf("ssh:port %q is not a valid number", portStr)})
				} else if portNum < 1 || portNum > 65535 {
					findings = append(findings, Finding{Severity: "ERROR", File: "connections.json", Path: connName + ".ssh:port", RuleID: "C03",
						Message: fmt.Sprintf("ssh:port %d is out of range [1, 65535]", portNum)})
				}
			}
		}

		// C04: ssh:identityfile paths should exist
		if ifRaw, ok := connMap["ssh:identityfile"]; ok && ifRaw != nil {
			if paths, ok := ifRaw.([]any); ok {
				for _, p := range paths {
					if pathStr, ok := p.(string); ok {
						if _, err := os.Stat(pathStr); os.IsNotExist(err) {
							findings = append(findings, Finding{Severity: "WARN", File: "connections.json", Path: connName + ".ssh:identityfile", RuleID: "C04",
								Message: fmt.Sprintf("identity file %q does not exist", pathStr)})
						}
					}
				}
			}
		}

		// C05: conn:wshpath if set should exist
		if wshRaw, ok := connMap["conn:wshpath"]; ok && wshRaw != nil {
			if wshPath, ok := wshRaw.(string); ok && wshPath != "" {
				if _, err := os.Stat(wshPath); os.IsNotExist(err) {
					findings = append(findings, Finding{Severity: "WARN", File: "connections.json", Path: connName + ".conn:wshpath", RuleID: "C05",
						Message: fmt.Sprintf("wsh path %q does not exist", wshPath)})
				}
			}
		}

		// C06: term:fontsize range 8-72
		if fsRaw, ok := connMap["term:fontsize"]; ok && fsRaw != nil {
			if f, ok := toFloat64(fsRaw); ok {
				if f < 8 || f > 72 {
					findings = append(findings, Finding{Severity: "ERROR", File: "connections.json", Path: connName + ".term:fontsize", RuleID: "C06",
						Message: fmt.Sprintf("term:fontsize %.4g is out of range [8, 72]", f)})
				}
			}
		}
	}

	return findings
}

func toFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
