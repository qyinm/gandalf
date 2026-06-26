package types

// SourceMachine describes the machine that produced a bundle.
type SourceMachine struct {
	HomeDir  string `json:"homeDir"`
	Platform string `json:"platform"`
	Hostname string `json:"hostname"`
}

// BundleSecurity captures bundle signing and redaction metadata.
type BundleSecurity struct {
	RawSecretsIncluded bool    `json:"rawSecretsIncluded"`
	RedactionPolicy    string  `json:"redactionPolicy"`
	Signed             bool    `json:"signed"`
	SignatureAlgorithm *string `json:"signatureAlgorithm,omitempty"`
	Signature          *string `json:"signature,omitempty"`
}

// BundleManifest is the bundle metadata document.
type BundleManifest struct {
	FormatVersion     uint32         `json:"formatVersion"`
	SnapshotName      string         `json:"snapshotName"`
	CreatedAt         string         `json:"createdAt"`
	ProjectPath       string         `json:"projectPath"`
	IncludesContent   bool           `json:"includesContent"`
	ContentFileCount  uint32         `json:"contentFileCount"`
	ContentTotalBytes uint64         `json:"contentTotalBytes"`
	SourceMachine     *SourceMachine `json:"sourceMachine,omitempty"`
	Security          BundleSecurity `json:"security"`
}

// BundleChecksums maps tar entry paths to SHA-256 hex digests.
type BundleChecksums struct {
	Algorithm string            `json:"algorithm"`
	Entries   map[string]string `json:"entries"`
}

// BundleExportOptions configures bundle export.
type BundleExportOptions struct {
	SnapshotName   string
	OutputPath     string
	StoreDir       string
	ProjectPath    string
	HomeDir        string
	IncludeContent *bool
	SignatureKey   *string
	Agent          *AgentID
}

// BundleExportResult is returned after a successful export.
type BundleExportResult struct {
	BundlePath string   `json:"bundlePath"`
	Checksum   string   `json:"checksum"`
	Warnings   []string `json:"warnings"`
}

// BundleImportOptions configures bundle import.
type BundleImportOptions struct {
	BundlePath     string
	StoreDir       string
	ProjectPath    string
	HomeDir        string
	ApplyContent   *bool
	DryRun         *bool
	Quarantine     *bool
	Trust          *bool
	SignatureKey   *string
	Agent          *AgentID
	TargetPlatform *string
}

// McpBinaryKind classifies MCP launch commands.
type McpBinaryKind string

const (
	McpBinaryPackageRunner   McpBinaryKind = "package_runner"
	McpBinarySourceLocalPath McpBinaryKind = "source_local_path"
	McpBinaryPathBinary      McpBinaryKind = "path_binary"
	McpBinaryCommand         McpBinaryKind = "command"
	McpBinaryRemoteURL       McpBinaryKind = "remote_url"
)

// McpBinaryInfo describes an MCP binary referenced in evidence.
type McpBinaryInfo struct {
	EvidenceID string         `json:"evidenceId"`
	Command    string         `json:"command"`
	Args       []string       `json:"args,omitempty"`
	URL        *string        `json:"url,omitempty"`
	BinaryKind *McpBinaryKind `json:"binaryKind,omitempty"`
}

// McpBinaryReport describes MCP binary availability on the target machine.
type McpBinaryReport struct {
	EvidenceID        string         `json:"evidenceId"`
	Command           string         `json:"command"`
	AvailableOnTarget bool           `json:"availableOnTarget"`
	BinaryKind        *McpBinaryKind `json:"binaryKind,omitempty"`
	ResolvedPath      *string        `json:"resolvedPath,omitempty"`
	Warning           *string        `json:"warning,omitempty"`
}

// MachineDiff summarizes cross-machine import differences.
type MachineDiff struct {
	SourceHome        string            `json:"sourceHome"`
	TargetHome        string            `json:"targetHome"`
	SourcePlatform    string            `json:"sourcePlatform"`
	TargetPlatform    string            `json:"targetPlatform"`
	SourceHostname    string            `json:"sourceHostname"`
	TargetHostname    string            `json:"targetHostname"`
	CrossOS           bool              `json:"crossOs"`
	OSDifferences     []string          `json:"osDifferences"`
	RemappedPaths     []string          `json:"remappedPaths"`
	SourceMcpBinaries []McpBinaryInfo   `json:"sourceMcpBinaries"`
	McpBinaryReport   []McpBinaryReport `json:"mcpBinaryReport"`
}

// BundleImportResult is returned after bundle import.
type BundleImportResult struct {
	SnapshotName          string          `json:"snapshotName"`
	EvidenceCount         int             `json:"evidenceCount"`
	IncludesContent       bool            `json:"includesContent"`
	ContentApplied        bool            `json:"contentApplied"`
	QuarantinedContentDir *string         `json:"quarantinedContentDir,omitempty"`
	Warnings              []string        `json:"warnings"`
	MachineDiff           *MachineDiff    `json:"machineDiff,omitempty"`
	Readiness             ReadinessReport `json:"readiness"`
}

// BundleInspectResult summarizes bundle metadata without importing.
type BundleInspectResult struct {
	BundlePath         string         `json:"bundlePath"`
	FormatVersion      uint32         `json:"formatVersion"`
	SnapshotName       string         `json:"snapshotName"`
	CreatedAt          string         `json:"createdAt"`
	ProjectPath        string         `json:"projectPath"`
	IncludesContent    bool           `json:"includesContent"`
	ContentFileCount   uint32         `json:"contentFileCount"`
	ContentTotalBytes  uint64         `json:"contentTotalBytes"`
	ChecksumAlgorithm  string         `json:"checksumAlgorithm"`
	BundleChecksum     string         `json:"bundleChecksum"`
	IsSigned           bool           `json:"isSigned"`
	SignatureAlgorithm *string        `json:"signatureAlgorithm,omitempty"`
	SourceMachine      *SourceMachine `json:"sourceMachine,omitempty"`
}

// BundleVerifyOptions configures bundle verification.
type BundleVerifyOptions struct {
	BundlePath   string
	SignatureKey *string
}

// BundleVerifyChecksumResult reports per-entry checksum verification.
type BundleVerifyChecksumResult struct {
	Checked        bool   `json:"checked"`
	OK             bool   `json:"ok"`
	EntriesChecked uint32 `json:"entriesChecked"`
}

// BundleVerifySignatureResult reports signature verification.
type BundleVerifySignatureResult struct {
	Signed    bool    `json:"signed"`
	Checked   bool    `json:"checked"`
	OK        bool    `json:"ok"`
	Algorithm *string `json:"algorithm,omitempty"`
}

// BundleVerifyResult is the full bundle verification report.
type BundleVerifyResult struct {
	BundlePath    string                      `json:"bundlePath"`
	Valid         bool                        `json:"valid"`
	FormatVersion *uint32                     `json:"formatVersion,omitempty"`
	SnapshotName  *string                     `json:"snapshotName,omitempty"`
	Checksums     BundleVerifyChecksumResult  `json:"checksums"`
	Signature     BundleVerifySignatureResult `json:"signature"`
	Warnings      []string                    `json:"warnings"`
	Errors        []string                    `json:"errors"`
}

// TarEntryType identifies tar archive entry kinds.
type TarEntryType int

const (
	TarEntryFile TarEntryType = iota
	TarEntryDirectory
)

// TarEntry is a POSIX ustar archive member.
type TarEntry struct {
	Path      string
	Content   []byte
	Mode      uint32
	Mtime     uint64
	EntryType TarEntryType
}
