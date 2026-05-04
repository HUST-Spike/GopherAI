package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	DocumentStatusUploaded    = "uploaded"
	DocumentStatusQueued      = "queued"
	DocumentStatusQueueFailed = "queue_failed"
	DocumentStatusIndexing    = "indexing"
	DocumentStatusIndexedMock = "indexed_mock"
	DocumentStatusIndexed     = "indexed"
	DocumentStatusIndexFailed = "index_failed"
	DocumentStatusDeleted     = "deleted"
)

const (
	IndexJobStatusQueued    = "queued"
	IndexJobStatusRunning   = "running"
	IndexJobStatusSucceeded = "succeeded"
	IndexJobStatusFailed    = "failed"
)

type Document struct {
	ID               string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserName         string         `gorm:"index;not null;type:varchar(50)" json:"username"`
	SessionID        string         `gorm:"index;type:varchar(36);default:''" json:"session_id,omitempty"`
	OriginalFilename string         `gorm:"type:varchar(255);not null" json:"original_filename"`
	StoredFilename   string         `gorm:"type:varchar(255);not null" json:"stored_filename"`
	FilePath         string         `gorm:"type:varchar(1024);not null" json:"file_path"`
	FileExt          string         `gorm:"type:varchar(20);not null" json:"file_ext"`
	MimeType         string         `gorm:"type:varchar(100)" json:"mime_type"`
	FileSize         int64          `json:"file_size"`
	SHA256           string         `gorm:"type:char(64);index" json:"sha256"`
	StorageBackend   string         `gorm:"type:varchar(30);default:'local'" json:"storage_backend"`
	StorageBucket    string         `gorm:"type:varchar(255)" json:"storage_bucket,omitempty"`
	StorageKey       string         `gorm:"type:varchar(1024)" json:"storage_key"`
	Status           string         `gorm:"index;type:varchar(30);not null" json:"status"`
	IndexVersion     int            `gorm:"default:1" json:"index_version"`
	ChunkCount       int            `gorm:"default:0" json:"chunk_count"`
	ErrorMessage     string         `gorm:"type:text" json:"error_message,omitempty"`
	LastTraceID      string         `gorm:"type:varchar(36)" json:"last_trace_id,omitempty"`
	LastQueuedAt     *time.Time     `json:"last_queued_at,omitempty"`
	IndexedAt        *time.Time     `json:"indexed_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type DocumentIndexJob struct {
	ID                 string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	DocumentID         string     `gorm:"index;not null;type:varchar(36)" json:"document_id"`
	EventID            string     `gorm:"uniqueIndex;type:varchar(36);not null" json:"event_id"`
	TraceID            string     `gorm:"index;type:varchar(36)" json:"trace_id,omitempty"`
	UserName           string     `gorm:"index;type:varchar(50)" json:"username"`
	QueueName          string     `gorm:"type:varchar(100)" json:"queue_name"`
	Status             string     `gorm:"index;type:varchar(30);not null" json:"status"`
	Attempt            int        `gorm:"default:1" json:"attempt"`
	WorkerID           string     `gorm:"type:varchar(100)" json:"worker_id,omitempty"`
	ChunkCount         int        `gorm:"default:0" json:"chunk_count"`
	MilvusCollection   string     `gorm:"type:varchar(100)" json:"milvus_collection,omitempty"`
	EmbeddingModel     string     `gorm:"type:varchar(100)" json:"embedding_model,omitempty"`
	EmbeddingDimension int        `gorm:"default:0" json:"embedding_dimension"`
	DurationMs         int64      `gorm:"default:0" json:"duration_ms"`
	ProcessAttempts    int        `gorm:"default:1" json:"process_attempts"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	ErrorMessage       string     `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
