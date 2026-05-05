package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
)

func GetUserSessionsByUserName(userName string) ([]model.SessionInfo, error) {
	manager := aihelper.GetGlobalManager()
	sessions := manager.GetUserSessions(userName)

	var infos []model.SessionInfo
	for _, s := range sessions {
		infos = append(infos, model.SessionInfo{
			SessionID: s,
			Title:     s,
		})
	}
	return infos, nil
}

func CreateSessionAndSendMessage(userName string, userQuestion string, modelType string) (string, string, code.Code) {
	newSession := &model.Session{
		ID:        uuid.New().String(),
		UserName:  userName,
		Title:     userQuestion,
		ModelType: modelType,
	}
	createdSession, err := session.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		return "", "", code.CodeServerBusy
	}

	manager := aihelper.GetGlobalManager()
	cfg := map[string]interface{}{
		"username":  userName,
		"sessionID": createdSession.ID,
	}
	helper, err := manager.GetOrCreateAIHelper(userName, createdSession.ID, modelType, cfg)
	if err != nil {
		log.Println("CreateSessionAndSendMessage GetOrCreateAIHelper error:", err)
		return "", "", code.AIModelFail
	}

	ctx := context.Background()
	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
		return "", "", code.AIModelFail
	}

	return createdSession.ID, aiResponse.Content, code.CodeSuccess
}

func CreateStreamSessionOnly(userName string, userQuestion string, modelType string) (string, code.Code) {
	newSession := &model.Session{
		ID:        uuid.New().String(),
		UserName:  userName,
		Title:     userQuestion,
		ModelType: modelType,
	}
	createdSession, err := session.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	return createdSession.ID, code.CodeSuccess
}

func StreamMessageToExistingSession(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		return code.CodeServerBusy
	}

	manager := aihelper.GetGlobalManager()
	cfg := map[string]interface{}{
		"username":  userName,
		"sessionID": sessionID,
	}
	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, cfg)
	if err != nil {
		log.Println("StreamMessageToExistingSession GetOrCreateAIHelper error:", err)
		return code.AIModelFail
	}

	cb := func(msg string) {
		_, err := writer.Write([]byte("data: " + msg + "\n\n"))
		if err != nil {
			log.Println("[SSE] Write error:", err)
			return
		}
		flusher.Flush()
	}

	ctx := context.Background()
	_, err = helper.StreamResponse(userName, ctx, cb, userQuestion)
	if err != nil {
		log.Println("StreamMessageToExistingSession StreamResponse error:", err)
		return code.AIModelFail
	}

	_, err = writer.Write([]byte("data: [DONE]\n\n"))
	if err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		return code.AIModelFail
	}
	flusher.Flush()

	return code.CodeSuccess
}

func CreateStreamSessionAndSendMessage(userName string, userQuestion string, modelType string, writer http.ResponseWriter) (string, code.Code) {
	sessionID, c := CreateStreamSessionOnly(userName, userQuestion, modelType)
	if c != code.CodeSuccess {
		return "", c
	}

	c = StreamMessageToExistingSession(userName, sessionID, userQuestion, modelType, writer)
	if c != code.CodeSuccess {
		return sessionID, c
	}

	return sessionID, code.CodeSuccess
}

func ChatSend(userName string, sessionID string, userQuestion string, modelType string) (string, code.Code) {
	manager := aihelper.GetGlobalManager()
	cfg := map[string]interface{}{
		"username":  userName,
		"sessionID": sessionID,
	}
	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, cfg)
	if err != nil {
		log.Println("ChatSend GetOrCreateAIHelper error:", err)
		return "", code.AIModelFail
	}

	ctx := context.Background()
	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("ChatSend GenerateResponse error:", err)
		return "", code.AIModelFail
	}

	return aiResponse.Content, code.CodeSuccess
}

func GetChatHistory(userName string, sessionID string) ([]model.History, code.Code) {
	manager := aihelper.GetGlobalManager()
	helper, exists := manager.GetAIHelper(userName, sessionID)
	if !exists {
		return nil, code.CodeServerBusy
	}

	messages := helper.GetMessages()
	history := make([]model.History, 0, len(messages))
	for _, msg := range messages {
		history = append(history, model.History{
			IsUser:  msg.IsUser,
			Role:    msg.GetRole(),
			Content: msg.Content,
		})
	}
	return history, code.CodeSuccess
}

func ChatStreamSend(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSession(userName, sessionID, userQuestion, modelType, writer)
}
