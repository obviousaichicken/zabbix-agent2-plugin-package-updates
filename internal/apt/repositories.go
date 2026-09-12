// Package apt implements the read-only Debian and Ubuntu package collector.
package apt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/packageinfo"
)

// RepositoryIndexes contains the logical repositories and enabled binary
// package indexes reported by apt-get indextargets.
type RepositoryIndexes struct {
	Repositories []packageinfo.Repository
	Targets      []IndexTarget
}

// IndexTarget describes one enabled binary Packages index. Source is the
// canonical, credential-free form used to match apt-cache policy output.
type IndexTarget struct {
	RepositoryID string
	Source       string
	Filename     string
	Architecture string
	Component    string
	Origin       string
	Label        string
	Suite        string
	Codename     string
	Trusted      bool
	Security     bool
}

// RepositoryParser parses apt repository index metadata. It has no mutable
// package state, so tests can inject an ID function without changing runtime
// behavior.
type RepositoryParser struct {
	repositoryID func(string) string
}

// NewRepositoryParser constructs a production repository parser.
func NewRepositoryParser() *RepositoryParser {
	return &RepositoryParser{repositoryID: hashedRepositoryID}
}

func newRepositoryParserForTest(repositoryID func(string) string) *RepositoryParser {
	return &RepositoryParser{repositoryID: repositoryID}
}

// ParseRepositoryIndexes parses repository metadata with the production ID
// generator.
func ParseRepositoryIndexes(data []byte) (RepositoryIndexes, error) {
	return NewRepositoryParser().Parse(data)
}

// Parse validates and normalizes enabled binary package index targets.
//

// Parse validates and normalizes enabled binary package index targets.
func (parser *RepositoryParser) Parse(data []byte) (RepositoryIndexes, error) {
	if parser == nil || parser.repositoryID == nil {
		return RepositoryIndexes{}, errors.New("APT repository parser is not configured")
	}

	records, err := parseDeb822(data)
	if err != nil {
		return RepositoryIndexes{}, fmt.Errorf("parse APT repository indexes: %w", err)
	}

	grouped := newRepositoryGrouping(parser.repositoryID, len(records))
	for recordNumber, record := range records {
		target, identity, enabled, targetErr := repositoryTarget(record)
		if targetErr != nil {
			return RepositoryIndexes{}, fmt.Errorf(
				"parse APT repository index record %d: %w",
				recordNumber+1,
				targetErr,
			)
		}
		if !enabled {
			continue
		}
		if addErr := grouped.add(target, identity); addErr != nil {
			return RepositoryIndexes{}, addErr
		}
	}

	return grouped.finish(), nil
}

// repositoryGrouping collects index targets into logical repositories. The
// identity, component and source bookkeeping is several maps that only make
// sense together, so they travel together rather than through one long loop.
type repositoryGrouping struct {
	repositoryID   func(string) string
	byIdentity     map[string]packageinfo.Repository
	identityByID   map[string]string
	representative map[string]IndexTarget
	components     map[string]map[string]struct{}
	sourceOwners   map[string]string
	result         RepositoryIndexes
}

func newRepositoryGrouping(repositoryID func(string) string, capacity int) *repositoryGrouping {
	return &repositoryGrouping{
		repositoryID:   repositoryID,
		byIdentity:     make(map[string]packageinfo.Repository),
		identityByID:   make(map[string]string),
		representative: make(map[string]IndexTarget),
		components:     make(map[string]map[string]struct{}),
		sourceOwners:   make(map[string]string),
		result: RepositoryIndexes{
			Repositories: make([]packageinfo.Repository, 0, capacity),
			Targets:      make([]IndexTarget, 0, capacity),
		},
	}
}

func (grouping *repositoryGrouping) add(target IndexTarget, identity string) error {
	repository, exists := grouping.byIdentity[identity]
	if !exists {
		var err error
		repository, err = grouping.register(identity, target)
		if err != nil {
			return err
		}
	}
	if target.Component != "" {
		grouping.components[repository.ID][target.Component] = struct{}{}
	}

	if owner, duplicate := grouping.sourceOwners[target.Source]; duplicate && owner != repository.ID {
		return errors.New("APT repository source maps to multiple logical repositories")
	}
	grouping.sourceOwners[target.Source] = repository.ID
	target.RepositoryID = repository.ID
	grouping.result.Targets = append(grouping.result.Targets, target)

	return nil
}

func (grouping *repositoryGrouping) register(
	identity string,
	target IndexTarget,
) (packageinfo.Repository, error) {
	repository := packageinfo.Repository{ID: grouping.repositoryID(identity)}
	if repository.ID == "" {
		return packageinfo.Repository{}, errors.New("APT repository ID generator returned an empty ID")
	}
	if owner, collision := grouping.identityByID[repository.ID]; collision && owner != identity {
		return packageinfo.Repository{}, fmt.Errorf("APT repository ID collision for %q", repository.ID)
	}
	grouping.identityByID[repository.ID] = identity
	grouping.byIdentity[identity] = repository
	grouping.representative[repository.ID] = target
	grouping.components[repository.ID] = make(map[string]struct{})
	grouping.result.Repositories = append(grouping.result.Repositories, repository)

	return repository, nil
}

// finish names every repository from its components and returns the grouping
// in a deterministic order.
func (grouping *repositoryGrouping) finish() RepositoryIndexes {
	result := grouping.result
	for index := range result.Repositories {
		repository := &result.Repositories[index]
		components := make([]string, 0, len(grouping.components[repository.ID]))
		for component := range grouping.components[repository.ID] {
			components = append(components, component)
		}
		sort.Strings(components)
		repository.Name = repositoryName(
			grouping.representative[repository.ID],
			grouping.identityByID[repository.ID],
			components,
		)
	}

	sort.Slice(result.Repositories, func(left, right int) bool {
		return result.Repositories[left].ID < result.Repositories[right].ID
	})
	sort.Slice(result.Targets, func(left, right int) bool {
		leftTarget := result.Targets[left]
		rightTarget := result.Targets[right]
		if leftTarget.Source != rightTarget.Source {
			return leftTarget.Source < rightTarget.Source
		}
		if leftTarget.Architecture != rightTarget.Architecture {
			return leftTarget.Architecture < rightTarget.Architecture
		}
		if leftTarget.Component != rightTarget.Component {
			return leftTarget.Component < rightTarget.Component
		}

		return leftTarget.Filename < rightTarget.Filename
	})

	return result
}

func repositoryTarget(record deb822Record) (IndexTarget, string, bool, error) {
	if record["identifier"] != "Packages" ||
		record["created-by"] != "Packages" ||
		record["target-of"] != "deb" {
		return IndexTarget{}, "", false, nil
	}

	enabled, err := yesNoField(record, "defaultenabled")
	if err != nil || !enabled {
		return IndexTarget{}, "", false, err
	}
	trusted, err := yesNoField(record, "trusted")
	if err != nil {
		return IndexTarget{}, "", false, err
	}

	description, sourceURL, err := sanitizedDescription(record["description"])
	if err != nil {
		return IndexTarget{}, "", false, errors.New("invalid Description repository URL")
	}
	for _, field := range []string{"uri", "base-uri", "repo-uri", "site"} {
		if record[field] == "" {
			continue
		}
		if _, sanitizeErr := sanitizedRepositoryURL(record[field]); sanitizeErr != nil {
			return IndexTarget{}, "", false, fmt.Errorf("invalid %s repository URL", field)
		}
	}

	target := IndexTarget{
		Source:       description,
		Filename:     record["filename"],
		Architecture: record["architecture"],
		Component:    record["component"],
		Origin:       record["origin"],
		Label:        record["label"],
		Suite:        record["suite"],
		Codename:     record["codename"],
		Trusted:      trusted,
	}
	if err := validateTargetFields(target); err != nil {
		return IndexTarget{}, "", false, err
	}
	target.Filename = filepath.Clean(target.Filename)

	fallbackURI, err := repositoryFallbackURL(record, sourceURL)
	if err != nil {
		return IndexTarget{}, "", false, err
	}
	identity := repositoryIdentity(target, fallbackURI)
	target.Security = isSecurityRepository(target)

	return target, identity, true, nil
}

// validateTargetFields rejects a record that cannot describe a usable index:
// a missing required field, a value spanning lines, or a relative filename.
func validateTargetFields(target IndexTarget) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "Filename", value: target.Filename},
		{name: "Architecture", value: target.Architecture},
	} {
		if field.value == "" {
			return fmt.Errorf("required field %s is empty", field.name)
		}
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "Description", value: target.Source},
		{name: "Filename", value: target.Filename},
		{name: "Architecture", value: target.Architecture},
		{name: "Component", value: target.Component},
		{name: "Origin", value: target.Origin},
		{name: "Label", value: target.Label},
		{name: "Suite", value: target.Suite},
		{name: "Codename", value: target.Codename},
	} {
		if strings.ContainsAny(field.value, "\r\n") {
			return fmt.Errorf("field %s must be a single line", field.name)
		}
	}
	if !filepath.IsAbs(target.Filename) {
		return errors.New("field Filename must be an absolute path")
	}

	return nil
}

func yesNoField(record deb822Record, name string) (bool, error) {
	switch record[name] {
	case "yes":
		return true, nil
	case "no":
		return false, nil
	case "":
		return false, fmt.Errorf("required field %s is empty", name)
	default:
		return false, fmt.Errorf("field %s must be yes or no", name)
	}
}

func sanitizedDescription(description string) (string, string, error) {
	fields := strings.Fields(description)
	if len(fields) < 2 {
		return "", "", errors.New("incomplete description")
	}
	sourceURL, err := sanitizedRepositoryURL(fields[0])
	if err != nil {
		return "", "", err
	}
	fields[0] = sourceURL

	return strings.Join(fields, " "), sourceURL, nil
}

func sanitizedRepositoryURL(raw string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\x00\r\n\t ") {
		return "", errors.New("invalid repository URL")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" {
		return "", errors.New("invalid repository URL")
	}
	if (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) &&
		parsed.Hostname() == "" {
		return "", errors.New("invalid repository URL")
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""

	return parsed.String(), nil
}

func repositoryFallbackURL(record deb822Record, descriptionURL string) (string, error) {
	for _, name := range []string{"repo-uri", "site", "base-uri", "uri"} {
		if record[name] == "" {
			continue
		}
		sanitized, err := sanitizedRepositoryURL(record[name])
		if err != nil {
			return "", fmt.Errorf("invalid %s repository URL", name)
		}

		return strings.TrimRight(sanitized, "/"), nil
	}

	return strings.TrimRight(descriptionURL, "/"), nil
}

func repositoryIdentity(target IndexTarget, fallbackURL string) string {
	parts := []string{target.Origin, target.Label, target.Suite, target.Codename}
	complete := true
	for _, part := range parts {
		if part == "" {
			complete = false
			break
		}
	}
	if !complete {
		parts = append(parts, fallbackURL)
	}

	return strings.Join(parts, "\x00")
}

func hashedRepositoryID(identity string) string {
	digest := sha256.Sum256([]byte(identity))

	return "apt-" + hex.EncodeToString(digest[:16])
}

func repositoryName(target IndexTarget, identity string, components []string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{target.Origin, target.Label} {
		if part == "" {
			continue
		}
		if len(parts) == 0 || !strings.EqualFold(parts[len(parts)-1], part) {
			parts = append(parts, part)
		}
	}
	if target.Suite != "" {
		parts = append(parts, "("+target.Suite+")")
	}
	name := strings.Join(parts, " ")
	if name == "" {
		identityParts := strings.Split(identity, "\x00")
		name = identityParts[len(identityParts)-1]
	}
	if len(components) != 0 {
		name += " [" + strings.Join(components, ", ") + "]"
	}

	return name
}

func isSecurityRepository(target IndexTarget) bool {
	if !target.Trusted {
		return false
	}

	switch {
	case target.Origin == "Debian" && target.Label == "Debian-Security":
		return strings.HasSuffix(target.Suite, "-security") &&
			strings.HasSuffix(target.Codename, "-security")
	case target.Origin == "Ubuntu" && target.Label == "Ubuntu":
		return strings.HasSuffix(target.Suite, "-security")
	case target.Origin == "UbuntuESMApps" && target.Label == "UbuntuESMApps":
		return strings.HasSuffix(target.Suite, "-apps-security")
	case target.Origin == "UbuntuESM" && target.Label == "UbuntuESM":
		return strings.HasSuffix(target.Suite, "-infra-security")
	default:
		return false
	}
}
