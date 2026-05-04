package file

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/config"
	documentdao "GopherAI/dao/document"
	"GopherAI/model"
	"GopherAI/utils"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type UploadRagFileResult struct {
	DocumentID string
	FilePath   string
	Status     string
}

// UploadRagFile saves the uploaded file, records metadata in MySQL, and queues a mock indexing task.
func UploadRagFile(username string, file *multipart.FileHeader, sessionID string, traceID string) (*UploadRagFileResult, error) {
	if err := utils.ValidateFile(file); err != nil {
		log.Printf("File validation failed trace_id=%s username=%s filename=%s error=%v", traceID, username, file.Filename, err)
		return nil, err
	}

	conf := config.GetConfig()
	if maxSize := conf.DocumentMaxFileSizeBytes(); file.Size > maxSize {
		return nil, errors.New("uploaded file exceeds configured max size")
	}

	userDir := filepath.Join(conf.DocumentUploadDir(), username)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		log.Printf("Failed to create user directory trace_id=%s dir=%s error=%v", traceID, userDir, err)
		return nil, err
	}

	documentID := utils.GenerateUUID()
	ext := strings.ToLower(filepath.Ext(file.Filename))
	storedFilename := documentID + ext
	filePath := filepath.Join(userDir, storedFilename)
	storageKey := filepath.ToSlash(filePath)

	src, err := file.Open()
	if err != nil {
		log.Printf("Failed to open uploaded file trace_id=%s document_id=%s error=%v", traceID, documentID, err)
		return nil, err
	}
	defer src.Close()

	dst, err := os.Create(filePath)
	if err != nil {
		log.Printf("Failed to create destination file trace_id=%s document_id=%s path=%s error=%v", traceID, documentID, filePath, err)
		return nil, err
	}
	defer dst.Close()

	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(dst, hash), src)
	if err != nil {
		log.Printf("Failed to copy file content trace_id=%s document_id=%s error=%v", traceID, documentID, err)
		return nil, err
	}
	sha256Sum := hex.EncodeToString(hash.Sum(nil))

	mimeType := file.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	doc := &model.Document{
		ID:               documentID,
		UserName:         username,
		SessionID:        sessionID,
		OriginalFilename: file.Filename,
		StoredFilename:   storedFilename,
		FilePath:         storageKey,
		FileExt:          ext,
		MimeType:         mimeType,
		FileSize:         written,
		SHA256:           sha256Sum,
		StorageBackend:   "local",
		StorageKey:       storageKey,
		Status:           model.DocumentStatusUploaded,
		IndexVersion:     1,
		LastTraceID:      traceID,
	}
	if _, err := documentdao.CreateDocument(doc); err != nil {
		log.Printf("Failed to create document record trace_id=%s document_id=%s error=%v", traceID, documentID, err)
		return nil, err
	}

	eventID := utils.GenerateUUID()
	jobID := utils.GenerateUUID()
	event := rabbitmq.DocumentUploadedEvent{
		EventID:          eventID,
		EventType:        rabbitmq.DocumentUploadedEventType,
		DocumentID:       documentID,
		JobID:            jobID,
		UserName:         username,
		SessionID:        sessionID,
		FilePath:         storageKey,
		OriginalFilename: file.Filename,
		MimeType:         mimeType,
		FileSize:         written,
		TraceID:          traceID,
		OccurredAt:       time.Now(),
		SchemaVersion:    1,
	}

	if err := publishDocumentUploadedWithRetry(event); err != nil {
		log.Printf("Failed to publish document event trace_id=%s document_id=%s error=%v", traceID, documentID, err)
		if updateErr := documentdao.MarkDocumentQueueFailed(documentID, traceID, err.Error()); updateErr != nil {
			log.Printf("Failed to mark document queue_failed trace_id=%s document_id=%s error=%v", traceID, documentID, updateErr)
		}
		return nil, err
	}

	now := time.Now()
	if err := documentdao.MarkDocumentQueued(documentID, traceID, now); err != nil {
		log.Printf("Failed to mark document queued trace_id=%s document_id=%s error=%v", traceID, documentID, err)
		return nil, err
	}

	job := &model.DocumentIndexJob{
		ID:         jobID,
		DocumentID: documentID,
		EventID:    eventID,
		TraceID:    traceID,
		UserName:   username,
		QueueName:  conf.DocumentIndexQueue(),
		Status:     model.IndexJobStatusQueued,
		Attempt:    1,
	}
	if _, err := documentdao.CreateIndexJob(job); err != nil {
		log.Printf("Failed to create document index job trace_id=%s document_id=%s job_id=%s error=%v", traceID, documentID, jobID, err)
		return nil, err
	}

	log.Printf("Document queued successfully trace_id=%s document_id=%s path=%s", traceID, documentID, storageKey)
	return &UploadRagFileResult{
		DocumentID: documentID,
		FilePath:   storageKey,
		Status:     model.DocumentStatusQueued,
	}, nil
}

func publishDocumentUploadedWithRetry(event rabbitmq.DocumentUploadedEvent) error {
	delays := []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second}
	var lastErr error
	for attempt := 1; attempt <= len(delays)+1; attempt++ {
		if err := rabbitmq.PublishDocumentUploaded(event); err != nil {
			lastErr = err
			log.Printf("Publish document event failed trace_id=%s document_id=%s attempt=%d error=%v", event.TraceID, event.DocumentID, attempt, err)
			if attempt <= len(delays) {
				time.Sleep(delays[attempt-1])
			}
			continue
		}
		return nil
	}
	return lastErr
}

func ListUserDocuments(userName string) ([]model.Document, error) {
	return documentdao.ListDocumentsByUserName(userName)
}

func GetUserDocument(userName string, documentID string) (*model.Document, error) {
	return documentdao.GetDocumentByIDAndUserName(documentID, userName)
}
