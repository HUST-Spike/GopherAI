package message

import (
	"GopherAI/common/mysql"
	"GopherAI/model"
	"time"
)

func GetMessagesBySessionID(sessionID string) ([]model.Message, error) {
	var msgs []model.Message
	err := mysql.DB.Where("session_id = ?", sessionID).Order("created_at asc").Find(&msgs).Error
	return msgs, err
}

func GetMessagesBySessionIDs(sessionIDs []string) ([]model.Message, error) {
	var msgs []model.Message
	if len(sessionIDs) == 0 {
		return msgs, nil
	}
	err := mysql.DB.Where("session_id IN ?", sessionIDs).Order("created_at asc").Find(&msgs).Error
	return msgs, err
}

func CreateMessage(message *model.Message) (*model.Message, error) {
	if err := mysql.DB.Create(message).Error; err != nil {
		return message, err
	}
	touchedAt := message.CreatedAt
	if touchedAt.IsZero() {
		touchedAt = time.Now()
	}
	err := mysql.DB.Model(&model.Session{}).
		Where("id = ?", message.SessionID).
		Update("updated_at", touchedAt).Error
	return message, err
}

func GetAllMessages() ([]model.Message, error) {
	var msgs []model.Message
	err := mysql.DB.Order("created_at asc").Find(&msgs).Error
	return msgs, err
}
