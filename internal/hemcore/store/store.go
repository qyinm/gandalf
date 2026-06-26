package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/qyinm/hem/internal/hemcore/fsutil"
	"github.com/qyinm/hem/internal/hemcore/types"
)

const (
	contentDir         = "content"
	timelineEventsDir  = "timeline/events"
	storeMode          = 0o700
)

var agentStoreDirs = []types.AgentID{
	types.AgentClaudeCode,
	types.AgentCodex,
	types.AgentCursor,
	types.AgentOpencode,
	types.AgentPiAgent,
	types.AgentProject,
	types.AgentUnknown,
}

var safeAgentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type ChecksumRecord struct {
	SourcePath string `json:"sourcePath"`
	Checksum   string `json:"checksum"`
}

type StoreSnapshot struct {
	Manifest      types.SnapshotManifest
	Evidence      []types.DiscoveredItem
	Graph         []types.GraphNode
	AuditFindings []types.AuditFinding
	Provenance    []types.ProvenanceEntry
	Content       []types.SnapshotContentEntry
	Checksums     map[string]ChecksumRecord
	Redactions    []json.RawMessage
}

func StoreSnapshotFrom(snapshot types.Snapshot) StoreSnapshot {
	return StoreSnapshot{
		Manifest:      snapshot.Manifest,
		Evidence:      snapshot.Evidence,
		Graph:         snapshot.Graph,
		AuditFindings: snapshot.AuditFindings,
		Provenance:    snapshot.Provenance,
		Content:       snapshot.Content,
	}
}

type TimelineListOptions struct {
	Agent            *types.AgentID
	ProjectPath      string
	Limit            *int
	OnCorruptEntry   func(TimelineCorruptEvent)
}

type TimelineCorruptEvent struct {
	FilePath string
	Error    string
}

type StoreError struct {
	UnsafeSnapshotName *string
	UnsafeContentPath  *string
	IO                 error
	JSON               error
}

func (e *StoreError) Error() string {
	switch {
	case e.UnsafeSnapshotName != nil:
		return fmt.Sprintf("Unsafe snapshot name: %q", *e.UnsafeSnapshotName)
	case e.UnsafeContentPath != nil:
		return fmt.Sprintf("Unsafe snapshot content path: %q", *e.UnsafeContentPath)
	case e.JSON != nil:
		return e.JSON.Error()
	case e.IO != nil:
		return e.IO.Error()
	default:
		return "store error"
	}
}

func (e *StoreError) Unwrap() error {
	if e.JSON != nil {
		return e.JSON
	}
	return e.IO
}

func DefaultStoreDir(homeDir string) string {
	return filepath.Join(homeDir, ".hem")
}

func AgentStoreDir(storeDir string, agent *types.AgentID) string {
	if agent == nil {
		return storeDir
	}
	return filepath.Join(storeDir, agent.String())
}

func snapshotDir(storeDir, name string, agent *types.AgentID) string {
	return filepath.Join(AgentStoreDir(storeDir, agent), name)
}

func EnsureStore(storeDir string) ([]types.AuditFinding, error) {
	existed, err := pathExists(storeDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(storeDir, storeMode); err != nil {
		return nil, &StoreError{IO: err}
	}
	if !existed {
		if err := setMode(storeDir, storeMode); err != nil {
			return nil, err
		}
	}

	mode, err := fileMode(storeDir)
	if err != nil {
		return nil, err
	}
	if mode&0o022 == 0 {
		return nil, nil
	}

	path := storeDir
	return []types.AuditFinding{{
		Code:     "WORLD_WRITABLE_STORE",
		Severity: types.SeverityHigh,
		Problem:  "The local hem snapshot store is writable by group or world.",
		Cause:    fmt.Sprintf("Store permissions are %o.", mode),
		Fix:      "Restrict the store directory to the current user with chmod 700.",
		Path:     &path,
	}}, nil
}

func ListAgents(storeDir string) ([]types.AgentID, error) {
	exists, err := pathExists(storeDir)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	entries, err := os.ReadDir(storeDir)
	if err != nil {
		return nil, &StoreError{IO: err}
	}

	var agents []types.AgentID
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSafeAgentName(name) {
			continue
		}
		agent := types.ParseAgentID(name)
		if !containsAgent(agentStoreDirs, agent) {
			continue
		}
		sub, err := os.ReadDir(filepath.Join(storeDir, name))
		if err != nil {
			return nil, &StoreError{IO: err}
		}
		hasSnapshot := false
		for _, child := range sub {
			if child.IsDir() {
				hasSnapshot = true
				break
			}
		}
		if hasSnapshot {
			agents = append(agents, agent)
		}
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].String() < agents[j].String()
	})
	return agents, nil
}

func WriteSnapshot(storeDir string, snapshot StoreSnapshot, agent *types.AgentID) error {
	name, err := validateSnapshotName(snapshot.Manifest.Name)
	if err != nil {
		return err
	}
	dir := snapshotDir(storeDir, name, agent)

	if _, err := EnsureStore(storeDir); err != nil {
		return err
	}
	if agent != nil {
		if _, err := EnsureStore(AgentStoreDir(storeDir, agent)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, storeMode); err != nil {
		return &StoreError{IO: err}
	}
	if err := setMode(dir, storeMode); err != nil {
		return err
	}

	checksums := snapshot.Checksums
	if checksums == nil {
		checksums = checksumsFromEvidence(snapshot.Evidence)
	}
	redactions := snapshot.Redactions
	if redactions == nil {
		redactions = []json.RawMessage{}
	}

	if err := writeJSONAtomic(filepath.Join(dir, "manifest.json"), snapshot.Manifest); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "evidence.json"), snapshot.Evidence); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "graph.json"), snapshot.Graph); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "audit-findings.json"), snapshot.AuditFindings); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "provenance.json"), snapshot.Provenance); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "checksums.json"), checksums); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "redactions.json"), redactions); err != nil {
		return err
	}

	if len(snapshot.Content) > 0 {
		contentPath := filepath.Join(dir, contentDir)
		if pathExistsBool(contentPath) {
			if err := os.RemoveAll(contentPath); err != nil {
				return &StoreError{IO: err}
			}
		}
		if err := os.MkdirAll(contentPath, storeMode); err != nil {
			return &StoreError{IO: err}
		}
		if err := setMode(contentPath, storeMode); err != nil {
			return err
		}

		for _, entry := range snapshot.Content {
			if entry.CaptureStatus != string(types.CaptureCaptured) {
				continue
			}
			if entry.Content == nil {
				continue
			}
			prefix := contentDir + "/"
			if !isSafeSnapshotRelativePath(entry.StoragePath) || !strings.HasPrefix(entry.StoragePath, prefix) {
				unsafePath := entry.StoragePath
				return &StoreError{UnsafeContentPath: &unsafePath}
			}
			target := filepath.Join(dir, filepath.FromSlash(entry.StoragePath))
			if err := os.MkdirAll(filepath.Dir(target), storeMode); err != nil {
				return &StoreError{IO: err}
			}
			if err := writeTextAtomic(target, *entry.Content); err != nil {
				return err
			}
		}

		index := make([]types.SnapshotContentEntry, len(snapshot.Content))
		for i, entry := range snapshot.Content {
			index[i] = entry
			index[i].Content = nil
		}
		if err := writeJSONAtomic(filepath.Join(dir, "content-index.json"), index); err != nil {
			return err
		}
	}

	return nil
}

func ReadSnapshot(storeDir, name string, agent *types.AgentID) (types.Snapshot, error) {
	safeName, err := validateSnapshotName(name)
	if err != nil {
		return types.Snapshot{}, err
	}
	dir := snapshotDir(storeDir, safeName, agent)

	var manifest types.SnapshotManifest
	if err := readJSON(filepath.Join(dir, "manifest.json"), &manifest); err != nil {
		return types.Snapshot{}, err
	}
	var evidence []types.DiscoveredItem
	if err := readJSON(filepath.Join(dir, "evidence.json"), &evidence); err != nil {
		return types.Snapshot{}, err
	}
	var graph []types.GraphNode
	if err := readJSON(filepath.Join(dir, "graph.json"), &graph); err != nil {
		return types.Snapshot{}, err
	}
	var auditFindings []types.AuditFinding
	if err := readJSON(filepath.Join(dir, "audit-findings.json"), &auditFindings); err != nil {
		return types.Snapshot{}, err
	}
	var provenance []types.ProvenanceEntry
	if err := readJSON(filepath.Join(dir, "provenance.json"), &provenance); err != nil {
		return types.Snapshot{}, err
	}
	content, err := readOptionalJSON[[]types.SnapshotContentEntry](filepath.Join(dir, "content-index.json"))
	if err != nil {
		return types.Snapshot{}, err
	}

	snapshot := types.Snapshot{
		Manifest:      manifest,
		Evidence:      evidence,
		Graph:         graph,
		AuditFindings: auditFindings,
		Provenance:    provenance,
	}
	if content != nil {
		snapshot.Content = *content
	}
	return snapshot, nil
}

func ReadSnapshotContent(storeDir, name string, entry types.SnapshotContentEntry, agent *types.AgentID) (string, error) {
	safeName, err := validateSnapshotName(name)
	if err != nil {
		return "", err
	}
	prefix := contentDir + "/"
	if !isSafeSnapshotRelativePath(entry.StoragePath) || !strings.HasPrefix(entry.StoragePath, prefix) {
		unsafePath := entry.StoragePath
		return "", &StoreError{UnsafeContentPath: &unsafePath}
	}
	path := filepath.Join(snapshotDir(storeDir, safeName, agent), filepath.FromSlash(entry.StoragePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", &StoreError{IO: err}
	}
	return string(data), nil
}

func ListSnapshots(storeDir string, agent *types.AgentID) ([]string, error) {
	baseDir := AgentStoreDir(storeDir, agent)
	exists, err := pathExists(baseDir)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, &StoreError{IO: err}
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSafeSnapshotName(name) {
			continue
		}
		if agent != nil {
			names = append(names, name)
		} else if pathExistsBool(filepath.Join(baseDir, name, "manifest.json")) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func SnapshotExists(storeDir, name string, agent *types.AgentID) (bool, error) {
	safeName, err := validateSnapshotName(name)
	if err != nil {
		return false, err
	}
	return pathExistsBool(filepath.Join(snapshotDir(storeDir, safeName, agent), "manifest.json")), nil
}

func AppendTimelineEntry(storeDir string, entry *types.TimelineEntry) error {
	if _, err := validateSnapshotName(entry.AfterSnapshotName); err != nil {
		return err
	}
	if entry.BeforeSnapshotName != nil {
		if _, err := validateSnapshotName(*entry.BeforeSnapshotName); err != nil {
			return err
		}
	}
	if _, err := EnsureStore(storeDir); err != nil {
		return err
	}
	dir := filepath.Join(storeDir, timelineEventsDir)
	if err := os.MkdirAll(dir, storeMode); err != nil {
		return &StoreError{IO: err}
	}
	if err := setMode(dir, storeMode); err != nil {
		return err
	}
	return writeJSONAtomic(timelineEntryPath(storeDir, entry), entry)
}

func ListTimelineEntries(storeDir string, options TimelineListOptions) ([]types.TimelineEntry, error) {
	dir := filepath.Join(storeDir, timelineEventsDir)
	exists, err := pathExists(dir)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	entries, err := readTimelineEntries(dir, options.OnCorruptEntry)
	if err != nil {
		return nil, err
	}
	if options.ProjectPath != "" {
		projectPath := resolvePathStr(options.ProjectPath)
		filtered := entries[:0]
		for _, entry := range entries {
			if resolvePathStr(entry.ProjectPath) == projectPath {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	if options.Agent != nil {
		agent := *options.Agent
		filtered := entries[:0]
		for _, entry := range entries {
			if (entry.Agent != nil && *entry.Agent == agent) || containsAgent(entry.Agents, agent) {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ObservedAt > entries[j].ObservedAt
	})
	if options.Limit != nil {
		if len(entries) > *options.Limit {
			entries = entries[:*options.Limit]
		}
	}
	return entries, nil
}

func LatestTimelineEntry(storeDir string, options TimelineListOptions) (*types.TimelineEntry, error) {
	limit := 1
	options.Limit = &limit
	entries, err := ListTimelineEntries(storeDir, options)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

func FindTimelineEntry(storeDir, reference string, options TimelineListOptions) (*types.TimelineEntry, error) {
	options.Limit = nil
	entries, err := ListTimelineEntries(storeDir, options)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].ID == reference || entries[i].AfterSnapshotName == reference {
			return &entries[i], nil
		}
	}
	return nil, nil
}

func StateHash(snapshot types.Snapshot) string {
	payload := map[string]any{
		"evidence":       snapshot.Evidence,
		"graph":          snapshot.Graph,
		"auditFindings":  snapshot.AuditFindings,
		"provenance":     snapshot.Provenance,
	}
	serialized, err := json.Marshal(payload)
	if err != nil {
		serialized = []byte{}
	}
	digest := sha256.Sum256(serialized)
	return fmt.Sprintf("sha256:%x", digest)
}

func validateSnapshotName(name string) (string, error) {
	if !isSafeSnapshotName(name) {
		unsafeName := name
		return "", &StoreError{UnsafeSnapshotName: &unsafeName}
	}
	return name, nil
}

func isSafeSnapshotName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return trimmed != "" &&
		!strings.Contains(name, "..") &&
		!strings.Contains(name, "/") &&
		!strings.Contains(name, `\`)
}

func isSafeAgentName(name string) bool {
	return safeAgentNamePattern.MatchString(name) &&
		!strings.Contains(name, "..") &&
		!strings.Contains(name, "/")
}

func isSafeSnapshotRelativePath(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if filepath.IsAbs(name) || strings.Contains(name, `\`) {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func writeJSONAtomic(filePath string, value any) error {
	serialized, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return &StoreError{JSON: err}
	}
	return writeTextAtomic(filePath, string(serialized)+"\n")
}

func writeTextAtomic(filePath, value string) error {
	if err := fsutil.WriteTextAtomically(filePath, value, 0o600); err != nil {
		return &StoreError{IO: err}
	}
	return nil
}

func readJSON(filePath string, target any) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &StoreError{IO: err}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return &StoreError{JSON: err}
	}
	return nil
}

func readOptionalJSON[T any](filePath string) (*T, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, &StoreError{IO: err}
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, &StoreError{JSON: err}
	}
	return &value, nil
}

func timelineEntryPath(storeDir string, entry *types.TimelineEntry) string {
	observed := strings.NewReplacer(":", "-", ".", "-").Replace(entry.ObservedAt)
	return filepath.Join(storeDir, timelineEventsDir, fmt.Sprintf("%s-%s.json", observed, entry.ID))
}

func readTimelineEntries(dir string, onCorruptEntry func(TimelineCorruptEvent)) ([]types.TimelineEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, &StoreError{IO: err}
	}

	var result []types.TimelineEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		var raw map[string]json.RawMessage
		if err := readJSON(path, &raw); err != nil {
			if onCorruptEntry != nil {
				onCorruptEntry(TimelineCorruptEvent{
					FilePath: path,
					Error:    fmt.Sprintf("invalid timeline JSON: %v", err),
				})
			}
			continue
		}
		normalized, err := normalizeTimelineEntry(raw)
		if err != nil {
			if onCorruptEntry != nil {
				onCorruptEntry(TimelineCorruptEvent{
					FilePath: path,
					Error:    fmt.Sprintf("invalid timeline JSON: %v", err),
				})
			}
			continue
		}
		result = append(result, normalized)
	}
	return result, nil
}

func normalizeTimelineEntry(raw map[string]json.RawMessage) (types.TimelineEntry, error) {
	var legacyDaemonRunID string
	if value, ok := raw["daemonRunId"]; ok {
		_ = json.Unmarshal(value, &legacyDaemonRunID)
	}

	captureID := ""
	if value, ok := raw["captureId"]; ok {
		var parsed string
		_ = json.Unmarshal(value, &parsed)
		if parsed != "" {
			captureID = parsed
		}
	}
	if captureID == "" && legacyDaemonRunID != "" {
		captureID = legacyDaemonRunID
	}
	if captureID == "" {
		if value, ok := raw["id"]; ok {
			_ = json.Unmarshal(value, &captureID)
		}
	}
	if captureID == "" {
		captureID = "legacy"
	}

	raw["source"] = json.RawMessage(`"manual"`)
	captureBytes, _ := json.Marshal(captureID)
	raw["captureId"] = captureBytes

	payload, err := json.Marshal(raw)
	if err != nil {
		return types.TimelineEntry{}, &StoreError{JSON: err}
	}
	var entry types.TimelineEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return types.TimelineEntry{}, &StoreError{JSON: err}
	}
	entry.Source = types.TimelineSourceManual
	entry.CaptureID = captureID
	return entry, nil
}

func checksumsFromEvidence(evidence []types.DiscoveredItem) map[string]ChecksumRecord {
	checksums := make(map[string]ChecksumRecord)
	for _, item := range evidence {
		if item.Checksum == nil || *item.Checksum == "" {
			continue
		}
		checksums[item.ID] = ChecksumRecord{
			SourcePath: item.SourcePath,
			Checksum:   *item.Checksum,
		}
	}
	return checksums
}

func pathExists(targetPath string) (bool, error) {
	_, err := os.Stat(targetPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, &StoreError{IO: err}
}

func pathExistsBool(targetPath string) bool {
	exists, _ := pathExists(targetPath)
	return exists
}

func resolvePathStr(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
}

func setMode(path string, mode fs.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(path, mode); err != nil {
		return &StoreError{IO: err}
	}
	return nil
}

func fileMode(path string) (uint32, error) {
	if runtime.GOOS == "windows" {
		return storeMode, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, &StoreError{IO: err}
	}
	return uint32(info.Mode().Perm()), nil
}

func containsAgent(agents []types.AgentID, target types.AgentID) bool {
	for _, agent := range agents {
		if agent == target {
			return true
		}
	}
	return false
}