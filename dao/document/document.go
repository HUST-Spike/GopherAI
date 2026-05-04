package document

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"time"

	"gorm.io/gorm/clause"
)

func CreateDocument(doc *model.Document) (*model.Document, error) {
	err := mysql.DB.Create(doc).Error
	return doc, err
}

func GetDocumentByID(id string) (*model.Document, error) {
	var doc model.Document
	err := mysql.DB.Where("id = ?", id).First(&doc).Error
	return &doc, err
}

func GetDocumentByIDAndUserName(id string, userName string) (*model.Document, error) {
	var doc model.Document
	err := mysql.DB.Where("id = ? AND user_name = ?", id, userName).First(&doc).Error
	return &doc, err
}

func ListDocumentsByUserName(userName string) ([]model.Document, error) {
	var docs []model.Document
	err := mysql.DB.Where("user_name = ?", userName).Order("created_at desc").Find(&docs).Error
	return docs, err
}

func MarkDocumentQueued(id string, traceID string, queuedAt time.Time) error {
	return mysql.DB.Model(&model.Document{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":         model.DocumentStatusQueued,
			"error_message":  "",
			"last_trace_id":  traceID,
			"last_queued_at": queuedAt,
		}).Error
}

func MarkDocumentQueueFailed(id string, traceID string, errMsg string) error {
	return mysql.DB.Model(&model.Document{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        model.DocumentStatusQueueFailed,
			"error_message": errMsg,
			"last_trace_id": traceID,
		}).Error
}

func CreateIndexJob(job *model.DocumentIndexJob) (*model.DocumentIndexJob, error) {
	err := mysql.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(job).Error
	return job, err
}
