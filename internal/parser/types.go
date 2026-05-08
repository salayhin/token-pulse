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
	InputTokens              int    `json:"input_tokens"`
	OutputTokens             int    `json:"output_tokens"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
	ServiceTier              string `json:"service_tier"`
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
