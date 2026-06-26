package bundle

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qyinm/gandalf/internal/gandalfcore/fsutil"
	"github.com/qyinm/gandalf/internal/gandalfcore/pathconfinement"
	"github.com/qyinm/gandalf/internal/gandalfcore/policy"
	"github.com/qyinm/gandalf/internal/gandalfcore/readiness"
	"github.com/qyinm/gandalf/internal/gandalfcore/scan"
	"github.com/qyinm/gandalf/internal/gandalfcore/store"
	"github.com/qyinm/gandalf/internal/gandalfcore/tar"
	"github.com/qyinm/gandalf/internal/gandalfcore/types"
)

const (
	formatVersion      = "1"
	homeToken          = "{home}"
	signatureAlgorithm = "HMAC-SHA256"
)

// Error represents bundle operation failures.
type Error struct {
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func maxBundleBytes() uint64 {
	if value := os.Getenv("GANDALF_MAX_BUNDLE_BYTES"); value != "" {
		var parsed uint64
		if _, err := fmt.Sscan(value, &parsed); err == nil {
			return parsed
		}
	}
	return 500 * 1024 * 1024
}

func maxContentBytes() uint64 {
	if value := os.Getenv("GANDALF_MAX_CONTENT_BYTES"); value != "" {
		var parsed uint64
		if _, err := fmt.Sscan(value, &parsed); err == nil {
			return parsed
		}
	}
	return 50 * 1024 * 1024
}

// Export writes a snapshot to a .gandalf bundle archive.
func Export(options *types.BundleExportOptions) (*types.BundleExportResult, error) {
	signatureKey := resolveSignatureKey(options.SignatureKey)
	includeContent := options.IncludeContent != nil && *options.IncludeContent

	snapshot, err := store.ReadSnapshot(options.StoreDir, options.SnapshotName, options.Agent)
	if err != nil {
		return nil, &Error{Message: "read snapshot", Cause: err}
	}

	var unsafeItems []types.DiscoveredItem
	for _, item := range snapshot.Evidence {
		if item.CaptureStatus == types.CaptureUnsafeToExport {
			unsafeItems = append(unsafeItems, item)
		}
	}
	if len(unsafeItems) > 0 {
		return nil, &Error{Message: fmt.Sprintf(
			"Cannot export: %d evidence item(s) are marked unsafe_to_export. First: %s",
			len(unsafeItems), unsafeItems[0].SourcePath,
		)}
	}

	if includeContent {
		var unsupported []types.DiscoveredItem
		for _, item := range snapshot.Evidence {
			if item.CaptureStatus == types.CaptureCaptured &&
				policy.RestorePolicyFor(item.Kind) == types.RestoreNotSupported {
				unsupported = append(unsupported, item)
			}
		}
		if len(unsupported) > 0 {
			return nil, &Error{Message: fmt.Sprintf(
				"Cannot export content bundle: %d not_supported evidence item(s) would lose restore data. First: %s (kind: %s). Use --metadata-only or remove unsupported items before exporting a restorable content bundle.",
				len(unsupported), unsupported[0].SourcePath, unsupported[0].Kind,
			)}
		}
	}

	var warnings []string
	sourceMachine := captureSourceMachine()
	now := nowMillis()
	var entries []types.TarEntry

	entries = append(entries, dirEntry(".gandalf/", now))
	entries = append(entries, fileEntry(".gandalf/format-version", []byte(formatVersion+"\n"), now))

	signed := signatureKey != nil
	var sigAlg *string
	if signed {
		alg := signatureAlgorithm
		sigAlg = &alg
	}

	manifest := types.BundleManifest{
		FormatVersion:   1,
		SnapshotName:    options.SnapshotName,
		CreatedAt:       snapshot.Manifest.CreatedAt,
		ProjectPath:     options.ProjectPath,
		IncludesContent: includeContent,
		SourceMachine:   &sourceMachine,
		Security: types.BundleSecurity{
			RawSecretsIncluded: false,
			RedactionPolicy:    "metadata-only",
			Signed:             signed,
			SignatureAlgorithm: sigAlg,
		},
	}

	manifestBytes, err := jsonPrettyLine(manifest)
	if err != nil {
		return nil, err
	}
	entries = append(entries, fileEntry(".gandalf/manifest.json", manifestBytes, now))
	entries = append(entries, dirEntry("snapshot/", now))

	bundledEvidence := normaliseEvidencePaths(snapshot.Evidence, options.HomeDir)
	bundledGraph := normaliseGraphPaths(snapshot.Graph, options.HomeDir)
	bundledProvenance := normaliseProvenancePaths(snapshot.Provenance, options.HomeDir)

	snapshotFiles := []struct {
		name string
		data any
	}{
		{"evidence.json", bundledEvidence},
		{"graph.json", bundledGraph},
		{"audit-findings.json", snapshot.AuditFindings},
		{"provenance.json", bundledProvenance},
		{"checksums.json", map[string]any{}},
		{"redactions.json", []any{}},
	}
	for _, file := range snapshotFiles {
		content, err := jsonPrettyLine(file.data)
		if err != nil {
			return nil, err
		}
		entries = append(entries, fileEntry("snapshot/"+file.name, content, now))
	}

	if includeContent {
		var totalContentBytes uint64
		var contentCount uint32

		seenPaths := make(map[string]struct{})
		var uniqueItems []types.DiscoveredItem
		for _, item := range snapshot.Evidence {
			if item.CaptureStatus != types.CaptureCaptured ||
				policy.RestorePolicyFor(item.Kind) != types.RestoreFullContent ||
				item.SourcePath == "" ||
				strings.HasPrefix(item.SourcePath, "~/.env") {
				continue
			}
			if _, seen := seenPaths[item.SourcePath]; seen {
				continue
			}
			seenPaths[item.SourcePath] = struct{}{}
			uniqueItems = append(uniqueItems, item)
		}

		entries = append(entries, dirEntry("content/", now))
		for _, item := range uniqueItems {
			sourceAbs := resolveSourcePath(item.SourcePath, options.HomeDir, options.ProjectPath)
			info, err := os.Stat(sourceAbs)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if uint64(info.Size()) > maxContentBytes() {
				warnings = append(warnings, fmt.Sprintf(
					"Skipped large file: %s (%d bytes > %d limit)",
					item.SourcePath, info.Size(), maxContentBytes(),
				))
				continue
			}
			content, err := os.ReadFile(sourceAbs)
			if err != nil {
				continue
			}
			normalisedPath := normaliseSourcePath(item.SourcePath, options.HomeDir)
			tarPath := "content/" + normalisedPath
			mtime := now
			if mod := info.ModTime(); !mod.IsZero() {
				mtime = uint64(mod.UnixMilli())
			}
			entries = append(entries, types.TarEntry{
				Path:      tarPath,
				Content:   content,
				Mode:      0o644,
				Mtime:     mtime,
				EntryType: types.TarEntryFile,
			})
			totalContentBytes += uint64(len(content))
			contentCount++
		}
		manifest.ContentFileCount = contentCount
		manifest.ContentTotalBytes = totalContentBytes
		if err := updateManifestEntry(&entries, &manifest); err != nil {
			return nil, err
		}
	}

	if signatureKey != nil {
		sig, err := signBundleEntries(entries, &manifest, *signatureKey)
		if err != nil {
			return nil, err
		}
		manifest.Security.Signature = &sig
		if err := updateManifestEntry(&entries, &manifest); err != nil {
			return nil, err
		}
	}

	checksumsContent, err := jsonPrettyLine(types.BundleChecksums{
		Algorithm: "SHA-256",
		Entries:   computeEntryChecksums(entries),
	})
	if err != nil {
		return nil, err
	}
	checksumsEntry := fileEntry(".gandalf/checksums.json", checksumsContent, now)

	finalEntries := make([]types.TarEntry, 0, len(entries)+1)
	checksumsInserted := false
	for _, entry := range entries {
		finalEntries = append(finalEntries, entry)
		if entry.Path == ".gandalf/manifest.json" && !checksumsInserted {
			finalEntries = append(finalEntries, checksumsEntry)
			checksumsInserted = true
		}
	}
	if !checksumsInserted {
		finalEntries = append(finalEntries, checksumsEntry)
	}

	archiveChecksum, err := tar.WriteTar(finalEntries, options.OutputPath)
	if err != nil {
		return nil, &Error{Message: "write tar", Cause: err}
	}

	return &types.BundleExportResult{
		BundlePath: options.OutputPath,
		Checksum:   archiveChecksum,
		Warnings:   warnings,
	}, nil
}

// Import reads a .gandalf bundle and optionally applies content.
func Import(options *types.BundleImportOptions) (*types.BundleImportResult, error) {
	signatureKey := resolveSignatureKey(options.SignatureKey)
	applyContent := options.ApplyContent != nil && *options.ApplyContent
	dryRun := options.DryRun != nil && *options.DryRun
	quarantine := options.Quarantine != nil && *options.Quarantine

	entries, _, err := tar.ReadTar(options.BundlePath)
	if err != nil {
		return nil, &Error{Message: "read tar", Cause: err}
	}

	formatEntry := findEntry(entries, ".gandalf/format-version")
	if formatEntry == nil {
		return nil, &Error{Message: "Invalid bundle: missing .gandalf/format-version"}
	}
	formatVersionStr := strings.TrimSpace(string(formatEntry.Content))
	if formatVersionStr != formatVersion {
		return nil, &Error{Message: fmt.Sprintf(
			`Unsupported bundle format version: "%s" (expected "%s")`, formatVersionStr, formatVersion,
		)}
	}

	manifestEntry := findEntry(entries, ".gandalf/manifest.json")
	if manifestEntry == nil {
		return nil, &Error{Message: "Invalid bundle: missing .gandalf/manifest.json"}
	}
	var manifest types.BundleManifest
	if err := json.Unmarshal(manifestEntry.Content, &manifest); err != nil {
		return nil, &Error{Message: "parse manifest", Cause: err}
	}

	sigVerification := verifyBundleSignature(entries, &manifest, signatureKey)
	if !sigVerification.OK {
		return nil, &Error{Message: sigVerification.Warning}
	}
	var trustWarning *string
	if sigVerification.Checked && signatureKey != nil {
		warning, err := enforceBundleKeyTrust(options.StoreDir, *signatureKey, options.Trust != nil && *options.Trust)
		if err != nil {
			return nil, err
		}
		trustWarning = warning
	}

	if checksumsEntry := findEntry(entries, ".gandalf/checksums.json"); checksumsEntry != nil {
		var checksums types.BundleChecksums
		if err := json.Unmarshal(checksumsEntry.Content, &checksums); err != nil {
			return nil, &Error{Message: "parse checksums", Cause: err}
		}
		for _, entry := range entries {
			if entry.Path == ".gandalf/checksums.json" {
				continue
			}
			expected, ok := checksums.Entries[entry.Path]
			if !ok {
				continue
			}
			actual := sha256Hex(entry.Content)
			if actual != expected {
				return nil, &Error{Message: fmt.Sprintf(
					`Checksum mismatch for "%s": expected %s, got %s`, entry.Path, expected, actual,
				)}
			}
		}
	}

	homeDir := options.HomeDir
	projectPath := options.ProjectPath
	allRoots := []string{homeDir, projectPath}

	for _, entry := range entries {
		if err := validateEntryPath(entry.Path); err != nil {
			return nil, err
		}
		if strings.HasPrefix(entry.Path, "content/") {
			relativePath := strings.TrimPrefix(entry.Path, "content/")
			resolved := resolveBundlePath(relativePath, homeDir, projectPath)
			if !anyRootContains(allRoots, resolved) {
				return nil, &Error{Message: fmt.Sprintf(
					`Content path "%s" resolves outside home and project directories`, relativePath,
				)}
			}
		}
	}

	info, err := os.Stat(options.BundlePath)
	if err != nil {
		return nil, &Error{Message: "stat bundle", Cause: err}
	}
	if uint64(info.Size()) > maxBundleBytes() {
		return nil, &Error{Message: fmt.Sprintf(
			"Bundle too large: %d bytes (max %d)", info.Size(), maxBundleBytes(),
		)}
	}

	if applyContent {
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Path, "content/") {
				continue
			}
			relativePath := strings.TrimPrefix(entry.Path, "content/")
			if strings.HasPrefix(relativePath, "~/") || strings.HasPrefix(relativePath, homeToken+"/") {
				return nil, &Error{Message: fmt.Sprintf(
					`Home-relative content path "%s" is not allowed. Bundle content must be project-relative.`, relativePath,
				)}
			}
			resolved := resolveBundlePath(relativePath, homeDir, projectPath)
			if !pathconfinement.IsStrictlyUnder(resolved, homeDir) &&
				!pathconfinement.IsStrictlyUnder(resolved, projectPath) {
				return nil, &Error{Message: fmt.Sprintf(
					`Content path "%s" resolves outside home and project directories`, relativePath,
				)}
			}
		}
	}

	targetPlatform := readiness.CurrentPlatform()
	if options.TargetPlatform != nil {
		targetPlatform = *options.TargetPlatform
	}
	targetHostname := hostname()

	var sourceHome *string
	if manifest.SourceMachine != nil {
		normalised := normaliseHomeForPlatform(manifest.SourceMachine.HomeDir, manifest.SourceMachine.Platform)
		sourceHome = &normalised
	}
	normalisedTargetHome := normaliseHomeForPlatform(homeDir, targetPlatform)

	var remappedPaths []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Path, "content/") || entry.EntryType != types.TarEntryFile {
			continue
		}
		relativePath := strings.TrimPrefix(entry.Path, "content/")
		if rest, ok := strings.CutPrefix(relativePath, homeToken+"/"); ok {
			sourceAbs := "source:" + rest
			if sourceHome != nil {
				sourceAbs = *sourceHome + "/" + rest
			}
			remappedPaths = append(remappedPaths, fmt.Sprintf("%s → %s/%s", sourceAbs, normalisedTargetHome, rest))
		}
	}

	crossOS := manifest.SourceMachine != nil && manifest.SourceMachine.Platform != targetPlatform
	var osDifferences []string
	if crossOS && manifest.SourceMachine != nil {
		osDifferences = append(osDifferences, fmt.Sprintf(
			"%s → %s (cross-OS restore)", manifest.SourceMachine.Platform, targetPlatform,
		))
	}

	evidenceEntry := findEntry(entries, "snapshot/evidence.json")
	var sourceMcpBinaries []types.McpBinaryInfo
	if evidenceEntry != nil {
		var evidence []types.DiscoveredItem
		_ = json.Unmarshal(evidenceEntry.Content, &evidence)
		sourceMcpBinaries = readiness.ExtractMcpBinaries(evidence, sourceHome)
	}
	mcpBinaryReport := readiness.CheckMcpBinaryAvailability(sourceMcpBinaries)

	machineDiff := types.MachineDiff{
		SourceHome:        "unknown",
		TargetHome:        homeDir,
		SourcePlatform:    "unknown",
		TargetPlatform:    targetPlatform,
		SourceHostname:    "unknown",
		TargetHostname:    targetHostname,
		CrossOS:           crossOS,
		OSDifferences:     osDifferences,
		RemappedPaths:     remappedPaths,
		SourceMcpBinaries: sourceMcpBinaries,
		McpBinaryReport:   mcpBinaryReport,
	}
	if manifest.SourceMachine != nil {
		machineDiff.SourceHome = manifest.SourceMachine.HomeDir
		machineDiff.SourcePlatform = manifest.SourceMachine.Platform
		machineDiff.SourceHostname = manifest.SourceMachine.Hostname
	}

	var warnings []string
	if sigVerification.Warning != "" {
		warnings = append(warnings, sigVerification.Warning)
	}
	if trustWarning != nil {
		warnings = append(warnings, *trustWarning)
	}

	var sourceEvidence []types.DiscoveredItem
	if evidenceEntry != nil {
		_ = json.Unmarshal(evidenceEntry.Content, &sourceEvidence)
	}
	targetEvidence := scan.ScanProject(&types.ScanOptions{
		ProjectPath: options.ProjectPath,
		HomeDir:     options.HomeDir,
		StoreDir:    options.StoreDir,
	}).Evidence

	readinessReport := readiness.BuildReadinessReport(sourceEvidence, &types.ReadinessOptions{
		SourceHomeDir:  sourceHome,
		TargetPlatform: &targetPlatform,
		ApplyContent:   applyContent,
		TargetEvidence: targetEvidence,
	})

	for _, item := range readinessReport.Items {
		if applyContent && !dryRun && item.Category == types.ReadinessBlocked {
			return nil, &Error{Message: fmt.Sprintf("%s: %s", item.Code, item.Problem)}
		}
	}

	if dryRun {
		evidenceCount := 0
		for _, entry := range entries {
			if strings.HasPrefix(entry.Path, "snapshot/") {
				evidenceCount++
			}
		}
		return &types.BundleImportResult{
			SnapshotName:    manifest.SnapshotName,
			EvidenceCount:   evidenceCount,
			IncludesContent: manifest.IncludesContent,
			Warnings:        warnings,
			MachineDiff:     &machineDiff,
			Readiness:       readinessReport,
		}, nil
	}

	graphEntry := findEntry(entries, "snapshot/graph.json")
	auditEntry := findEntry(entries, "snapshot/audit-findings.json")
	if graphEntry == nil || auditEntry == nil || evidenceEntry == nil {
		return nil, &Error{Message: "Invalid bundle: missing snapshot data files"}
	}
	provenanceEntry := findEntry(entries, "snapshot/provenance.json")

	var rawEvidence []types.DiscoveredItem
	if err := json.Unmarshal(evidenceEntry.Content, &rawEvidence); err != nil {
		return nil, &Error{Message: "parse evidence", Cause: err}
	}
	importedEvidence, err := resolveSnapshotPathsForImport(rawEvidence, homeDir)
	if err != nil {
		return nil, err
	}

	var rawGraph []types.GraphNode
	if err := json.Unmarshal(graphEntry.Content, &rawGraph); err != nil {
		return nil, &Error{Message: "parse graph", Cause: err}
	}
	importedGraph, err := resolveSnapshotPathsForImportGraph(rawGraph, homeDir)
	if err != nil {
		return nil, err
	}

	var importedProvenance []types.ProvenanceEntry
	if provenanceEntry != nil {
		var rawProvenance []types.ProvenanceEntry
		if err := json.Unmarshal(provenanceEntry.Content, &rawProvenance); err != nil {
			return nil, &Error{Message: "parse provenance", Cause: err}
		}
		importedProvenance, err = resolveSnapshotPathsForImportProvenance(rawProvenance, homeDir)
		if err != nil {
			return nil, err
		}
	}

	var auditFindings []types.AuditFinding
	if err := json.Unmarshal(auditEntry.Content, &auditFindings); err != nil {
		return nil, &Error{Message: "parse audit findings", Cause: err}
	}

	snapshot := types.Snapshot{
		Manifest: types.SnapshotManifest{
			SchemaVersion: "0.1",
			Name:          manifest.SnapshotName,
			CreatedAt:     manifest.CreatedAt,
			ProjectPath:   manifest.ProjectPath,
			Security: types.SnapshotSecurity{
				RawSecretsIncluded: false,
				RedactionPolicy:    "metadata-only",
			},
		},
		Evidence:      importedEvidence,
		Graph:         importedGraph,
		AuditFindings: auditFindings,
		Provenance:    importedProvenance,
	}
	if err := store.WriteSnapshot(options.StoreDir, store.StoreSnapshotFrom(snapshot), options.Agent); err != nil {
		return nil, &Error{Message: "write snapshot", Cause: err}
	}

	contentApplied := false
	var quarantinedContentDir *string
	if applyContent {
		var applyEntries []types.TarEntry
		for _, entry := range entries {
			if strings.HasPrefix(entry.Path, "content/") && entry.EntryType == types.TarEntryFile {
				applyEntries = append(applyEntries, entry)
			}
		}
		if quarantine {
			safeName := sanitizeSnapshotName(manifest.SnapshotName)
			quarantineDir := filepath.Join(options.StoreDir, "quarantine", fmt.Sprintf("%s-%d", safeName, nowMillis()))
			for _, entry := range applyEntries {
				relativePath := strings.TrimPrefix(entry.Path, "content/")
				relativePath = strings.ReplaceAll(relativePath, homeToken, "home")
				quarantinePath := filepath.Join(quarantineDir, relativePath)
				if err := os.MkdirAll(filepath.Dir(quarantinePath), 0o700); err != nil {
					return nil, &Error{Message: "create quarantine dir", Cause: err}
				}
				if err := os.WriteFile(quarantinePath, entry.Content, 0o644); err != nil {
					return nil, &Error{Message: "write quarantine file", Cause: err}
				}
			}
			warnings = append(warnings, fmt.Sprintf(
				"Content files quarantined for inspection at %s; no target files were modified.", quarantineDir,
			))
			quarantinedContentDir = &quarantineDir
		} else {
			roots := pathconfinement.Roots{HomeDir: homeDir, ProjectPath: projectPath}
			for _, entry := range applyEntries {
				relativePath := strings.TrimPrefix(entry.Path, "content/")
				resolved := resolveBundlePath(relativePath, homeDir, projectPath)
				if _, err := pathconfinement.ValidateConstrainedWritePath(resolved, &roots); err != nil {
					return nil, &Error{Message: err.Error()}
				}
				if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
					return nil, &Error{Message: "create parent dir", Cause: err}
				}
				if err := fsutil.WriteTextAtomically(resolved, string(entry.Content), 0o644); err != nil {
					return nil, &Error{Message: "write content file", Cause: err}
				}
			}
			contentApplied = true
		}
	}

	return &types.BundleImportResult{
		SnapshotName:          manifest.SnapshotName,
		EvidenceCount:         len(importedEvidence),
		IncludesContent:       manifest.IncludesContent,
		ContentApplied:        contentApplied,
		QuarantinedContentDir: quarantinedContentDir,
		Warnings:              warnings,
		MachineDiff:           &machineDiff,
		Readiness:             readinessReport,
	}, nil
}

// Inspect reads bundle metadata without importing.
func Inspect(bundlePath string) (*types.BundleInspectResult, error) {
	entries, bundleChecksum, err := tar.ReadTar(bundlePath)
	if err != nil {
		return nil, &Error{Message: "read tar", Cause: err}
	}
	manifestEntry := findEntry(entries, ".gandalf/manifest.json")
	if manifestEntry == nil {
		return nil, &Error{Message: "Invalid bundle: missing .gandalf/manifest.json"}
	}
	var manifest types.BundleManifest
	if err := json.Unmarshal(manifestEntry.Content, &manifest); err != nil {
		return nil, &Error{Message: "parse manifest", Cause: err}
	}
	algorithm := "SHA-256"
	if checksumsEntry := findEntry(entries, ".gandalf/checksums.json"); checksumsEntry != nil {
		var checksums types.BundleChecksums
		if err := json.Unmarshal(checksumsEntry.Content, &checksums); err == nil && checksums.Algorithm != "" {
			algorithm = checksums.Algorithm
		}
	}
	return &types.BundleInspectResult{
		BundlePath:         bundlePath,
		FormatVersion:      manifest.FormatVersion,
		SnapshotName:       manifest.SnapshotName,
		CreatedAt:          manifest.CreatedAt,
		ProjectPath:        manifest.ProjectPath,
		IncludesContent:    manifest.IncludesContent,
		ContentFileCount:   manifest.ContentFileCount,
		ContentTotalBytes:  manifest.ContentTotalBytes,
		ChecksumAlgorithm:  algorithm,
		BundleChecksum:     bundleChecksum,
		IsSigned:           manifest.Security.Signed,
		SignatureAlgorithm: manifest.Security.SignatureAlgorithm,
		SourceMachine:      manifest.SourceMachine,
	}, nil
}

// Verify checks bundle format, checksums, and signature.
func Verify(options *types.BundleVerifyOptions) (*types.BundleVerifyResult, error) {
	entries, _, err := tar.ReadTar(options.BundlePath)
	if err != nil {
		return nil, &Error{Message: "read tar", Cause: err}
	}
	signatureKey := resolveSignatureKey(options.SignatureKey)
	var warnings, errors []string
	var formatVersionPtr *uint32

	if formatEntry := findEntry(entries, ".gandalf/format-version"); formatEntry != nil {
		version := strings.TrimSpace(string(formatEntry.Content))
		if version != formatVersion {
			errors = append(errors, fmt.Sprintf(
				`Unsupported bundle format version: "%s" (expected "%s")`, version, formatVersion,
			))
		} else {
			v := uint32(1)
			formatVersionPtr = &v
		}
	} else {
		errors = append(errors, "Invalid bundle: missing .gandalf/format-version")
	}

	manifestEntry := findEntry(entries, ".gandalf/manifest.json")
	if manifestEntry == nil {
		errors = append(errors, "Invalid bundle: missing .gandalf/manifest.json")
		return &types.BundleVerifyResult{
			BundlePath: options.BundlePath,
			Valid:      false,
			Checksums:  types.BundleVerifyChecksumResult{},
			Signature:  types.BundleVerifySignatureResult{OK: true},
			Errors:     errors,
		}, nil
	}

	var manifest types.BundleManifest
	if err := json.Unmarshal(manifestEntry.Content, &manifest); err != nil {
		errors = append(errors, "Invalid bundle manifest JSON: "+err.Error())
		return &types.BundleVerifyResult{
			BundlePath: options.BundlePath,
			Valid:      false,
			Checksums:  types.BundleVerifyChecksumResult{},
			Signature:  types.BundleVerifySignatureResult{OK: true},
			Errors:     errors,
		}, nil
	}

	sigVerification := verifyBundleSignature(entries, &manifest, signatureKey)
	signature := types.BundleVerifySignatureResult{
		Signed:    manifest.Security.Signed,
		Checked:   sigVerification.Checked,
		OK:        sigVerification.OK,
		Algorithm: manifest.Security.SignatureAlgorithm,
	}
	if sigVerification.Warning != "" {
		if sigVerification.OK {
			warnings = append(warnings, sigVerification.Warning)
		} else {
			errors = append(errors, sigVerification.Warning)
		}
	}

	checksumResult := types.BundleVerifyChecksumResult{}
	if checksumsEntry := findEntry(entries, ".gandalf/checksums.json"); checksumsEntry != nil {
		checksumResult.Checked = true
		var checksums types.BundleChecksums
		if err := json.Unmarshal(checksumsEntry.Content, &checksums); err != nil {
			errors = append(errors, "Invalid checksums JSON: "+err.Error())
		} else {
			for _, entry := range entries {
				if entry.Path == ".gandalf/checksums.json" {
					continue
				}
				expected, ok := checksums.Entries[entry.Path]
				if !ok {
					continue
				}
				checksumResult.EntriesChecked++
				actual := sha256Hex(entry.Content)
				if actual != expected {
					errors = append(errors, fmt.Sprintf(
						`Checksum mismatch for "%s": expected %s, got %s`, entry.Path, expected, actual,
					))
				}
			}
			checksumResult.OK = true
			for _, e := range errors {
				if strings.HasPrefix(e, "Checksum mismatch") {
					checksumResult.OK = false
					break
				}
			}
		}
	} else {
		errors = append(errors, "Invalid bundle: missing .gandalf/checksums.json")
	}

	snapshotName := manifest.SnapshotName
	return &types.BundleVerifyResult{
		BundlePath:    options.BundlePath,
		Valid:         len(errors) == 0,
		FormatVersion: formatVersionPtr,
		SnapshotName:  &snapshotName,
		Checksums:     checksumResult,
		Signature:     signature,
		Warnings:      warnings,
		Errors:        errors,
	}, nil
}

type signatureVerification struct {
	OK      bool
	Checked bool
	Warning string
}

func resolveSignatureKey(explicit *string) *string {
	if explicit != nil {
		return explicit
	}
	if value := os.Getenv("GANDALF_BUNDLE_KEY"); value != "" {
		return &value
	}
	return nil
}

func signBundleEntries(entries []types.TarEntry, manifest *types.BundleManifest, key string) (string, error) {
	mac := hmac.New(sha256.New, []byte(key))
	payload := canonicalSignaturePayload(entries, manifest)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func verifyBundleSignature(entries []types.TarEntry, manifest *types.BundleManifest, key *string) signatureVerification {
	if !manifest.Security.Signed {
		return signatureVerification{OK: true}
	}
	if key == nil {
		return signatureVerification{
			Warning: "Bundle is signed, but no signature key was provided; signature was not verified.",
		}
	}
	if manifest.Security.Signature == nil {
		return signatureVerification{
			OK:      false,
			Checked: true,
			Warning: "Signed bundle manifest is missing security.signature.",
		}
	}
	actual, _ := signBundleEntries(entries, manifest, *key)
	if actual != *manifest.Security.Signature {
		return signatureVerification{
			OK:      false,
			Checked: true,
			Warning: "Bundle signature verification failed.",
		}
	}
	return signatureVerification{OK: true, Checked: true}
}

func canonicalSignaturePayload(entries []types.TarEntry, manifest *types.BundleManifest) []byte {
	cloned := *manifest
	cloned.Security.Signature = nil
	manifestBytes, _ := json.Marshal(cloned)

	var hmacEntries []types.TarEntry
	for _, entry := range entries {
		if entry.Path == ".gandalf/manifest.json" || entry.Path == ".gandalf/checksums.json" {
			continue
		}
		if entry.EntryType == types.TarEntryFile {
			hmacEntries = append(hmacEntries, entry)
		}
	}
	sort.Slice(hmacEntries, func(i, j int) bool {
		return hmacEntries[i].Path < hmacEntries[j].Path
	})

	var payload []byte
	payload = append(payload, manifestBytes...)
	payload = append(payload, '\n')
	for _, entry := range hmacEntries {
		payload = append(payload, []byte(fmt.Sprintf("%s\n%d\n", entry.Path, len(entry.Content)))...)
		payload = append(payload, entry.Content...)
		payload = append(payload, '\n')
	}
	return payload
}

func enforceBundleKeyTrust(storeDir, key string, trust bool) (*string, error) {
	fingerprint := sha256Hex([]byte(key))
	filePath := filepath.Join(storeDir, "trust", "bundle-signing-key.json")
	var trusted map[string]any
	if data, err := os.ReadFile(filePath); err == nil {
		_ = json.Unmarshal(data, &trusted)
	}
	stored, _ := trusted["fingerprint"].(string)
	if stored == "" {
		if !trust {
			msg := "No trusted bundle signing key is recorded. Re-run with --trust after verifying the source."
			return &msg, nil
		}
		if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
			return nil, &Error{Message: "create trust dir", Cause: err}
		}
		payload, _ := json.MarshalIndent(map[string]any{
			"fingerprint": fingerprint,
			"trustedAt":   time.Now().UTC().Format(time.RFC3339),
		}, "", "  ")
		if err := os.WriteFile(filePath, append(payload, '\n'), 0o600); err != nil {
			return nil, &Error{Message: "write trust file", Cause: err}
		}
		prefix := fingerprint
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		msg := fmt.Sprintf("Trusted bundle signing key fingerprint %s…", prefix)
		return &msg, nil
	}
	if stored != fingerprint {
		return nil, &Error{Message: fmt.Sprintf(
			"Bundle signing key fingerprint %s… does not match trusted key fingerprint %s…",
			truncate(fingerprint, 12), truncate(stored, 12),
		)}
	}
	return nil, nil
}

func normaliseHomeForPlatform(homeDir, machinePlatform string) string {
	if machinePlatform == "linux" {
		if username, ok := strings.CutPrefix(homeDir, "/Users/"); ok {
			if idx := strings.Index(username, "/"); idx >= 0 {
				username = username[:idx]
			}
			return "/home/" + username
		}
	}
	if machinePlatform == "darwin" {
		if username, ok := strings.CutPrefix(homeDir, "/home/"); ok {
			if idx := strings.Index(username, "/"); idx >= 0 {
				username = username[:idx]
			}
			return "/Users/" + username
		}
	}
	return homeDir
}

func normaliseSourcePath(sourcePath, homeDir string) string {
	if rest, ok := strings.CutPrefix(sourcePath, "~/"); ok {
		return homeToken + "/" + rest
	}
	resolvedSource, _ := filepath.EvalSymlinks(sourcePath)
	resolvedHome, _ := filepath.EvalSymlinks(homeDir)
	if filepath.IsAbs(sourcePath) &&
		(resolvedSource == resolvedHome || strings.HasPrefix(resolvedSource, resolvedHome+string(os.PathSeparator))) {
		rel, err := filepath.Rel(resolvedHome, resolvedSource)
		if err != nil || rel == "." {
			return homeToken
		}
		return homeToken + "/" + filepath.ToSlash(rel)
	}
	return sourcePath
}

func resolveBundlePath(normalisedPath, homeDir, projectPath string) string {
	if rest, ok := strings.CutPrefix(normalisedPath, homeToken+"/"); ok {
		return filepath.Join(homeDir, rest)
	}
	return filepath.Join(projectPath, normalisedPath)
}

func normaliseEvidencePaths(items []types.DiscoveredItem, homeDir string) []types.DiscoveredItem {
	out := make([]types.DiscoveredItem, len(items))
	for i, item := range items {
		item.SourcePath = normaliseSourcePath(item.SourcePath, homeDir)
		out[i] = item
	}
	return out
}

func normaliseGraphPaths(items []types.GraphNode, homeDir string) []types.GraphNode {
	out := make([]types.GraphNode, len(items))
	for i, item := range items {
		item.SourcePath = normaliseSourcePath(item.SourcePath, homeDir)
		out[i] = item
	}
	return out
}

func normaliseProvenancePaths(items []types.ProvenanceEntry, homeDir string) []types.ProvenanceEntry {
	out := make([]types.ProvenanceEntry, len(items))
	for i, item := range items {
		item.SourcePath = normaliseSourcePath(item.SourcePath, homeDir)
		out[i] = item
	}
	return out
}

func resolveSnapshotPathForImport(sourcePath, homeDir string) (string, error) {
	if rest, ok := strings.CutPrefix(sourcePath, homeToken+"/"); ok {
		if err := pathconfinement.ValidateHomeRelativeImportSegment(rest); err != nil {
			return "", err
		}
		return filepath.Join(homeDir, rest), nil
	}
	if strings.Contains(sourcePath, "..") {
		return "", fmt.Errorf(`path traversal detected: "%s" contains ".."`, sourcePath)
	}
	return sourcePath, nil
}

func resolveSnapshotPathsForImport(items []types.DiscoveredItem, homeDir string) ([]types.DiscoveredItem, error) {
	out := make([]types.DiscoveredItem, len(items))
	for i, item := range items {
		resolved, err := resolveSnapshotPathForImport(item.SourcePath, homeDir)
		if err != nil {
			return nil, &Error{Message: err.Error()}
		}
		item.SourcePath = resolved
		out[i] = item
	}
	return out, nil
}

func resolveSnapshotPathsForImportGraph(items []types.GraphNode, homeDir string) ([]types.GraphNode, error) {
	out := make([]types.GraphNode, len(items))
	for i, item := range items {
		resolved, err := resolveSnapshotPathForImport(item.SourcePath, homeDir)
		if err != nil {
			return nil, &Error{Message: err.Error()}
		}
		item.SourcePath = resolved
		out[i] = item
	}
	return out, nil
}

func resolveSnapshotPathsForImportProvenance(items []types.ProvenanceEntry, homeDir string) ([]types.ProvenanceEntry, error) {
	out := make([]types.ProvenanceEntry, len(items))
	for i, item := range items {
		resolved, err := resolveSnapshotPathForImport(item.SourcePath, homeDir)
		if err != nil {
			return nil, &Error{Message: err.Error()}
		}
		item.SourcePath = resolved
		out[i] = item
	}
	return out, nil
}

func captureSourceMachine() types.SourceMachine {
	home := os.Getenv("HOME")
	return types.SourceMachine{
		HomeDir:  home,
		Platform: readiness.CurrentPlatform(),
		Hostname: hostname(),
	}
}

func resolveSourcePath(sourcePath, homeDir, projectPath string) string {
	if rest, ok := strings.CutPrefix(sourcePath, "~/"); ok {
		return filepath.Join(homeDir, rest)
	}
	if filepath.IsAbs(sourcePath) {
		return sourcePath
	}
	return filepath.Join(projectPath, sourcePath)
}

func computeEntryChecksums(entries []types.TarEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		out[entry.Path] = sha256Hex(entry.Content)
	}
	return out
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func jsonPrettyLine(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, &Error{Message: "marshal JSON", Cause: err}
	}
	return append(data, '\n'), nil
}

func updateManifestEntry(entries *[]types.TarEntry, manifest *types.BundleManifest) error {
	content, err := jsonPrettyLine(manifest)
	if err != nil {
		return err
	}
	for i := range *entries {
		if (*entries)[i].Path == ".gandalf/manifest.json" {
			(*entries)[i].Content = content
			return nil
		}
	}
	return nil
}

func dirEntry(path string, mtime uint64) types.TarEntry {
	return types.TarEntry{Path: path, Mode: 0o755, Mtime: mtime, EntryType: types.TarEntryDirectory}
}

func fileEntry(path string, content []byte, mtime uint64) types.TarEntry {
	return types.TarEntry{Path: path, Content: content, Mode: 0o644, Mtime: mtime, EntryType: types.TarEntryFile}
}

func nowMillis() uint64 {
	return uint64(time.Now().UnixMilli())
}

func hostname() string {
	if value := os.Getenv("HOSTNAME"); value != "" {
		return value
	}
	if value := os.Getenv("COMPUTERNAME"); value != "" {
		return value
	}
	return "unknown"
}

func findEntry(entries []types.TarEntry, path string) *types.TarEntry {
	for i := range entries {
		if entries[i].Path == path {
			return &entries[i]
		}
	}
	return nil
}

func validateEntryPath(path string) error {
	if strings.Contains(path, "..") {
		return &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" contains ".."`, path)}
	}
	if strings.ContainsRune(path, '\x00') {
		return &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" contains null byte`, path)}
	}
	if filepath.IsAbs(path) {
		return &Error{Message: fmt.Sprintf(`Path traversal detected: "%s" is absolute`, path)}
	}
	return nil
}

func anyRootContains(roots []string, resolved string) bool {
	for _, root := range roots {
		if pathconfinement.IsStrictlyUnder(resolved, root) {
			return true
		}
	}
	return false
}

func sanitizeSnapshotName(name string) string {
	var b strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') ||
			ch == '.' || ch == '_' || ch == '-' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
