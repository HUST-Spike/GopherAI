package rabbitmq

import (
	"GopherAI/config"
	"encoding/json"
	"time"

	"github.com/streadway/amqp"
)

const DocumentUploadedEventType = "document.uploaded"

type DocumentUploadedEvent struct {
	EventID          string    `json:"event_id"`
	EventType        string    `json:"event_type"`
	DocumentID       string    `json:"document_id"`
	JobID            string    `json:"job_id"`
	UserName         string    `json:"user_name"`
	SessionID        string    `json:"session_id,omitempty"`
	FilePath         string    `json:"file_path"`
	OriginalFilename string    `json:"original_filename"`
	MimeType         string    `json:"mime_type"`
	FileSize         int64     `json:"file_size"`
	TraceID          string    `json:"trace_id"`
	OccurredAt       time.Time `json:"occurred_at"`
	SchemaVersion    int       `json:"schema_version"`
}

func PublishDocumentUploaded(event DocumentUploadedEvent) error {
	if event.EventType == "" {
		event.EventType = DocumentUploadedEventType
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = 1
	}

	if conn == nil {
		initConn()
	}

	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	conf := config.GetConfig()
	exchange := conf.DocumentIndexExchange()
	queue := conf.DocumentIndexQueue()
	routingKey := conf.DocumentIndexRoutingKey()

	if err := ch.ExchangeDeclare(exchange, "direct", true, false, false, false, nil); err != nil {
		return err
	}

	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return err
	}

	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		return err
	}

	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return ch.Publish(exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
}
