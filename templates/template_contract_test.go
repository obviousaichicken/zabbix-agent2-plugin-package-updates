package templates_test

import (
	"fmt"
	"maps"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const templateFile = "template-package-updates-by-zabbix-agent2.yaml"

type object = map[string]any

func readTemplates(t *testing.T) object {
	t.Helper()
	data, err := os.ReadFile(templateFile)
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var rejectAliases func(*yaml.Node)
	rejectAliases = func(n *yaml.Node) {
		if n.Kind == yaml.AliasNode {
			t.Fatalf("template contains YAML alias at line %d", n.Line)
		}
		for _, child := range n.Content {
			rejectAliases(child)
		}
	}
	rejectAliases(&document)
	var root object
	if err := document.Decode(&root); err != nil {
		t.Fatal(err)
	}
	return asObject(t, root["zabbix_export"])
}

func asObject(t *testing.T, value any) object {
	t.Helper()
	o, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T: %v", value, value)
	}
	return o
}

func objects(t *testing.T, value any) []object {
	t.Helper()
	if value == nil {
		return nil
	}
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("expected array, got %T: %v", value, value)
	}
	result := make([]object, 0, len(values))
	for _, v := range values {
		result = append(result, asObject(t, v))
	}
	return result
}

func text(t *testing.T, value any) string {
	t.Helper()
	s, ok := value.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", value, value)
	}
	return s
}

func equal(t *testing.T, label string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got: %#v\nwant: %#v", label, got, want)
	}
}

func fields(t *testing.T, got, want object) {
	t.Helper()
	for key, value := range want {
		equal(t, key, got[key], value)
	}
}

func indexBy(t *testing.T, entries []object, key string) map[string]object {
	t.Helper()
	result := make(map[string]object, len(entries))
	for _, entry := range entries {
		name := text(t, entry[key])
		if _, exists := result[name]; exists {
			t.Fatalf("duplicate %s: %s", key, name)
		}
		result[name] = entry
	}
	return result
}

func exactKeys(t *testing.T, entries map[string]object, expected string) {
	t.Helper()
	want := strings.Fields(expected)
	slices.Sort(want)
	equal(t, "keys", slices.Sorted(maps.Keys(entries)), want)
}

func stepParameter(t *testing.T, item object, kind string) string {
	t.Helper()
	for _, step := range objects(t, item["preprocessing"]) {
		if step["type"] == kind {
			parameters, ok := step["parameters"].([]any)
			if !ok || len(parameters) != 1 {
				t.Fatalf("invalid %s parameters: %v", kind, step)
			}
			return text(t, parameters[0])
		}
	}
	t.Fatalf("missing %s preprocessing: %v", kind, item)
	return ""
}

func contains(t *testing.T, source string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(source, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
}

func normalize(value any, active, passive string) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(object, len(v))
		for key, child := range v {
			if key != "uuid" {
				result[key] = normalize(child, active, passive)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, child := range v {
			result[i] = normalize(child, active, passive)
		}
		return result
	case string:
		if v == "ZABBIX_ACTIVE" || v == "ZABBIX_PASSIVE" {
			return "ZABBIX_AGENT"
		}
		return strings.ReplaceAll(v, active, passive)
	default:
		return value
	}
}

func TestTemplateExport(t *testing.T) {
	t.Parallel()
	export := readTemplates(t)
	equal(t, "export version", export["version"], "7.0")
	pattern := regexp.MustCompile(`^[0-9a-f]{12}4[0-9a-f]{3}[89ab][0-9a-f]{15}$`)
	seen := map[string]bool{}
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if uuid, exists := v["uuid"]; exists {
				id := text(t, uuid)
				if !pattern.MatchString(id) || seen[id] {
					t.Errorf("invalid or duplicate UUID: %q", id)
				}
				seen[id] = true
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(export)
	templates := indexBy(t, objects(t, export["templates"]), "template")
	equal(t, "template count", len(templates), 4)
	for _, family := range []string{"DNF", "APT"} {
		passive := family + " by Zabbix agent 2"
		active := passive + " active"
		if templates[passive] == nil || templates[active] == nil {
			t.Fatalf("missing %s template pair", family)
		}
		equal(t, family+" parity", normalize(templates[passive], active, passive), normalize(templates[active], active, passive))
	}
}

func TestTemplateMasterReferences(t *testing.T) {
	t.Parallel()
	for _, template := range objects(t, readTemplates(t)["templates"]) {
		t.Run(text(t, template["template"]), func(t *testing.T) {
			t.Parallel()
			items := indexBy(t, objects(t, template["items"]), "key")
			entries := objects(t, template["items"])
			for _, rule := range objects(t, template["discovery_rules"]) {
				entries = append(entries, rule)
				entries = append(entries, objects(t, rule["item_prototypes"])...)
			}
			for _, entry := range entries {
				master := entry["master_item"]
				if master == nil {
					if entry["type"] == "DEPENDENT" {
						t.Errorf("%s has no master item", entry["key"])
					}
					continue
				}
				key := text(t, asObject(t, master)["key"])
				if items[key] == nil {
					t.Errorf("%s references missing master %s", entry["key"], key)
				}
			}
		})
	}
}

func TestTemplateFamilyContracts(t *testing.T) {
	t.Parallel()
	for _, template := range objects(t, readTemplates(t)["templates"]) {
		name := text(t, template["template"])
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			items := indexBy(t, objects(t, template["items"]), "key")
			macros := indexBy(t, objects(t, template["macros"]), "macro")
			if strings.HasPrefix(name, "DNF") {
				checkDNFContract(t, template, items, macros)
				checkDiscoveryContract(t, template, items)
				checkAdvisoryTriggers(t, template, items)
			} else {
				checkAPTContract(t, template, items, macros)
			}
		})
	}
}

func checkDNFContract(t *testing.T, template object, items, macros map[string]object) {
	t.Helper()
	exactKeys(t, items, `packages.get advisories.get dnf.advisory.discovery.data
		dnf.collection.complete dnf.collection.duration dnf.classification.complete
		dnf.updates dnf.repositories dnf.updates.pending dnf.reboot.pending
		dnf.updates.security dnf.updates.bugfix dnf.updates.enhancement dnf.updates.other
		dnf.last_update.result dnf.last_update.timestamp
		dnf.advisory.collection.complete dnf.advisory.collection.duration
		dnf.advisory.details.complete dnf.advisory.cves.complete dnf.advisory.issue_dates.complete
		dnf.advisory.total dnf.advisory.cves
		dnf.advisory.critical dnf.advisory.important dnf.advisory.moderate dnf.advisory.low dnf.advisory.unknown
		dnf.advisory.packages.critical dnf.advisory.packages.important dnf.advisory.packages.moderate
		dnf.advisory.packages.low dnf.advisory.packages.unknown
		dnf.advisory.oldest.timestamp dnf.advisory.oldest.age dnf.advisory.oldest.basis`)
	exactKeys(t, macros, `{$DNF.ADVISORY.LLD.CRITICAL} {$DNF.ADVISORY.LLD.IMPORTANT}
		{$DNF.ADVISORY.LLD.MODERATE} {$DNF.ADVISORY.LLD.LOW} {$DNF.ADVISORY.LLD.UNKNOWN}
		{$DNF.ADVISORY.NODATA.TIME} {$DNF.ADVISORY.UPDATE.INTERVAL}
		{$DNF.COLLECTION.DURATION.MAX} {$DNF.COLLECTION.DURATION.WINDOW} {$DNF.NODATA.TIME}
		{$DNF.SECURITY.ADVISORY.MAX.AGE} {$DNF.SECURITY.CRITICAL.MIN} {$DNF.SECURITY.IMPORTANT.MIN}
		{$DNF.SECURITY.MIN} {$DNF.SECURITY.UNKNOWN.MAX} {$DNF.UPDATE.INTERVAL}`)
	for key, macro := range macros {
		if strings.HasPrefix(key, "{$DNF.ADVISORY.LLD.") {
			equal(t, key+" disabled by default", macro["value"], "0")
		}
	}
	kind := "ZABBIX_PASSIVE"
	if strings.HasSuffix(text(t, template["template"]), " active") {
		kind = "ZABBIX_ACTIVE"
	}
	fields(t, items["advisories.get"], object{
		"type": kind, "delay": "{$DNF.ADVISORY.UPDATE.INTERVAL}", "history": "0", "value_type": "TEXT", "timeout": "30s",
	})
	paths := map[string]string{
		"collection.complete": "$.collection.complete", "collection.duration": "$.collection.duration_ms",
		"details.complete": "$.metadata.details_complete", "cves.complete": "$.metadata.cves_complete",
		"issue_dates.complete": "$.metadata.issue_dates_complete", "total": "$.summary.advisories",
		"cves": "$.summary.unique_cves", "oldest.basis": "$.summary.oldest_vendor_timestamp_basis",
	}
	for _, severity := range []string{"critical", "important", "moderate", "low", "unknown"} {
		paths[severity] = "$.summary.advisories_by_severity." + severity
		paths["packages."+severity] = "$.summary.package_updates_by_severity." + severity
	}
	for suffix, path := range paths {
		equal(t, suffix+" JSONPath", stepParameter(t, items["dnf.advisory."+suffix], "JSONPATH"), path)
	}
	for key, item := range items {
		if strings.HasPrefix(key, "dnf.advisory.") {
			fields(t, item, object{"type": "DEPENDENT", "master_item": object{"key": "advisories.get"}})
		}
	}
	for _, suffix := range []string{"collection.complete", "details.complete", "cves.complete", "issue_dates.complete"} {
		var kinds []string
		for _, step := range objects(t, items["dnf.advisory."+suffix]["preprocessing"]) {
			kinds = append(kinds, text(t, step["type"]))
		}
		equal(t, suffix+" boolean conversion", kinds, []string{"JSONPATH", "BOOL_TO_DECIMAL"})
	}
	for key, fragments := range map[string][]string{
		"dnf.advisory.oldest.timestamp": {"oldest_vendor_timestamp", "Date.parse", "return 0"},
		"dnf.advisory.oldest.age":       {"oldest_vendor_age_seconds", "isFinite", "return 0"},
		"dnf.last_update.timestamp":     {"last_update.timestamp === null", "return 0"},
	} {
		js := stepParameter(t, items[key], "JAVASCRIPT")
		contains(t, js, fragments...)
		if strings.Contains(js, "return null") {
			t.Errorf("%s must return a recoverable numeric value", key)
		}
	}
}

func checkDiscoveryContract(t *testing.T, template object, items map[string]object) {
	t.Helper()
	projection := items["dnf.advisory.discovery.data"]
	fields(t, projection, object{"type": "DEPENDENT", "history": "0", "value_type": "TEXT", "master_item": object{"key": "advisories.get"}})
	equal(t, "projection step count", len(objects(t, projection["preprocessing"])), 1)
	contains(t, stepParameter(t, projection, "JAVASCRIPT"),
		"data.schema_version !== 1", "data.collection.complete !== true",
		"data.metadata.details_complete !== true", "data.metadata.cves_complete !== true",
		"data.metadata.issue_dates_complete !== true", "must be exactly 0 or 1", "id.length > 256",
		"id.charCodeAt(character).toString(16)", "return JSON.stringify(records)")
	discovery := indexBy(t, objects(t, template["discovery_rules"]), "key")["dnf.advisory.discovery"]
	fields(t, discovery, object{
		"type": "DEPENDENT", "lifetime_type": "DELETE_AFTER", "lifetime": "30d",
		"enabled_lifetime_type": "DISABLE_AFTER", "enabled_lifetime": "1d",
		"master_item": object{"key": "dnf.advisory.discovery.data"},
	})
	prototypes := indexBy(t, objects(t, discovery["item_prototypes"]), "key")
	exactKeys(t, prototypes, `dnf.advisory.presence[{#ADVISORY_SAFE_ID}]
		dnf.advisory.vendor.timestamp[{#ADVISORY_SAFE_ID}] dnf.advisory.packages.count[{#ADVISORY_SAFE_ID}]
		dnf.advisory.packages.list[{#ADVISORY_SAFE_ID}] dnf.advisory.cves.list[{#ADVISORY_SAFE_ID}]`)
	for key, prototype := range prototypes {
		contains(t, key, "{#ADVISORY_SAFE_ID}")
		contains(t, text(t, prototype["name"]), "{#ADVISORY_ID}")
		fields(t, prototype, object{"master_item": object{"key": "dnf.advisory.discovery.data"}})
		tags := indexBy(t, objects(t, prototype["tags"]), "tag")
		equal(t, "advisory tag", tags["advisory"]["value"], "{#ADVISORY_ID}")
		equal(t, "severity tag", tags["severity"]["value"], "{#ADVISORY_SEVERITY}")
	}
	presence := objects(t, prototypes["dnf.advisory.presence[{#ADVISORY_SAFE_ID}]"]["preprocessing"])
	if len(presence) == 0 {
		t.Fatal("missing presence preprocessing")
	}
	fields(t, presence[0], object{"type": "JSONPATH", "error_handler": "CUSTOM_VALUE", "error_handler_params": "0"})
	paths := indexBy(t, objects(t, discovery["lld_macro_paths"]), "lld_macro")
	equal(t, "LLD path count", len(paths), 4)
	for macro, path := range map[string]string{
		"{#ADVISORY_ID}": "$.id", "{#ADVISORY_SAFE_ID}": "$.safe_id",
		"{#ADVISORY_SEVERITY}": "$.severity", "{#ADVISORY_TITLE}": "$.title",
	} {
		equal(t, macro, paths[macro]["path"], path)
	}
	triggers := objects(t, discovery["trigger_prototypes"])
	if len(triggers) != 1 {
		t.Fatalf("want one advisory trigger prototype, got %d", len(triggers))
	}
	equal(t, "presence trigger", triggers[0]["expression"], fmt.Sprintf("last(/%s/dnf.advisory.presence[{#ADVISORY_SAFE_ID}])=1", template["template"]))
	severities := map[string]string{}
	for _, override := range objects(t, discovery["overrides"]) {
		conditions := objects(t, asObject(t, override["filter"])["conditions"])
		operations := objects(t, override["operations"])
		if len(conditions) != 1 || len(operations) != 1 {
			t.Fatal("severity override requires one condition and operation")
		}
		fields(t, conditions[0], object{"macro": "{#ADVISORY_SEVERITY}"})
		fields(t, operations[0], object{"operationobject": "TRIGGER_PROTOTYPE", "operator": "REGEXP", "value": `^DNF: Advisory \[`})
		severities[strings.Trim(text(t, conditions[0]["value"]), "^$")] = text(t, operations[0]["severity"])
	}
	equal(t, "severity overrides", severities, map[string]string{
		"critical": "DISASTER", "important": "HIGH", "moderate": "WARNING", "low": "INFO", "unknown": "HIGH",
	})
}

func checkAdvisoryTriggers(t *testing.T, template object, items map[string]object) {
	t.Helper()
	var entries []object
	for _, item := range items {
		entries = append(entries, objects(t, item["triggers"])...)
	}
	triggers := indexBy(t, entries, "name")
	name := text(t, template["template"])
	last := func(key string) string { return "last(/" + name + "/" + key + ")" }
	advisory := func(key string) string { return last("dnf.advisory." + key) }
	expressions := map[string]string{
		"DNF: Advisory collection is unavailable": advisory("collection.complete") + "=0 or nodata(/" + name +
			"/dnf.advisory.collection.complete,{$DNF.ADVISORY.NODATA.TIME})=1",
		"DNF: Critical security advisories are applicable": advisory("critical") + ">={$DNF.SECURITY.CRITICAL.MIN}",
		"DNF: Important security advisories are applicable": advisory("critical") + "<{$DNF.SECURITY.CRITICAL.MIN} and " +
			advisory("important") + ">={$DNF.SECURITY.IMPORTANT.MIN}",
		"DNF: Applicable security advisory is old": advisory("oldest.age") + ">{$DNF.SECURITY.ADVISORY.MAX.AGE}",
		"DNF: Security advisory severity is unknown": advisory("unknown") + ">{$DNF.SECURITY.UNKNOWN.MAX} or " +
			advisory("packages.unknown") + ">{$DNF.SECURITY.UNKNOWN.MAX}",
		"DNF: Advisory metadata is incomplete": advisory("details.complete") + "=0 or " + advisory("cves.complete") + "=0 or " +
			advisory("issue_dates.complete") + "=0",
		"DNF: Security package updates lack advisory objects": last("dnf.updates.security") + ">(" + advisory("packages.critical") + "+" +
			advisory("packages.important") + "+" + advisory("packages.moderate") + "+" +
			advisory("packages.low") + "+" + advisory("packages.unknown") + ")",
	}
	for trigger, expression := range expressions {
		equal(t, trigger, triggers[trigger]["expression"], expression)
	}
	contains(t, text(t, triggers["DNF: Security updates are available"]["expression"]), "dnf.updates.security", "{$DNF.SECURITY.MIN}")
}

func checkAPTContract(t *testing.T, template object, items, macros map[string]object) {
	t.Helper()
	exactKeys(t, items, `packages.get apt.collection.complete apt.collection.duration apt.repositories apt.updates
		apt.updates.pending apt.updates.security apt.updates.other apt.reboot.pending
		apt.last_update.result apt.last_update.timestamp apt.metadata.refreshed apt.metadata.age`)
	exactKeys(t, macros, `{$APT.UPDATE.INTERVAL} {$APT.NODATA.TIME} {$APT.COLLECTION.DURATION.MAX}
		{$APT.SECURITY.MIN} {$APT.METADATA.AGE.MAX}`)
	for _, key := range []string{"apt.last_update.timestamp", "apt.metadata.refreshed"} {
		js := stepParameter(t, items[key], "JAVASCRIPT")
		contains(t, js, "return 0")
		if strings.Contains(js, "return null") {
			t.Errorf("%s must return a recoverable numeric value", key)
		}
	}
	found := false
	for _, rule := range objects(t, template["discovery_rules"]) {
		for _, prototype := range objects(t, rule["item_prototypes"]) {
			if strings.HasPrefix(text(t, prototype["key"]), "apt.repository.update.details") {
				contains(t, stepParameter(t, prototype, "JAVASCRIPT"), "updates[i].identifier")
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing APT repository package details prototype")
	}
}
