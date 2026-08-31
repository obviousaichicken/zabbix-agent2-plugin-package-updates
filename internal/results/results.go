// Package results builds the structured payload returned by the plugin.
package results

import (
	"time"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/packageinfo"
)

// Collection records collection status and duration.
type Collection struct {
	Complete   bool  `json:"complete"`
	DurationMS int64 `json:"duration_ms"` //nolint:tagliatelle // JSON schema uses snake_case.
}

// Classification records advisory category completeness.
type Classification struct {
	Complete         bool     `json:"complete"`
	FailedCategories []string `json:"failed_categories"` //nolint:tagliatelle // JSON schema uses snake_case.
}

// Summary contains collection counts.
type Summary struct {
	Repositories   int              `json:"repositories"`
	Updates        int              `json:"updates"`
	UpdatesPending bool             `json:"updates_pending"`
	RebootPending  bool             `json:"reboot_pending"`
	UpdateTypes    UpdateTypeCounts `json:"update_types"`
	LastUpdate     LastUpdate       `json:"last_update"`
}

// UpdateTypeCounts contains advisory category counts.
type UpdateTypeCounts struct {
	Security    int `json:"security"`
	Bugfix      int `json:"bugfix"`
	Enhancement int `json:"enhancement"`
	Other       int `json:"other"`
}

// LastUpdate describes the most recent completed transaction that upgraded a package.
type LastUpdate struct {
	Timestamp *time.Time `json:"timestamp"`
	Result    string     `json:"result"`
}

// NewLastUpdate converts a neutral last update to payload form.
func NewLastUpdate(update *packageinfo.LastUpdate) LastUpdate {
	if update == nil {
		return LastUpdate{Result: packageinfo.LastUpdateResultNotRecorded}
	}

	return LastUpdate{
		Timestamp: &update.Timestamp,
		Result:    update.Result,
	}
}

// Repository contains a repository and its update count.
type Repository struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UpdateCount int    `json:"update_count"` //nolint:tagliatelle // JSON schema uses snake_case.
}
