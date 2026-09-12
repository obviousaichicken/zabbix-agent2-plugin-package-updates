//go:build integration

package templates_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const projectionKey = "dnf.advisory.discovery.data"

var severities = []string{"critical", "important", "moderate", "low", "unknown"}

//nolint:paralleltest // Import and lifecycle scenarios intentionally run serially against one disposable server.
func TestZabbixTemplateIntegration(t *testing.T) {
	api := connectZabbix(t)
	templates := importTestTemplates(t, api)
	for _, name := range slices.Sorted(maps.Keys(templates)) {
		t.Run(name, func(t *testing.T) {
			host := newFixtureHost(t, api, templates[name])
			if strings.HasPrefix(name, "APT") {
				checkAPTPipeline(t, host)
				return
			}
			checkDiscoveryPipeline(t, host)
			checkTriggerLifecycle(t, host)
		})
	}
}

func discoveryMacros(value string) map[string]string {
	macros := make(map[string]string, len(severities))
	for _, severity := range severities {
		macros["{$DNF.ADVISORY.LLD."+strings.ToUpper(severity)+"}"] = value
	}
	return macros
}

func advisoryFixture() object {
	ids := []string{"RLSA-2026:100", "quote\"\\path:2026", "RLSA-2026:300", "RLSA-2026:400", "RLSA-2026:500"}
	records := make([]any, 0, len(severities))
	for i, severity := range severities {
		records = append(records, object{
			"id": ids[i], "type": "security", "severity": severity, "title": severity + " fixture",
			"issued_at": "2026-08-01T10:00:00Z", "updated_at": nil,
			"cve_ids": []any{}, "affected_update_nevras": []any{},
		})
	}
	records[0] = object{
		"id": ids[0], "type": "security", "severity": "critical", "title": "Critical fixture",
		"issued_at": "2026-08-01T10:00:00Z", "updated_at": nil,
		"cve_ids":                []any{"CVE-2026-1001", "CVE-2026-1000", "CVE-2026-1001"},
		"affected_update_nevras": []any{"zlib-2.0-1.x86_64", "alpha-2.0-1.x86_64", "zlib-2.0-1.x86_64"},
	}
	return object{
		"schema_version": 1, "collection": object{"complete": true, "duration_ms": 0},
		"metadata": object{"details_complete": true, "cves_complete": true, "issue_dates_complete": true},
		"summary": object{
			"advisories": 5, "unique_cves": 2,
			"advisories_by_severity":      object{"critical": 1, "important": 1, "moderate": 1, "low": 1, "unknown": 1},
			"package_updates_by_severity": object{"critical": 2, "important": 0, "moderate": 0, "low": 0, "unknown": 0},
			"oldest_vendor_timestamp":     nil, "oldest_vendor_age_seconds": nil, "oldest_vendor_timestamp_basis": "unknown",
		},
		"advisories": records,
	}
}

func projectionRecords(h *fixtureHost) ([]object, error) {
	item := h.items[projectionKey]
	value, _ := item["lastvalue"].(string)
	if item["state"] != "0" || value == "" {
		return nil, fmt.Errorf("projection not supported: %v", item)
	}
	var records []object
	if err := json.Unmarshal([]byte(value), &records); err != nil {
		return nil, err
	}
	return records, nil
}

func checkDiscoveryPipeline(t *testing.T, h *fixtureHost) {
	t.Helper()
	// No host overrides: these assertions exercise the shipped defaults.
	h.feed(t, "advisories.get", advisoryFixture(), func() error { return h.value(projectionKey, "[]") })
	incomplete := advisoryFixture()
	incomplete["metadata"] = object{"details_complete": false, "cves_complete": false, "issue_dates_complete": false}
	h.feed(t, "advisories.get", incomplete, func() error { return h.value(projectionKey, "[]") })
	h.macros(t, discoveryMacros("1"))
	var expected []object
	h.feed(t, "advisories.get", advisoryFixture(), func() error {
		records, err := projectionRecords(h)
		if err != nil {
			return err
		}
		if len(records) != 5 {
			return fmt.Errorf("want all five severities, got %v", records)
		}
		expected = records
		return nil
	})
	byID := indexBy(t, expected, "id")
	quoted := byID["quote\"\\path:2026"]
	// Literal UTF-16 encoding for the quoted/backslashed fixture ID.
	equal(t, "safe identifier", quoted["safe_id"], "a00710075006f007400650022005c0070006100740068003a0032003000320036")
	critical := byID["RLSA-2026:100"]
	equal(t, "package count", critical["affected_package_count"], float64(2))
	equal(t, "sorted unique packages", critical["affected_packages"], `["alpha-2.0-1.x86_64","zlib-2.0-1.x86_64"]`)
	equal(t, "sorted unique CVEs", critical["cves"], `["CVE-2026-1000","CVE-2026-1001"]`)
	reversed := advisoryFixture()
	records := reversed["advisories"].([]any)
	slices.Reverse(records)
	h.feed(t, "advisories.get", reversed, func() error {
		actual, err := projectionRecords(h)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("projection order changed: %v", actual)
		}
		return nil
	})
	checkDiscoveredItems(t, h, expected)
	checkDisappearance(t, h, text(t, quoted["safe_id"]))
	checkRejectedDiscovery(t, h)
}

func checkDiscoveredItems(t *testing.T, h *fixtureHost, records []object) {
	t.Helper()
	priorities := map[string]string{"critical": "5", "important": "4", "moderate": "2", "low": "1", "unknown": "4"}
	h.feed(t, "advisories.get", advisoryFixture(), func() error {
		for _, record := range records {
			id := fmt.Sprint(record["safe_id"])
			for suffix, want := range map[string]string{
				"presence": "1", "vendor.timestamp": "1785578400",
				"packages.count": fmt.Sprint(record["affected_package_count"]),
				"packages.list":  fmt.Sprint(record["affected_packages"]), "cves.list": fmt.Sprint(record["cves"]),
			} {
				if err := h.value("dnf.advisory."+suffix+"["+id+"]", want); err != nil {
					return err
				}
			}
			item := h.items["dnf.advisory.presence["+id+"]"]
			var triggers []object
			h.api.mustCall(t, "trigger.get", object{"itemids": []any{item["itemid"]}, "output": []string{"value", "priority", "state", "error"}}, &triggers)
			if len(triggers) != 1 {
				return fmt.Errorf("want one discovered trigger for %s, got %v", id, triggers)
			}
			problem := triggers[0]["value"] == "1" && triggers[0]["state"] == "0"
			priority := priorities[fmt.Sprint(record["severity"])]
			if !problem || triggers[0]["priority"] != priority {
				return fmt.Errorf("discovered trigger for %s: %v", id, triggers)
			}
		}
		return nil
	})
}

func checkDisappearance(t *testing.T, h *fixtureHost, safeID string) {
	t.Helper()
	payload := advisoryFixture()
	records := payload["advisories"].([]any)
	payload["advisories"] = append(records[:1], records[2:]...)
	asObject(t, payload["summary"])["advisories"] = 4
	key := "dnf.advisory.presence[" + safeID + "]"
	h.feed(t, "advisories.get", payload, func() error {
		if err := h.value(key, "0"); err != nil {
			return err
		}
		var triggers []object
		h.api.mustCall(t, "trigger.get", object{"itemids": []any{h.items[key]["itemid"]}, "output": []string{"value", "state"}}, &triggers)
		if len(triggers) != 1 || triggers[0]["value"] != "0" || triggers[0]["state"] != "0" {
			return fmt.Errorf("disappeared advisory did not recover: %v", triggers)
		}
		return nil
	})
}

// Each subtest changes the same host's macros and checks recovery from the preceding state.
func checkRejectedDiscovery(t *testing.T, h *fixtureHost) {
	t.Helper()
	type rejectionCase struct {
		name     string
		disabled bool
		macro    string
		mutate   func(object)
		error    string
	}
	cases := []rejectionCase{
		{name: "macro 2", macro: "2", error: "must be exactly 0 or 1"},
		{name: "macro -1", macro: "-1", error: "must be exactly 0 or 1"},
		{name: "macro true", macro: "true", error: "must be exactly 0 or 1"},
		{name: "macro false", macro: "false", error: "must be exactly 0 or 1"},
		{
			name:   "collection incomplete",
			mutate: func(p object) { asObject(t, p["collection"])["complete"] = false },
			error:  "requires a complete collection",
		},
		{
			name: "disabled collection incomplete", disabled: true,
			mutate: func(p object) { asObject(t, p["collection"])["complete"] = false },
			error:  "requires a complete collection",
		},
		{
			name:   "count mismatch",
			mutate: func(p object) { asObject(t, p["summary"])["advisories"] = 6 },
			error:  "payload count is inconsistent",
		},
		{
			name: "disabled count mismatch", disabled: true,
			mutate: func(p object) { asObject(t, p["summary"])["advisories"] = 6 },
			error:  "payload count is inconsistent",
		},
	}
	for _, capability := range []string{"details_complete", "cves_complete", "issue_dates_complete"} {
		cases = append(cases, rejectionCase{
			name:   capability,
			mutate: func(p object) { asObject(t, p["metadata"])[capability] = false },
			error:  "requires complete detail, CVE, and issue-date metadata",
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			macros := discoveryMacros("1")
			if tc.disabled {
				macros = discoveryMacros("0")
			}
			h.macros(t, macros)
			// Establish a supported value first, so stale errors cannot pass.
			h.feed(t, "advisories.get", advisoryFixture(), func() error {
				records, err := projectionRecords(h)
				if err != nil {
					return err
				}
				want := 5
				if tc.disabled {
					want = 0
				}
				if len(records) != want {
					return fmt.Errorf("want %d records, got %v", want, records)
				}
				return nil
			})
			if tc.macro != "" {
				macros["{$DNF.ADVISORY.LLD.CRITICAL}"] = tc.macro
				h.macros(t, macros)
			}
			payload := advisoryFixture()
			if tc.mutate != nil {
				tc.mutate(payload)
			}
			h.feed(t, "advisories.get", payload, func() error { return h.unsupported(projectionKey, tc.error) })
		})
	}
}

var advisoryTriggerNames = map[string]string{
	"unavailable": "DNF: Advisory collection is unavailable",
	"critical":    "DNF: Critical security advisories are applicable",
	"important":   "DNF: Important security advisories are applicable",
	"old":         "DNF: Applicable security advisory is old",
	"unknown":     "DNF: Security advisory severity is unknown",
	"incomplete":  "DNF: Advisory metadata is incomplete",
	"unmatched":   "DNF: Security package updates lack advisory objects",
}

func healthyAdvisories(t *testing.T) object {
	t.Helper()
	payload := advisoryFixture()
	payload["advisories"] = []any{}
	summary := asObject(t, payload["summary"])
	summary["advisories"] = 0
	summary["unique_cves"] = 0
	for _, field := range []string{"advisories_by_severity", "package_updates_by_severity"} {
		values := asObject(t, summary[field])
		for key := range values {
			values[key] = 0
		}
	}
	return payload
}

func (h *fixtureHost) problems(t *testing.T, want []string) error {
	t.Helper()
	var triggers []object
	h.api.mustCall(t, "trigger.get", object{
		"hostids": []string{h.id}, "output": []string{"description", "value", "state", "error"},
		"filter": object{"description": slices.Collect(maps.Values(advisoryTriggerNames))},
	}, &triggers)
	if len(triggers) != len(advisoryTriggerNames) {
		return fmt.Errorf("missing triggers: %v", triggers)
	}
	byName := indexBy(t, triggers, "description")
	for short, name := range advisoryTriggerNames {
		value := "0"
		if slices.Contains(want, short) {
			value = "1"
		}
		if byName[name]["value"] != value || byName[name]["state"] != "0" {
			return fmt.Errorf("%s: want %s, got %v", name, value, byName[name])
		}
	}
	return nil
}

// Problem and recovery are ordered transitions on a shared fixture host.
func checkTriggerLifecycle(t *testing.T, h *fixtureHost) {
	t.Helper()
	h.macros(t, discoveryMacros("0"))
	packages := loadPayload(t, "../internal/results/testdata/packages-dnf.golden.json")
	asObject(t, asObject(t, packages["summary"])["update_types"])["security"] = 0
	h.feed(t, "packages.get", packages, func() error { return h.value("dnf.updates.security", "0") })
	cases := []struct {
		name        string
		counts      object
		packages    object
		age         any
		incomplete  bool
		unavailable bool
		security    int
		want        []string
	}{
		{name: "healthy"},
		{name: "Critical suppresses Important", counts: object{"critical": 1, "important": 3}, want: []string{"critical"}},
		{name: "Important without Critical", counts: object{"important": 1}, want: []string{"important"}},
		{name: "old vendor timestamp", age: 604801, want: []string{"old"}},
		{name: "unknown advisory", counts: object{"unknown": 1}, want: []string{"unknown"}},
		{name: "unknown package", packages: object{"unknown": 1}, want: []string{"unknown"}},
		{name: "DNF4 incomplete metadata", incomplete: true, want: []string{"incomplete"}},
		{name: "collection incomplete", unavailable: true, want: []string{"unavailable"}},
		{name: "unmatched security update", security: 2, packages: object{"important": 1}, want: []string{"unmatched"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := healthyAdvisories(t)
			summary := asObject(t, payload["summary"])
			maps.Copy(asObject(t, summary["advisories_by_severity"]), tc.counts)
			maps.Copy(asObject(t, summary["package_updates_by_severity"]), tc.packages)
			summary["oldest_vendor_age_seconds"] = tc.age
			if tc.incomplete {
				payload["metadata"] = object{"details_complete": false, "cves_complete": false, "issue_dates_complete": false}
			}
			if tc.unavailable {
				asObject(t, payload["collection"])["complete"] = false
			}
			asObject(t, asObject(t, packages["summary"])["update_types"])["security"] = tc.security
			h.feed(t, "packages.get", packages, func() error { return h.value("dnf.updates.security", fmt.Sprint(tc.security)) })
			h.feed(t, "advisories.get", payload, func() error { return h.problems(t, tc.want) })
			// Every problem scenario must recover using the real expressions.
			asObject(t, asObject(t, packages["summary"])["update_types"])["security"] = 0
			h.feed(t, "packages.get", packages, func() error { return h.value("dnf.updates.security", "0") })
			h.feed(t, "advisories.get", healthyAdvisories(t), func() error { return h.problems(t, nil) })
		})
	}
	macros := discoveryMacros("0")
	macros["{$DNF.ADVISORY.NODATA.TIME}"] = "30s"
	h.macros(t, macros)
	await(t, 90*time.Second, "real nodata trigger", func() error { return h.problems(t, []string{"unavailable"}) })
	h.feed(t, "advisories.get", healthyAdvisories(t), func() error { return h.problems(t, nil) })
}

func checkAPTPipeline(t *testing.T, h *fixtureHost) {
	t.Helper()
	payload := loadPayload(t, "../internal/results/testdata/packages-apt.golden.json")
	h.feed(t, "packages.get", payload, func() error {
		for key, value := range map[string]string{
			"apt.collection.complete": "1", "apt.updates.security": "1", "apt.updates.other": "1",
			"apt.reboot.pending": "1", "apt.last_update.timestamp": "0", "apt.metadata.age": "1800",
			`apt.repository.update.details["apt-11111111111111111111111111111111"]`: "openssl:amd64=3:3.0.2-0ubuntu1.20",
		} {
			if err := h.value(key, value); err != nil {
				return err
			}
		}
		return nil
	})
	asObject(t, payload["metadata"])["refreshed_at"] = nil
	h.feed(t, "packages.get", payload, func() error { return h.value("apt.metadata.refreshed", "0") })
}
