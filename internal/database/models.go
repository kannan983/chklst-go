package database

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"chklst-go/internal/crypto"

	"gorm.io/gorm"
)

// StringArray is a custom type for JSON string arrays in SQLite
type StringArray []string

// Scan implements sql.Scanner interface
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = []string{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("failed to unmarshal StringArray value")
	}

	return json.Unmarshal(bytes, a)
}

// Value implements driver.Valuer interface
func (a StringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	return json.Marshal(a)
}

// Project represents a deployment project
type Project struct {
	ID             uint         `gorm:"primaryKey" json:"id"`
	Name           string       `gorm:"unique;not null;index" json:"name"`
	BuildServer    string       `json:"build_server"`
	DeployServer   string       `json:"deploy_server"`
	DatabaseName   string       `json:"database_name"`
	Environment    string       `json:"environment"`
	BackupLocation string       `json:"backup_location"`
	Description    string       `gorm:"type:text" json:"description"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`

	// Relationships
	// Components must NOT be omitempty: a project with zero components would lose
	// the key entirely and the frontend store crashes on components.push(...).
	Components  []Component  `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"components"`
	Deployments []Deployment `gorm:"foreignKey:ProjectID;constraint:OnDelete:CASCADE" json:"deployments,omitempty"`
}

// Component represents a project component
type Component struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ProjectID    uint      `gorm:"not null;index" json:"project_id"`
	Name         string    `gorm:"not null;index" json:"name"`
	Developer    string    `json:"developer"`
	VCSType      string    `gorm:"default:'git'" json:"vcs_type"` // git, svn, etc.
	VCSURL       string    `json:"vcs_url"`
	BuildCommand string    `json:"build_command"`
	ComponentURL string    `json:"component_url"`
	Enabled      bool      `gorm:"default:true" json:"enabled"`
	Description  string    `gorm:"type:text" json:"description"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	Project     Project      `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Deployments []Deployment `gorm:"foreignKey:ComponentID;constraint:OnDelete:CASCADE" json:"deployments,omitempty"`
}

// Deployment represents a deployment record
type Deployment struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	JiraID              string    `gorm:"index" json:"jira_id"` // PAT-1234 (Jira ticket key)
	Timestamp           time.Time `gorm:"index" json:"timestamp"`
	ProjectID           uint      `gorm:"not null;index" json:"project_id"`
	ComponentID         *uint     `gorm:"index" json:"component_id"` // Nullable for legacy data
	Environment         string    `json:"environment"`
	VCSURL              string    `json:"vcs_url"`
	DeveloperName       string    `json:"developer_name"`
	BuildServer         string    `json:"build_server"`
	DeployServer        string    `json:"deploy_server"`
	DatabaseName        string    `json:"database_name"`
	DBBackupLocation    string    `json:"db_backup_location"`
	DatabaseScript      string    `gorm:"type:text" json:"database_script"`
	PreviousBuildBackup string    `json:"previous_build_backup"`
	BuildStatus         string    `gorm:"default:'pending'" json:"build_status"`
	DeployStatus        string    `gorm:"default:'pending'" json:"deploy_status"`
	Notes               string    `gorm:"type:text" json:"notes"`
	DeployedBy          string    `json:"deployed_by"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	// Automation & Tracking fields
	ChangeTicket    string `json:"change_ticket"`     // ServiceNow/other change ticket
	JiraCommentSent bool   `json:"jira_comment_sent"` // Was Jira comment posted?
	WebhookSent     bool   `json:"webhook_sent"`      // Was webhook notification sent?

	// Relationships
	Project   Project    `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
	Component *Component `gorm:"foreignKey:ComponentID" json:"component,omitempty"`
}

// Library stores preset values for dropdowns
type Library struct {
	ID            uint        `gorm:"primaryKey" json:"id"`
	Developers    StringArray `gorm:"type:json" json:"developers"`
	BuildServers  StringArray `gorm:"type:json" json:"build_servers"`
	DeployServers StringArray `gorm:"type:json" json:"deploy_servers"`
	Environments  StringArray `gorm:"type:json" json:"environments"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// TableName overrides the table name for Library
func (Library) TableName() string {
	return "library"
}

// Settings stores application settings (singleton)
type Settings struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	DefaultDeployedBy  string    `json:"default_deployed_by"`
	ExcelExportPath    string    `json:"excel_export_path"`
	AutoClearAfterSave bool      `json:"auto_clear_after_save"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Jira Integration
	JiraURL     string `json:"jira_url"`     // e.g., "https://ttsme.atlassian.net"
	JiraEmail   string `json:"jira_email"`   // e.g., "you@example.com"
	JiraToken   string `json:"jira_token"`   // API token (stored as-is, consider encryption later)
	JiraProject string `json:"jira_project"` // e.g., "PAT"

	// Webhook Notifications
	TeamsWebhookURL string `json:"teams_webhook_url"` // Microsoft Teams incoming webhook
	WebhooksEnabled bool   `json:"webhooks_enabled"`  // Master toggle for webhooks

	// AI / Ollama (local LLM for daily summary + analytics narratives), aka "Parson"
	AIEnabled bool   `json:"ai_enabled"` // Master toggle for AI features
	OllamaURL string `json:"ollama_url"` // e.g. "http://localhost:11434" (host.docker.internal in Docker)
	AIModel   string `json:"ai_model"`   // e.g. "gemma4:e4b"

	// PlannedWeekly is standing "planned activities for the week" text that the user
	// fills once; it persists across days and is included in every daily report
	// alongside the day-specific planned activities.
	PlannedWeekly string `gorm:"type:text" json:"planned_weekly"`

	// Git Insights (GitHub). Token is encrypted at rest (see hooks below).
	GitHubToken string `json:"github_token"` // PAT with repo read access
	GitHubOwner string `json:"github_owner"` // user or org login
	GitHubRepos string `json:"github_repos"` // comma-separated repo names; blank = all owner repos

	// Parson agent configuration.
	// ParsonSystemPrompt overrides the built-in system prompt; blank = use default.
	ParsonSystemPrompt string  `gorm:"type:text" json:"parson_system_prompt"`
	ParsonTemperature  float64 `json:"parson_temperature"` // 0 = use default (0.4)

	// Email (SMTP) for sending the daily report.
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`     // 587 (STARTTLS), 465 (TLS), 25 (none)
	SMTPUser     string `json:"smtp_user"`
	SMTPPassword string `json:"smtp_password"` // app password (TODO encrypt at rest)
	SMTPFrom     string `json:"smtp_from"`
	SMTPTo       string `json:"smtp_to"`       // comma-separated recipients
	SMTPCc       string `json:"smtp_cc"`       // comma-separated cc
	SMTPSecurity string `json:"smtp_security"` // "starttls" | "tls" | "none"
	SMTPTestTo   string `json:"smtp_test_to"`  // where "Send Test Email" goes (defaults to To if blank)

	// Daily summary scheduler (Parson auto-runs the report).
	SummaryScheduleEnabled bool   `json:"summary_schedule_enabled"`
	SummaryGenerateTime    string `json:"summary_generate_time"` // "HH:MM" local, e.g. "18:30"
	SummarySendTime        string `json:"summary_send_time"`     // "HH:MM" local, e.g. "20:00"
	SummaryWeekdays        string `json:"summary_weekdays"`      // CSV of weekday nums, 0=Sun .. 6=Sat
	SummaryAutoSend        bool   `json:"summary_auto_send"`     // send at send-time without manual approval
}

// TableName overrides the table name for Settings
func (Settings) TableName() string {
	return "settings"
}

// --- At-rest encryption for sensitive settings ---
// Secrets are encrypted only in the database. GORM hooks encrypt on write and
// decrypt on read, so the rest of the app (and the JSON API) always sees plaintext.
// No-op when APP_ENCRYPTION_KEY is unset (see internal/crypto).

func (s *Settings) encryptSecrets() {
	s.SMTPPassword = crypto.Encrypt(s.SMTPPassword)
	s.JiraToken = crypto.Encrypt(s.JiraToken)
	s.TeamsWebhookURL = crypto.Encrypt(s.TeamsWebhookURL)
	s.GitHubToken = crypto.Encrypt(s.GitHubToken)
}

func (s *Settings) decryptSecrets() {
	s.SMTPPassword = crypto.Decrypt(s.SMTPPassword)
	s.JiraToken = crypto.Decrypt(s.JiraToken)
	s.TeamsWebhookURL = crypto.Decrypt(s.TeamsWebhookURL)
	s.GitHubToken = crypto.Decrypt(s.GitHubToken)
}

// BeforeSave encrypts secrets just before they hit the DB.
func (s *Settings) BeforeSave(*gorm.DB) error { s.encryptSecrets(); return nil }

// AfterSave restores plaintext in the in-memory struct (so the handler returns
// plaintext to the caller after a save).
func (s *Settings) AfterSave(*gorm.DB) error { s.decryptSecrets(); return nil }

// AfterFind decrypts secrets loaded from the DB.
func (s *Settings) AfterFind(*gorm.DB) error { s.decryptSecrets(); return nil }

// DailySummary is the per-day deployment status report. The deployment facts are
// pulled from the deployments table at generate time; this row holds the human
// notes and the AI-written email so it can be reviewed, edited, and sent.
type DailySummary struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	Date   string `gorm:"uniqueIndex" json:"date"`        // "2026-06-08" (one report per day)
	Status string `gorm:"default:'draft'" json:"status"` // draft | generated | approved | sent | failed

	// Human-entered context (added during the day, by ~6 PM)
	Notes      string `gorm:"type:text" json:"notes"`      // extra activities not captured as deployments
	Planned    string `gorm:"type:text" json:"planned"`    // planned activities for tomorrow
	Roadblocks string `gorm:"type:text" json:"roadblocks"` // roadblocks / suggestions

	// AI output (editable before sending)
	GeneratedSubject string `gorm:"type:text" json:"generated_subject"`
	GeneratedBody    string `gorm:"type:text" json:"generated_body"`

	// Delivery (used by Phase 2 SMTP + scheduler)
	Recipients string     `json:"recipients"` // comma-separated; blank = use Settings defaults
	SentAt     *time.Time `json:"sent_at"`
	SendError  string     `gorm:"type:text" json:"send_error"`

	// Parson activity tracking
	GeneratedAt       *time.Time `json:"generated_at"`        // when Parson wrote it
	GenerationSeconds float64    `json:"generation_seconds"`  // how long generation took
	GeneratedByModel  string     `json:"generated_by_model"`  // which model produced it
	Reviewed          bool       `json:"reviewed"`            // did the user edit it after generation?
	LockedAt          *time.Time `json:"locked_at"`           // when it became final (generate or last edit)
	AutoGenerated     bool       `json:"auto_generated"`      // produced by the scheduler vs manual

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Holiday marks a date on which Parson must NOT auto-generate or send.
type Holiday struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Date      string    `gorm:"uniqueIndex" json:"date"` // "2006-01-02"
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// TeamsEvent is one raw MQTT message from Teams for Linux (teams/#), or a logger
// marker (_logger/*). Stored raw; all analytics are computed at report time.
type TeamsEvent struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	TS       time.Time `gorm:"index" json:"ts"` // receive time, UTC
	Topic    string    `gorm:"not null;index" json:"topic"`
	Payload  string    `gorm:"type:text" json:"payload"`
	Retained bool      `json:"retained"` // delivered as a retained replay
}

// TableName overrides the table name for DailySummary
func (DailySummary) TableName() string {
	return "daily_summaries"
}
