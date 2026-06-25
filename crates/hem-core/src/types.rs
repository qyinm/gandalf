use serde::{Deserialize, Deserializer, Serialize};
use serde_json::Value;

pub const EVIDENCE_KINDS: &[&str] = &[
    "agent_config",
    "agent_instruction",
    "mcp_server",
    "permission",
    "skill",
    "extension",
    "env_key",
    "hook",
    "symlink",
    "unsupported",
];

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize)]
#[serde(rename_all = "kebab-case")]
pub enum AgentId {
    #[serde(rename = "claude-code")]
    ClaudeCode,
    Codex,
    Cursor,
    Opencode,
    #[serde(rename = "pi-agent")]
    PiAgent,
    Project,
    Unknown,
}

impl<'de> Deserialize<'de> for AgentId {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let value = String::deserialize(deserializer)?;
        Ok(AgentId::from_str(&value))
    }
}

impl AgentId {
    pub fn from_str(value: &str) -> Self {
        match value {
            "claude-code" => AgentId::ClaudeCode,
            "codex" => AgentId::Codex,
            "cursor" => AgentId::Cursor,
            "opencode" => AgentId::Opencode,
            "pi-agent" => AgentId::PiAgent,
            "project" => AgentId::Project,
            "unknown" => AgentId::Unknown,
            _ => AgentId::Unknown,
        }
    }

    pub fn as_str(self) -> &'static str {
        match self {
            AgentId::ClaudeCode => "claude-code",
            AgentId::Codex => "codex",
            AgentId::Cursor => "cursor",
            AgentId::Opencode => "opencode",
            AgentId::PiAgent => "pi-agent",
            AgentId::Project => "project",
            AgentId::Unknown => "unknown",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EvidenceKind {
    AgentConfig,
    AgentInstruction,
    McpServer,
    Permission,
    Skill,
    Extension,
    EnvKey,
    Hook,
    Symlink,
    Unsupported,
}

impl EvidenceKind {
    pub fn as_str(self) -> &'static str {
        match self {
            EvidenceKind::AgentConfig => "agent_config",
            EvidenceKind::AgentInstruction => "agent_instruction",
            EvidenceKind::McpServer => "mcp_server",
            EvidenceKind::Permission => "permission",
            EvidenceKind::Skill => "skill",
            EvidenceKind::Extension => "extension",
            EvidenceKind::EnvKey => "env_key",
            EvidenceKind::Hook => "hook",
            EvidenceKind::Symlink => "symlink",
            EvidenceKind::Unsupported => "unsupported",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RestorePolicy {
    FullContentSupported,
    StructuredFieldsOnly,
    KeyInventoryOnly,
    NotSupported,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EvidenceScope {
    User,
    Project,
    Managed,
    Unknown,
}

impl EvidenceScope {
    pub fn as_str(self) -> &'static str {
        match self {
            EvidenceScope::User => "user",
            EvidenceScope::Project => "project",
            EvidenceScope::Managed => "managed",
            EvidenceScope::Unknown => "unknown",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum CaptureStatus {
    Captured,
    Redacted,
    Omitted,
    ParseFailed,
    UnsafeToExport,
    Unsupported,
}

impl CaptureStatus {
    pub fn as_str(self) -> &'static str {
        match self {
            CaptureStatus::Captured => "captured",
            CaptureStatus::Redacted => "redacted",
            CaptureStatus::Omitted => "omitted",
            CaptureStatus::ParseFailed => "parse_failed",
            CaptureStatus::UnsafeToExport => "unsafe_to_export",
            CaptureStatus::Unsupported => "unsupported",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Severity {
    None,
    Low,
    Medium,
    High,
    Critical,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum EvidenceParser {
    Json,
    Toml,
    Markdown,
    Dotenv,
    Filesystem,
    Unknown,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum EvidenceConfidence {
    Low,
    Medium,
    High,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DiscoveredItem {
    pub id: String,
    pub agent: AgentId,
    pub kind: EvidenceKind,
    pub source_path: String,
    pub scope: EvidenceScope,
    pub precedence: u32,
    pub parser: EvidenceParser,
    pub sensitivity: String,
    pub content_policy: String,
    pub restore_policy: RestorePolicy,
    pub capture_status: CaptureStatus,
    pub confidence: EvidenceConfidence,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub value: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub checksum: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SnapshotManifest {
    pub schema_version: String,
    pub name: String,
    pub created_at: String,
    pub project_path: String,
    pub security: SnapshotSecurity,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SnapshotSecurity {
    pub raw_secrets_included: bool,
    pub redaction_policy: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SnapshotContentEntry {
    pub evidence_id: String,
    pub source_path: String,
    pub restore_path: String,
    pub checksum: String,
    pub byte_length: u64,
    pub encoding: String,
    pub storage_path: String,
    pub capture_status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct Snapshot {
    pub manifest: SnapshotManifest,
    pub evidence: Vec<DiscoveredItem>,
    pub graph: Vec<GraphNode>,
    pub audit_findings: Vec<AuditFinding>,
    pub provenance: Vec<ProvenanceEntry>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub content: Option<Vec<SnapshotContentEntry>>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GraphNode {
    pub id: String,
    pub agent: AgentId,
    pub scope: EvidenceScope,
    pub source_path: String,
    pub entity_kind: EvidenceKind,
    pub entity_name: String,
    pub effective_value: Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub overridden_by: Option<String>,
    pub confidence: EvidenceConfidence,
    pub evidence_id: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct AuditFinding {
    pub code: String,
    pub severity: Severity,
    pub problem: String,
    pub cause: String,
    pub fix: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub evidence_id: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ProvenanceEntry {
    pub node_id: String,
    pub evidence_id: String,
    pub source_path: String,
    pub scope: EvidenceScope,
    pub precedence: u32,
    pub confidence: EvidenceConfidence,
    pub capture_status: CaptureStatus,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanOptions {
    pub project_path: String,
    pub home_dir: String,
    pub store_dir: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub explain: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent: Option<AgentId>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub scope: Option<EvidenceScope>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RestoreAction {
    Create,
    Update,
    Delete,
    Skip,
    Conflict,
    Unsupported,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ItemDiff {
    pub changes: Vec<String>,
    pub additions: Vec<String>,
    pub removals: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RestorePlanItem {
    pub item_id: String,
    pub agent: AgentId,
    pub kind: EvidenceKind,
    pub source_path: String,
    pub depends_on: Vec<String>,
    pub action: RestoreAction,
    pub current_state: Option<DiscoveredItem>,
    pub target_state: Option<DiscoveredItem>,
    pub diff: ItemDiff,
    pub risk_level: Severity,
    pub risk_reason: String,
    pub needs_confirmation: bool,
    pub confirmation_prompt: String,
    pub rollback_instruction: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RestorePlan {
    pub plan_id: String,
    pub source_snapshot: String,
    pub target_project: String,
    pub target_home: String,
    pub created_at: String,
    pub item_count: u32,
    pub risk_summary: RiskSummary,
    pub items: Vec<RestorePlanItem>,
    pub rollback_plan: RollbackPlan,
    pub execution_order: Vec<String>,
    pub unsupported_items: Vec<UnsupportedPlanItem>,
    pub plan_metadata: RestorePlanMetadata,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RiskSummary {
    pub none: u32,
    pub low: u32,
    pub medium: u32,
    pub high: u32,
    pub critical: u32,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UnsupportedPlanItem {
    pub item_id: String,
    pub agent: AgentId,
    pub kind: EvidenceKind,
    pub source_path: String,
    pub reason: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RollbackPlan {
    pub steps: Vec<RollbackStep>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RollbackStep {
    pub item_id: String,
    pub action: String,
    pub instruction: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RestorePlanMetadata {
    pub planner_version: String,
    pub generated_by: String,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RestoreItemStatus {
    Pending,
    Applied,
    Failed,
    Skipped,
    Unsupported,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RestoreItem {
    pub item_id: String,
    pub path: String,
    #[serde(rename = "type")]
    pub item_type: String,
    pub source: String,
    pub dest: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub action: Option<RestoreAction>,
    pub status: RestoreItemStatus,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error_message: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub skip_reason: Option<String>,
    pub execution_order: u32,
    pub rollback_state: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target_content: Option<Value>,
    pub can_rollback: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub apply_at: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RuntimeOptions {
    pub project_path: String,
    pub home_dir: String,
    pub store_dir: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent: Option<AgentId>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub scope: Option<EvidenceScope>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub capture_content: Option<bool>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RestoreOptions {
    pub source_snapshot: String,
    pub project_path: String,
    pub home_dir: String,
    pub store_dir: String,
    pub dry_run: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent: Option<AgentId>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub scope: Option<EvidenceScope>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ScanTrust {
    pub read_only: bool,
    pub network: String,
    pub commands_executed: Vec<String>,
    pub store_write_location: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanResult {
    pub trust: ScanTrust,
    pub evidence: Vec<DiscoveredItem>,
    pub blind_spots: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CurrentState {
    pub scan: ScanResult,
    pub snapshot: Snapshot,
    pub store_findings: Vec<AuditFinding>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplyOptions {
    pub fail_fast: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rollback: Option<bool>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplyFailure {
    pub item_id: String,
    pub reason: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplySummary {
    pub total: u32,
    pub successful: u32,
    pub failed: u32,
    pub skipped: u32,
    pub unsupported: u32,
    pub failures: Vec<ApplyFailure>,
    pub applied_items: Vec<RestoreItem>,
    pub status_registry: std::collections::HashMap<String, RestoreItemStatus>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum UndoStatus {
    Undone,
    Skipped,
    Failed,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct UndoResult {
    pub item_id: String,
    pub status: UndoStatus,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RollbackSummary {
    pub total: u32,
    pub undone: u32,
    pub skipped: u32,
    pub failed: u32,
    pub results: Vec<UndoResult>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplyWithRollbackResult {
    pub apply_summary: ApplySummary,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub rollback_summary: Option<RollbackSummary>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum TimelineEntrySource {
    Manual,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TimelineEntryEventKind {
    Baseline,
    SetupChanged,
    Unchanged,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum TimelineRestoreReadiness {
    Full,
    Partial,
    #[serde(rename = "observe-only")]
    ObserveOnly,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum TimelineConfidence {
    Low,
    Medium,
    High,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TimelineChangeSummary {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub previous_entry_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub previous_snapshot_name: Option<String>,
    pub has_changes: bool,
    pub semantic_change_count: u32,
    pub raw_source_change_count: u32,
    pub highlights: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TimelineChangedSurface {
    pub kind: String,
    pub change_type: String,
    pub path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub entity_name: Option<String>,
    pub restorable: bool,
    pub observe_only: bool,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TimelineEntry {
    pub schema_version: String,
    pub id: String,
    pub source: TimelineEntrySource,
    pub event_kind: TimelineEntryEventKind,
    pub title: String,
    pub project_path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent: Option<AgentId>,
    pub agents: Vec<AgentId>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub before_snapshot_name: Option<String>,
    pub after_snapshot_name: String,
    pub capture_id: String,
    pub created_at: String,
    pub observed_at: String,
    pub changed_surfaces: Vec<TimelineChangedSurface>,
    pub restore_readiness: TimelineRestoreReadiness,
    pub confidence: TimelineConfidence,
    pub confidence_reason: String,
    pub evidence_count: u32,
    pub graph_node_count: u32,
    pub audit_finding_count: u32,
    pub changes: TimelineChangeSummary,
}

impl Default for RiskSummary {
    fn default() -> Self {
        Self {
            none: 0,
            low: 0,
            medium: 0,
            high: 0,
            critical: 0,
        }
    }
}