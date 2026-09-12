//go:build integration

package templates_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type zabbixAPI struct {
	url    string
	token  string
	client *http.Client
}

func (api *zabbixAPI) call(ctx context.Context, method string, params, result any) error {
	request := object{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json-rpc")
	if api.token != "" {
		req.Header.Set("Authorization", "Bearer "+api.token)
	}
	response, err := api.client.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", method, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("read %s: %w", method, err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d: %s", method, response.StatusCode, body)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode %s: %w (body: %s)", method, err, body)
	}
	if e, exists := envelope["error"]; exists {
		return fmt.Errorf("%s: %s", method, e)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(envelope["result"], result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}

func (api *zabbixAPI) mustCall(t *testing.T, method string, params, result any) {
	t.Helper()
	if err := api.call(t.Context(), method, params, result); err != nil {
		t.Fatal(err)
	}
}

func await(t *testing.T, timeout time.Duration, description string, check func() error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	for {
		err := check()
		if err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %s: %v; last observation: %v", description, ctx.Err(), err)
		case <-time.After(time.Second):
		}
	}
}

func connectZabbix(t *testing.T) *zabbixAPI {
	t.Helper()
	url := os.Getenv("ZBX_TEST_API_URL")
	if url == "" {
		url = "http://127.0.0.1:17070/api_jsonrpc.php"
	}
	api := &zabbixAPI{url: url, client: &http.Client{Timeout: 10 * time.Second}}
	var version string
	await(t, 3*time.Minute, "Zabbix API", func() error {
		return api.call(t.Context(), "apiinfo.version", object{}, &version)
	})
	t.Logf("Zabbix API version: %s", version)
	if branch := os.Getenv("ZBX_TEST_VERSION"); branch != "" && !strings.HasPrefix(version, branch+".") {
		t.Fatalf("Zabbix API version %s does not match requested branch %s", version, branch)
	}
	api.mustCall(t, "user.login", object{"username": "Admin", "password": "zabbix"}, &api.token)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.call(ctx, "user.logout", []any{}, nil); err != nil {
			t.Logf("logout: %v", err)
		}
	})
	return api
}

func importTestTemplates(t *testing.T, api *zabbixAPI) map[string]object {
	t.Helper()
	source, err := os.ReadFile(templateFile)
	if err != nil {
		t.Fatal(err)
	}
	rules := object{}
	for _, kind := range []string{"template_groups", "templates", "items", "discoveryRules", "triggers", "valueMaps"} {
		rules[kind] = object{"createMissing": true, "updateExisting": true}
	}
	for range 2 {
		var imported bool
		api.mustCall(t, "configuration.import", object{"format": "yaml", "source": string(source), "rules": rules}, &imported)
		if !imported {
			t.Fatal("Zabbix did not import the template")
		}
	}
	names := []string{"DNF by Zabbix agent 2", "DNF by Zabbix agent 2 active", "APT by Zabbix agent 2", "APT by Zabbix agent 2 active"}
	var templates []object
	api.mustCall(t, "template.get", object{"output": []string{"templateid", "host"}, "filter": object{"host": names}}, &templates)
	if len(templates) != 4 {
		t.Fatalf("expected four imported templates, got %v", templates)
	}
	return indexBy(t, templates, "host")
}

type fixtureHost struct {
	api      *zabbixAPI
	id       string
	items    map[string]object
	sequence int
}

func newFixtureHost(t *testing.T, api *zabbixAPI, template object) *fixtureHost {
	t.Helper()
	var templateItems []object
	api.mustCall(t, "item.get", object{
		"templateids": []any{template["templateid"]}, "output": []string{"itemid", "key_"},
		"filter": object{"key_": []string{"packages.get", "advisories.get", "dnf.advisory.discovery.data"}},
	}, &templateItems)
	for _, item := range templateItems {
		update := object{"itemid": item["itemid"], "history": "1h"}
		if item["key_"] != "dnf.advisory.discovery.data" {
			// Zabbix forbids changing an inherited host item's type. Adapt only
			// the transport on the disposable template, after testing imports
			// of the original YAML. The processing pipeline stays untouched.
			update["type"] = 2 // Zabbix trapper; accepts history.push.
			update["delay"] = "0"
		}
		api.mustCall(t, "item.update", update, nil)
	}
	var groups []object
	api.mustCall(t, "hostgroup.get", object{"output": []string{"groupid"}, "filter": object{"name": "Template fixture tests"}}, &groups)
	var groupID string
	if len(groups) == 0 {
		var result map[string][]string
		api.mustCall(t, "hostgroup.create", object{"name": "Template fixture tests"}, &result)
		groupID = result["groupids"][0]
	} else {
		groupID = text(t, groups[0]["groupid"])
	}
	var created map[string][]string
	api.mustCall(t, "host.create", object{
		"host":       fmt.Sprintf("template-fixture-%d", time.Now().UnixNano()),
		"groups":     []object{{"groupid": groupID}},
		"templates":  []object{{"templateid": template["templateid"]}},
		"interfaces": []object{{"type": 1, "main": 1, "useip": 1, "ip": "127.0.0.1", "dns": "", "port": "10050"}},
	}, &created)
	host := &fixtureHost{api: api, id: created["hostids"][0]}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.call(ctx, "host.delete", []string{host.id}, nil); err != nil {
			t.Errorf("delete fixture host: %v", err)
		}
	})
	host.refresh(t)
	return host
}

func (h *fixtureHost) refresh(t *testing.T) {
	t.Helper()
	var items []object
	h.api.mustCall(t, "item.get", object{
		"hostids": []string{h.id}, "output": []string{"itemid", "key_", "lastvalue", "lastclock", "state", "error"},
	}, &items)
	h.items = indexBy(t, items, "key_")
}

func (h *fixtureHost) macros(t *testing.T, values map[string]string) {
	t.Helper()
	macros := make([]object, 0, len(values))
	for key, value := range values {
		macros = append(macros, object{"macro": key, "value": value})
	}
	h.api.mustCall(t, "host.update", object{"hostid": h.id, "macros": macros}, nil)
}

func (h *fixtureHost) push(t *testing.T, key string, payload object) error {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var result struct {
		Data []struct {
			Error string `json:"error"`
		} `json:"data"`
	}
	err = h.api.call(t.Context(), "history.push", []object{{"itemid": h.items[key]["itemid"], "value": string(data)}}, &result)
	if err != nil {
		return err
	}
	if len(result.Data) != 1 || result.Data[0].Error != "" {
		return fmt.Errorf("history.push %s: %+v", key, result)
	}
	return nil
}

func (h *fixtureHost) feed(t *testing.T, key string, payload object, check func() error) {
	t.Helper()
	h.sequence++
	payload = maps.Clone(payload)
	collection := maps.Clone(asObject(t, payload["collection"]))
	collection["duration_ms"] = h.sequence * 1000
	payload["collection"] = collection
	barrier := "dnf.collection.duration"
	if payload["backend"] == "apt" {
		barrier = "apt.collection.duration"
	} else if key == "advisories.get" {
		barrier = "dnf.advisory.collection.duration"
	}
	// Re-submit while caches and LLD converge. A successful API push alone
	// does not prove that asynchronous preprocessing or discovery completed.
	await(t, 60*time.Second, key+" preprocessing/discovery", func() error {
		if err := h.push(t, key, payload); err != nil {
			return err
		}
		h.refresh(t)
		if err := h.value(barrier, fmt.Sprint(h.sequence)); err != nil {
			return err
		}
		return check()
	})
}

func (h *fixtureHost) value(key, want string) error {
	item := h.items[key]
	if item == nil || item["state"] != "0" || item["lastclock"] == "0" || item["lastvalue"] != want {
		return fmt.Errorf("%s: want supported value %q, got %v", key, want, item)
	}
	return nil
}

func (h *fixtureHost) unsupported(key, fragment string) error {
	item := h.items[key]
	message, _ := item["error"].(string)
	if item["state"] != "1" || !strings.Contains(message, fragment) {
		return fmt.Errorf("%s: want unsupported containing %q, got %v", key, fragment, item)
	}
	return nil
}

func loadPayload(t *testing.T, path string) object {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result object
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
