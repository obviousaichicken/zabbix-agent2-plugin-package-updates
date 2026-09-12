package apt

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/command"
	"github.com/obviousaichicken/zabbix-agent2-plugin-package-updates/internal/packageinfo"
)

const installedQueryFormat = "${binary:Package}|${Architecture}|${Version}|${db:Status-Status}\\n"

var errRunnerRequired = errors.New("runner is required")

// Runner executes APT and dpkg commands.
type Runner interface {
	Run(context.Context, command.Request) (command.Result, error)
}

// CommandPaths contains the resolved executables used by the APT collector.
type CommandPaths struct {
	APTGet    string
	APTCache  string
	DPKGQuery string
	DPKG      string
}

// PackageData contains the repository/update portion of an APT snapshot.
// Reboot and history data are added by the later collection stage.
type PackageData struct {
	Repositories []packageinfo.Repository
	Updates      []packageinfo.Update
	Metadata     packageinfo.Metadata

	// Installed is the package set enumerated for this collection. Reboot
	// detection reuses it instead of running dpkg-query a second time.
	Installed []InstalledPackage
}

// Client runs bounded, read-only APT package collection.
type Client struct {
	runner         Runner
	paths          CommandPaths
	stat           func(string) (fs.FileInfo, error)
	readFile       func(string) ([]byte, error)
	now            func() time.Time
	policyBudget   int
	rebootMarker   string
	refreshSignals []string
	history        *HistoryReader
}

var _ interface {
	Collect(context.Context) (packageinfo.Snapshot, error)
} = (*Client)(nil)

// New constructs an APT client after resolving every required executable.
func New(runner Runner) (*Client, error) {
	if runner == nil {
		return nil, errRunnerRequired
	}

	paths := CommandPaths{}
	commands := []struct {
		name        string
		destination *string
	}{
		{name: "apt-get", destination: &paths.APTGet},
		{name: "apt-cache", destination: &paths.APTCache},
		{name: "dpkg-query", destination: &paths.DPKGQuery},
		{name: "dpkg", destination: &paths.DPKG},
	}
	for _, candidate := range commands {
		path, err := exec.LookPath(candidate.name)
		if err != nil {
			return nil, fmt.Errorf("find %s: %w", candidate.name, err)
		}
		*candidate.destination = path
	}

	return NewAtPaths(runner, paths)
}

// NewAtPaths constructs an APT client from already-resolved executables.
func NewAtPaths(runner Runner, paths CommandPaths) (*Client, error) {
	return newClient(runner, paths, os.Stat, os.ReadFile, time.Now)
}

func newClientForTest(
	runner Runner,
	paths CommandPaths,
	stat func(string) (fs.FileInfo, error),
	now func() time.Time,
) (*Client, error) {
	return newClient(runner, paths, stat, os.ReadFile, now)
}

func newClient(
	runner Runner,
	paths CommandPaths,
	stat func(string) (fs.FileInfo, error),
	readFile func(string) ([]byte, error),
	now func() time.Time,
) (*Client, error) {
	if runner == nil {
		return nil, errRunnerRequired
	}
	for _, required := range []struct {
		name string
		path string
	}{
		{name: "apt-get", path: paths.APTGet},
		{name: "apt-cache", path: paths.APTCache},
		{name: "dpkg-query", path: paths.DPKGQuery},
		{name: "dpkg", path: paths.DPKG},
	} {
		if required.path == "" {
			return nil, fmt.Errorf("%s path is required", required.name)
		}
	}
	if stat == nil {
		return nil, errors.New("filesystem stat function is required")
	}
	if readFile == nil {
		return nil, errors.New("filesystem read function is required")
	}
	if now == nil {
		return nil, errors.New("clock function is required")
	}

	history, err := newHistoryReader(osHistoryFileSystem{}, defaultHistoryDirectory, time.Local)
	if err != nil {
		return nil, fmt.Errorf("configure APT history reader: %w", err)
	}

	return &Client{
		runner:   runner,
		paths:    paths,
		stat:     stat,
		readFile: readFile,
		now:      now,
		policyBudget: policyArgumentBudget(
			execArgumentLimit(),
			environmentBytes(os.Environ()),
			[]string{paths.APTCache, "policy"},
		),
		rebootMarker:   defaultRebootMarker,
		refreshSignals: defaultRefreshSignals(),
		history:        history,
	}, nil
}

// testSystem replaces every host path and dependency the APT client touches so
// tests never reach the real filesystem.
type testSystem struct {
	runner            Runner
	paths             CommandPaths
	stat              func(string) (fs.FileInfo, error)
	readFile          func(string) ([]byte, error)
	now               func() time.Time
	historyFileSystem historyFileSystem
	historyDirectory  string
	rebootMarker      string
	refreshSignals    []string
	policyBudget      int
	location          *time.Location
}

func newClientWithSystemForTest(system testSystem) (*Client, error) {
	readFile := system.readFile
	if readFile == nil {
		readFile = func(string) ([]byte, error) { return []byte("6.1.0-28-amd64\n"), nil }
	}
	client, err := newClient(system.runner, system.paths, system.stat, readFile, system.now)
	if err != nil {
		return nil, err
	}
	history, err := newHistoryReaderForTest(
		system.historyFileSystem,
		system.historyDirectory,
		system.location,
	)
	if err != nil {
		return nil, err
	}
	if system.rebootMarker == "" {
		return nil, errors.New("reboot marker path is required")
	}
	if len(system.refreshSignals) == 0 {
		return nil, errors.New("at least one refresh signal path is required")
	}
	client.history = history
	client.rebootMarker = system.rebootMarker
	client.refreshSignals = append([]string(nil), system.refreshSignals...)
	if system.policyBudget != 0 {
		client.policyBudget = system.policyBudget
	}

	return client, nil
}

// Collect returns one complete, uncached APT package snapshot.
func (client *Client) Collect(ctx context.Context) (packageinfo.Snapshot, error) {
	data, err := client.Packages(ctx)
	if err != nil {
		return packageinfo.Snapshot{}, fmt.Errorf("collect APT packages: %w", err)
	}
	rebootPending, err := client.RebootPending(ctx, data.Installed)
	if err != nil {
		return packageinfo.Snapshot{}, fmt.Errorf("collect APT reboot status: %w", err)
	}
	lastUpdate, historyWarning, err := client.LastUpdate(ctx)
	if err != nil {
		return packageinfo.Snapshot{}, fmt.Errorf("collect APT update history: %w", err)
	}
	var warnings []string
	if historyWarning != "" {
		warnings = append(warnings, "APT update history unavailable: "+historyWarning)
	}

	return packageinfo.Snapshot{
		Backend: packageinfo.BackendAPT,
		Capabilities: packageinfo.Capabilities{
			Classification: packageinfo.ClassificationCapabilities{
				Security:    packageinfo.CapabilitySupported,
				Bugfix:      packageinfo.CapabilityUnsupported,
				Enhancement: packageinfo.CapabilityUnsupported,
				Other:       packageinfo.CapabilitySupported,
			},
			RepositoryAttribution: packageinfo.CapabilitySupported,
			// Library-only reboots are invisible without
			// /run/reboot-required, whose writers are optional packages.
			RebootDetection: packageinfo.CapabilityBestEffort,
			LastUpdate:      packageinfo.CapabilityBestEffort,
			MetadataAge:     packageinfo.CapabilitySupported,
		},
		Metadata:      data.Metadata,
		Repositories:  data.Repositories,
		Updates:       data.Updates,
		RebootPending: rebootPending,
		LastUpdate:    lastUpdate,
		Warnings:      warnings,
	}, nil
}

// Packages collects enabled repositories, exact candidate policies, pending
// updates, and how long ago APT last refreshed this host's indexes.
func (client *Client) Packages(ctx context.Context) (PackageData, error) {
	nativeArchitecture, err := client.nativeArchitecture(ctx)
	if err != nil {
		return PackageData{}, err
	}

	indexResult, err := client.run(
		ctx,
		"apt-get indextargets",
		client.paths.APTGet,
		[]string{"indextargets"},
		nil,
	)
	if err != nil {
		return PackageData{}, fmt.Errorf("list APT repository indexes: %w", err)
	}
	indexes, err := ParseRepositoryIndexes(indexResult.Stdout)
	if err != nil {
		return PackageData{}, err
	}
	err = client.validateIndexTargets(ctx, indexes.Targets)
	if err != nil {
		return PackageData{}, fmt.Errorf("validate APT package indexes: %w", err)
	}
	metadata, err := client.refreshMetadata(ctx)
	if err != nil {
		return PackageData{}, fmt.Errorf("collect APT index metadata: %w", err)
	}

	installedResult, err := client.run(
		ctx,
		"dpkg-query installed packages",
		client.paths.DPKGQuery,
		[]string{"--show", "--showformat=" + installedQueryFormat},
		nil,
	)
	if err != nil {
		return PackageData{}, fmt.Errorf("list installed packages: %w", err)
	}
	installed, err := ParseInstalledPackages(installedResult.Stdout)
	if err != nil {
		return PackageData{}, err
	}

	policies, err := client.packagePolicies(ctx, installed, indexes, nativeArchitecture)
	if err != nil {
		return PackageData{}, err
	}
	updates, err := client.pendingUpdates(ctx, policies)
	if err != nil {
		return PackageData{}, err
	}

	return PackageData{
		Repositories: indexes.Repositories,
		Updates:      updates,
		Metadata:     metadata,
		Installed:    installed,
	}, nil
}

// nativeArchitecture reports dpkg's native architecture, which apt-cache
// policy needs to resolve unqualified package headers back to the packages
// that were requested.
func (client *Client) nativeArchitecture(ctx context.Context) (string, error) {
	result, err := client.run(
		ctx,
		"dpkg print native architecture",
		client.paths.DPKG,
		[]string{"--print-architecture"},
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("read dpkg native architecture: %w", err)
	}

	architecture := strings.TrimSpace(string(result.Stdout))
	if !validArchitecture(architecture) {
		return "", errors.New("dpkg reported an invalid native architecture")
	}

	return architecture, nil
}

func (client *Client) packagePolicies(
	ctx context.Context,
	installed []InstalledPackage,
	indexes RepositoryIndexes,
	nativeArchitecture string,
) ([]PackagePolicy, error) {
	// Every batch is another apt-cache process that reloads the whole APT
	// cache, so the budget fits as many packages into one invocation as the
	// host's exec limit safely allows.
	argumentBatches, err := BatchPolicyArguments(installed, client.policyBudget)
	if err != nil {
		return nil, err
	}
	policies := make([]PackagePolicy, 0, len(installed))
	offset := 0
	for _, arguments := range argumentBatches {
		args := make([]string, 1, len(arguments)+1)
		args[0] = "policy"
		args = append(args, arguments...)
		result, runErr := client.run(
			ctx,
			fmt.Sprintf("apt-cache policy (%d packages)", len(arguments)),
			client.paths.APTCache,
			args,
			nil,
		)
		if runErr != nil {
			return nil, fmt.Errorf("query APT package policy: %w", runErr)
		}

		batchPolicies, parseErr := ParsePackagePolicies(
			result.Stdout,
			PolicyRequest{
				Packages:           installed[offset : offset+len(arguments)],
				NativeArchitecture: nativeArchitecture,
			},
			indexes,
		)
		if parseErr != nil {
			return nil, parseErr
		}
		policies = append(policies, batchPolicies...)
		offset += len(arguments)
	}
	if offset != len(installed) {
		return nil, errors.New("APT policy batching did not cover every installed package")
	}

	sort.Slice(policies, func(left, right int) bool {
		if policies[left].Name != policies[right].Name {
			return policies[left].Name < policies[right].Name
		}

		return policies[left].Architecture < policies[right].Architecture
	})

	return policies, nil
}

// pendingUpdates derives updates from apt-cache policy alone.
//
// dpkg's database is live, so the installed set enumerated by dpkg-query and
// the policies queried a moment later are two reads of moving state. Comparing
// them and failing on any difference turned a routine unattended-upgrades run
// into a failed check, because collection overlapping a package transaction is
// normal rather than exceptional.
//
// Both versions therefore come from the same apt-cache policy block: apt reads
// /var/lib/dpkg/status itself, so its Installed and Candidate lines are a
// coherent pair by construction and need no cross-check. dpkg-query is reduced
// to choosing which packages to ask about, which makes a package changing
// mid-collection an ordinary snapshot boundary: it is reported as it was seen,
// or omitted, and the next collection picks it up.
func (client *Client) pendingUpdates(
	ctx context.Context,
	policies []PackagePolicy,
) ([]packageinfo.Update, error) {
	updates := make([]packageinfo.Update, 0)
	seen := make(map[string]struct{}, len(policies))
	for _, policy := range policies {
		key := packageKey(policy.Name, policy.Architecture)
		if _, duplicate := seen[key]; duplicate {
			return nil, errors.New("APT policy contains a duplicate package")
		}
		seen[key] = struct{}{}

		// Removed between enumeration and the policy query. A package that
		// is not installed has no pending update.
		if policy.Installed == nil {
			continue
		}
		if policy.Candidate == nil || policy.Candidate.Full == policy.Installed.Full {
			continue
		}

		newer, err := client.candidateIsNewer(ctx, policy, *policy.Candidate)
		if err != nil {
			return nil, err
		}
		if !newer {
			continue
		}
		source, err := preferredCandidateSource(policy.CandidateSources)
		if err != nil {
			return nil, fmt.Errorf(
				"select candidate source for %s:%s: %w",
				policy.Name,
				policy.Architecture,
				err,
			)
		}

		updateType := packageinfo.UpdateTypeOther
		if winningCandidateIsSecurity(policy.CandidateSources) {
			updateType = packageinfo.UpdateTypeSecurity
		}
		update := packageinfo.Update{
			Name:         policy.Name,
			Epoch:        policy.Candidate.Epoch,
			Version:      policy.Candidate.Version,
			Release:      policy.Candidate.Release,
			Arch:         policy.Architecture,
			RepositoryID: source.RepositoryID,
			Type:         updateType,
		}
		packageinfo.SetIdentity(packageinfo.BackendAPT, &update)
		updates = append(updates, update)
	}

	sort.Slice(updates, func(left, right int) bool {
		if updates[left].RepositoryID != updates[right].RepositoryID {
			return updates[left].RepositoryID < updates[right].RepositoryID
		}
		if updates[left].Name != updates[right].Name {
			return updates[left].Name < updates[right].Name
		}

		return updates[left].Arch < updates[right].Arch
	})

	return updates, nil
}

func (client *Client) candidateIsNewer(
	ctx context.Context,
	policy PackagePolicy,
	candidate DebianVersion,
) (bool, error) {
	result, err := client.run(
		ctx,
		"dpkg compare package versions",
		client.paths.DPKG,
		[]string{"--compare-versions", candidate.Full, "gt", policy.Installed.Full},
		[]int{1},
	)
	if err != nil {
		return false, fmt.Errorf(
			"compare package versions for %s:%s: %w",
			policy.Name,
			policy.Architecture,
			err,
		)
	}
	switch result.ExitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	default:
		return false, errors.New("dpkg --compare-versions returned an unexpected exit status")
	}
}

func preferredCandidateSource(sources []PolicySource) (PolicySource, error) {
	if len(sources) == 0 {
		return PolicySource{}, errors.New("candidate has no repository sources")
	}

	preferred := sources[0]
	for _, source := range sources[1:] {
		if source.Priority > preferred.Priority {
			preferred = source
		}
	}

	return preferred, nil
}

func winningCandidateIsSecurity(sources []PolicySource) bool {
	if len(sources) == 0 {
		return false
	}

	highestPriority := sources[0].Priority
	for _, source := range sources[1:] {
		if source.Priority > highestPriority {
			highestPriority = source.Priority
		}
	}
	for _, source := range sources {
		if source.Priority == highestPriority && source.Security {
			return true
		}
	}

	return false
}

// commandEnvironment is the environment every APT command runs with. The
// runner merges it over the inherited environment, so both contribute to the
// exec argument budget.
func commandEnvironment() map[string]string {
	return map[string]string{
		"LC_ALL": "C",
		"LANG":   "C",
	}
}

// environmentBytes estimates what the child process environment costs against
// the same exec limit as the argument list.
func environmentBytes(inherited []string) int {
	total := 0
	for _, entry := range inherited {
		total += len(entry) + 1 + execArgumentPointerBytes
	}
	for name, value := range commandEnvironment() {
		// Overrides may replace an inherited entry rather than add one, so
		// counting them again only ever overestimates the cost.
		total += len(name) + len(value) + 2 + execArgumentPointerBytes
	}

	return total
}

func (client *Client) run(
	ctx context.Context,
	operation string,
	path string,
	args []string,
	acceptedExitCodes []int,
) (command.Result, error) {
	result, err := client.runner.Run(ctx, command.Request{
		Name:              path,
		Args:              args,
		AcceptedExitCodes: acceptedExitCodes,
		Env:               commandEnvironment(),
	})
	if err != nil {
		return result, &CommandError{
			operation:  operation,
			exitStatus: result.ExitCode,
			diagnostic: command.Diagnostic(result.Stderr),
			err:        err,
		}
	}

	return result, nil
}
