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
        Ok(match value.as_str() {
            "claude-code" => AgentId::ClaudeCode,
            "codex" => AgentId::Codex,
            "cursor" => AgentId::Cursor,
            "opencode" => AgentId::Opencode,
            "pi-agent" => AgentId::PiAgent,
            "project" => AgentId::Project,
            "unknown" => AgentId::Unknown,
            _ => AgentId::Unknown,
        })
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