package parser

import (
	"encoding/json"
	"time"
)

type RecordType string

const (
	TypeUser           RecordType = "user"
	TypeAssistant      RecordType = "assistant"
	TypeSystem         RecordType = "system"
	TypeAttachment     RecordType = "attachment"
	TypeFileSnapshot   RecordType = "file-history-snapshot"
	TypeQueueOperation RecordType = "queue-operation"
	TypeProgress       RecordType = "progress"
	TypeLastPrompt     RecordType = "last-prompt"
	TypePermissionMode RecordType = "permission-mode"
	TypeAITitle        RecordType = "ai-title"
	TypeCustomTitle    RecordType = "custom-title"
	TypeAgentName      RecordType = "agent-name"
	TypePRLink         RecordType = "pr-link"
)

// Record is the union of fields seen across all session JSONL record types.
// Most fields are optional; consumers should switch on Type.
type Record struct {
	Type        RecordType      `json:"type"`
	UUID        string          `json:"uuid,omitempty"`
	ParentUUID  *string         `json:"parentUuid,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
	Timestamp   time.Time       `json:"timestamp,omitempty"`
	CWD         string          `json:"cwd,omitempty"`
	GitBranch   string          `json:"gitBranch,omitempty"`
	Version     string          `json:"version,omitempty"`
	UserType    string          `json:"userType,omitempty"`
	Entrypoint  string          `json:"entrypoint,omitempty"`
	IsSidechain bool            `json:"isSidechain,omitempty"`
	Message     json.RawMessage `json:"message,omitempty"`
	Title       string          `json:"title,omitempty"`

	// Per-record-type single-field payloads. Each only populates when Type
	// matches; consumers switch on Type to decide which to read.
	AITitle     string `json:"aiTitle,omitempty"`     // ai-title records
	CustomTitle string `json:"customTitle,omitempty"` // custom-title records
	AgentName   string `json:"agentName,omitempty"`   // agent-name records
}

type AssistantMessage struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Model      string         `json:"model"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      Usage          `json:"usage"`
}

type Usage struct {
	InputTokens int `json:"input_tokens"`
	// CacheCreationInputTokens is the wire-format sum across all TTLs (5m + 1h).
	// Always equals CacheCreation.Ephemeral5mInputTokens + CacheCreation.Ephemeral1hInputTokens
	// when the split is present; on older payloads only this sum is populated.
	CacheCreationInputTokens int             `json:"cache_creation_input_tokens"`
	CacheCreation            CacheCreationTL `json:"cache_creation"`
	CacheReadInputTokens     int             `json:"cache_read_input_tokens"`
	OutputTokens             int             `json:"output_tokens"`
	ServiceTier              string          `json:"service_tier"`
}

// CacheCreationTL splits cache-creation tokens by ephemeral TTL — Anthropic
// charges the 1h cache at a higher rate than the 5m cache.
type CacheCreationTL struct {
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
}

// ContentBlock covers text / thinking / tool_use / tool_result variants.
type ContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`    // tool_use id
	Name      string          `json:"name,omitempty"`  // tool name
	Input     json.RawMessage `json:"input,omitempty"` // tool input
	ToolUseID string          `json:"tool_use_id,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // tool_result content
}

// UserMessage content can be a plain string or an array of ContentBlock.
type UserMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}
