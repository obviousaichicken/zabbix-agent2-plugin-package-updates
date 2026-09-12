package results

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/packageinfo"
)

const PackageSchemaVersion = 1

// PackagePayload is the package-manager-neutral packages.get response.
type PackagePayload struct {
	SchemaVersion  int                 `json:"schema_version"`
	Backend        string              `json:"backend"`
	Capabilities   PackageCapabilities `json:"capabilities"`
	Metadata       PackageMetadata     `json:"metadata"`
	Collection     Collection          `json:"collection"`
	Classification Classification      `json:"classification"`
	Summary        Summary             `json:"summary"`
	Repositories   []Repository        `json:"repositories"`
	Updates        []PackageUpdate     `json:"updates"`
}

// PackageCapabilities serializes generic backend fidelity.
type PackageCapabilities struct {
	Classification        PackageClassificationCapabilities `json:"classification"`
	RepositoryAttribution string                            `json:"repository_attribution"`
	RebootDetection       string                            `json:"reboot_detection"`
	LastUpdate            string                            `json:"last_update"`
	MetadataAge           string                            `json:"metadata_age"`
}

// PackageClassificationCapabilities serializes classification fidelity.
type PackageClassificationCapabilities struct {
	Security    string `json:"security"`
	Bugfix      string `json:"bugfix"`
	Enhancement string `json:"enhancement"`
	Other       string `json:"other"`
}

// PackageMetadata describes the oldest participating package index.
type PackageMetadata struct {
	RefreshedAt *time.Time `json:"refreshed_at"`
	AgeSeconds  *int64     `json:"age_seconds"`
}

// PackageUpdate contains generic update details.
type PackageUpdate struct {
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`
	Epoch        string `json:"epoch"`
	Version      string `json:"version"`
	Release      string `json:"release"`
	Arch         string `json:"arch"`
	Type         string `json:"type"`
	FullVersion  string `json:"full_version"`
	Identifier   string `json:"identifier"`
}

// BuildPackages validates and builds a deterministic packages.get payload.
func BuildPackages(snapshot packageinfo.Snapshot) (PackagePayload, error) {
	if err := validatePackageSnapshot(snapshot); err != nil {
		return PackagePayload{}, err
	}

	repositories, updates := buildPackageItems(snapshot)

	return PackagePayload{
		SchemaVersion: PackageSchemaVersion,
		Backend:       snapshot.Backend.String(),
		Capabilities:  newPackageCapabilities(snapshot.Capabilities),
		Metadata: PackageMetadata{
			RefreshedAt: snapshot.Metadata.RefreshedAt,
			AgeSeconds:  snapshot.Metadata.AgeSeconds,
		},
		Collection: Collection{Complete: true},
		Classification: Classification{
			Complete:         true,
			FailedCategories: make([]string, 0),
		},
		Summary: Summary{
			Repositories:   len(snapshot.Repositories),
			Updates:        len(snapshot.Updates),
			UpdatesPending: len(snapshot.Updates) > 0,
			RebootPending:  snapshot.RebootPending,
			UpdateTypes:    countPackageUpdateTypes(snapshot.Updates),
			LastUpdate:     NewLastUpdate(snapshot.LastUpdate),
		},
		Repositories: repositories,
		Updates:      updates,
	}, nil
}

// buildPackageItems converts repositories and updates to payload form, counts
// updates per repository, and orders both deterministically.
func buildPackageItems(snapshot packageinfo.Snapshot) ([]Repository, []PackageUpdate) {
	repositories := make([]Repository, 0, len(snapshot.Repositories))
	updates := make([]PackageUpdate, 0, len(snapshot.Updates))
	updateCounts := make(map[string]int, len(snapshot.Repositories))

	for _, repository := range snapshot.Repositories {
		repositories = append(repositories, Repository{ID: repository.ID, Name: repository.Name})
	}
	for _, update := range snapshot.Updates {
		updateCounts[update.RepositoryID]++
		updates = append(updates, PackageUpdate{
			RepositoryID: update.RepositoryID,
			Name:         update.Name,
			Epoch:        update.Epoch,
			Version:      update.Version,
			Release:      update.Release,
			Arch:         update.Arch,
			Type:         update.Type.String(),
			FullVersion:  update.FullVersion,
			Identifier:   update.Identifier,
		})
	}
	for index := range repositories {
		repositories[index].UpdateCount = updateCounts[repositories[index].ID]
	}

	sort.Slice(repositories, func(i, j int) bool {
		return repositories[i].ID < repositories[j].ID
	})
	sort.Slice(updates, func(i, j int) bool {
		if updates[i].RepositoryID != updates[j].RepositoryID {
			return updates[i].RepositoryID < updates[j].RepositoryID
		}
		if updates[i].Name != updates[j].Name {
			return updates[i].Name < updates[j].Name
		}

		return updates[i].Arch < updates[j].Arch
	})

	return repositories, updates
}

func validatePackageSnapshot(snapshot packageinfo.Snapshot) error {
	if err := snapshot.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid package snapshot: %w", err)
	}
	if err := validateBackendCapabilities(snapshot); err != nil {
		return err
	}
	if err := validatePackageMetadata(snapshot); err != nil {
		return err
	}
	if err := validateSnapshotLastUpdate(snapshot.LastUpdate); err != nil {
		return err
	}

	return validateSnapshotUpdates(snapshot)
}

func validateSnapshotLastUpdate(lastUpdate *packageinfo.LastUpdate) error {
	if lastUpdate == nil {
		return nil
	}
	if lastUpdate.Timestamp.IsZero() {
		return errors.New("last update timestamp is required")
	}
	if lastUpdate.Result != packageinfo.LastUpdateResultSuccess &&
		lastUpdate.Result != packageinfo.LastUpdateResultFailed {
		return fmt.Errorf("invalid last update result %q", lastUpdate.Result)
	}

	return nil
}

func validateSnapshotUpdates(snapshot packageinfo.Snapshot) error {
	seenAPTUpdates := make(map[string]struct{}, len(snapshot.Updates))
	for _, update := range snapshot.Updates {
		if err := validateSnapshotUpdate(snapshot, update); err != nil {
			return err
		}
		if snapshot.Backend != packageinfo.BackendAPT {
			continue
		}
		// APT addresses a package by name and architecture, so the same
		// name may legitimately appear twice but the pair may not.
		key := update.Name + "\x00" + update.Arch
		if _, exists := seenAPTUpdates[key]; exists {
			return fmt.Errorf("duplicate APT update %s:%s", update.Name, update.Arch)
		}
		seenAPTUpdates[key] = struct{}{}
	}

	return nil
}

func validateSnapshotUpdate(snapshot packageinfo.Snapshot, update packageinfo.Update) error {
	if update.Name == "" || update.Arch == "" || update.Version == "" {
		return fmt.Errorf("update %q has incomplete package identity", update.Name)
	}
	if snapshot.Backend == packageinfo.BackendDNF && update.Release == "" {
		return fmt.Errorf("DNF update %q has no release", update.Name)
	}
	if update.FullVersion != packageinfo.FullVersion(update) {
		return fmt.Errorf("update %q has inconsistent full version", update.Name)
	}
	if update.Identifier != packageinfo.Identifier(snapshot.Backend, update) {
		return fmt.Errorf("update %q has inconsistent identifier", update.Name)
	}
	if classificationCapability(snapshot.Capabilities.Classification, update.Type) ==
		packageinfo.CapabilityUnsupported {
		return fmt.Errorf("update %q uses unsupported classification %s", update.Name, update.Type)
	}

	return nil
}

func validateBackendCapabilities(snapshot packageinfo.Snapshot) error {
	capabilities := snapshot.Capabilities
	if capabilities.RepositoryAttribution != packageinfo.CapabilitySupported {
		return errors.New("repository attribution must be supported")
	}

	switch snapshot.Backend {
	case packageinfo.BackendDNF:
		return validateDNFCapabilities(capabilities)
	case packageinfo.BackendAPT:
		return validateAPTCapabilities(capabilities)
	case packageinfo.BackendUnknown:
		return errors.New("unknown package backend")
	}

	return nil
}

func validateDNFCapabilities(capabilities packageinfo.Capabilities) error {
	classification := capabilities.Classification
	if classification.Security != packageinfo.CapabilitySupported ||
		classification.Bugfix != packageinfo.CapabilitySupported ||
		classification.Enhancement != packageinfo.CapabilitySupported ||
		classification.Other != packageinfo.CapabilitySupported ||
		capabilities.RebootDetection != packageinfo.CapabilitySupported ||
		capabilities.LastUpdate != packageinfo.CapabilitySupported ||
		capabilities.MetadataAge != packageinfo.CapabilityUnsupported {
		return errors.New("invalid DNF capability combination")
	}

	return nil
}

// APT reboot detection is best effort: a newer installed kernel is always
// visible, but library-only reboots need /run/reboot-required, which only
// optional packages write.
func validateAPTCapabilities(capabilities packageinfo.Capabilities) error {
	classification := capabilities.Classification
	if classification.Security != packageinfo.CapabilitySupported ||
		classification.Bugfix != packageinfo.CapabilityUnsupported ||
		classification.Enhancement != packageinfo.CapabilityUnsupported ||
		classification.Other != packageinfo.CapabilitySupported ||
		capabilities.RebootDetection != packageinfo.CapabilityBestEffort ||
		capabilities.LastUpdate != packageinfo.CapabilityBestEffort ||
		capabilities.MetadataAge != packageinfo.CapabilitySupported {
		return errors.New("invalid APT capability combination")
	}

	return nil
}

func validatePackageMetadata(snapshot packageinfo.Snapshot) error {
	refreshedAt := snapshot.Metadata.RefreshedAt
	ageSeconds := snapshot.Metadata.AgeSeconds
	if snapshot.Capabilities.MetadataAge == packageinfo.CapabilityUnsupported {
		if refreshedAt != nil || ageSeconds != nil {
			return errors.New("metadata age fields present when unsupported")
		}

		return nil
	}
	if refreshedAt == nil || refreshedAt.IsZero() || ageSeconds == nil || *ageSeconds < 0 {
		return errors.New("supported metadata age is incomplete")
	}

	return nil
}

func classificationCapability(
	capabilities packageinfo.ClassificationCapabilities,
	updateType packageinfo.UpdateType,
) packageinfo.Capability {
	switch updateType {
	case packageinfo.UpdateTypeSecurity:
		return capabilities.Security
	case packageinfo.UpdateTypeBugfix:
		return capabilities.Bugfix
	case packageinfo.UpdateTypeEnhancement:
		return capabilities.Enhancement
	case packageinfo.UpdateTypeOther:
		return capabilities.Other
	case packageinfo.UpdateTypeUnknown:
		return packageinfo.CapabilityUnknown
	default:
		return packageinfo.CapabilityUnknown
	}
}

func newPackageCapabilities(capabilities packageinfo.Capabilities) PackageCapabilities {
	return PackageCapabilities{
		Classification: PackageClassificationCapabilities{
			Security:    capabilities.Classification.Security.String(),
			Bugfix:      capabilities.Classification.Bugfix.String(),
			Enhancement: capabilities.Classification.Enhancement.String(),
			Other:       capabilities.Classification.Other.String(),
		},
		RepositoryAttribution: capabilities.RepositoryAttribution.String(),
		RebootDetection:       capabilities.RebootDetection.String(),
		LastUpdate:            capabilities.LastUpdate.String(),
		MetadataAge:           capabilities.MetadataAge.String(),
	}
}

func countPackageUpdateTypes(updates []packageinfo.Update) UpdateTypeCounts {
	var counts UpdateTypeCounts
	for _, update := range updates {
		switch update.Type {
		case packageinfo.UpdateTypeSecurity:
			counts.Security++
		case packageinfo.UpdateTypeBugfix:
			counts.Bugfix++
		case packageinfo.UpdateTypeEnhancement:
			counts.Enhancement++
		case packageinfo.UpdateTypeOther:
			counts.Other++
		case packageinfo.UpdateTypeUnknown:
			// Validation rejects this before aggregation.
		}
	}

	return counts
}
