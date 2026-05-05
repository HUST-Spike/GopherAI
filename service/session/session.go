package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/config"
	"GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// resolveModelType applies the server-side default when the client omits
// modelType. This keeps the new "all-in-one" frontend (which never sends
// modelType) working while still letting smoke tests pin a specific model
// (1=OpenAI, 2=RAG, 3=raw MCP, 4=Ollama, 5=SmartModel).
func resolveModelType(modelType string) string {
	if modelType != "" {
		return modelType
	}
	if d := config.GetConfig().AIModelConfig.DefaultModelType; d != "" {
		return d
	}
	return "5"
}

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
	modelType = resolveModelType(modelType)
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
	modelType = resolveModelType(modelType)
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

	modelType = resolveModelType(modelType)
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

	traceID := uuid.New().String()
	writeEvt := func(evt aihelper.StreamEvent) {
		if _, err := writer.Write(append([]byte("data: "), append(evt.Encode(), []byte("\n\n")...)...)); err != nil {
			log.Println("[SSE] Write error:", err)
			return
		}
		flusher.Flush()
	}

	// First frame ties session + trace together so the frontend (and grep'ing
	// devs) can correlate every later event with the same trace_id row in
	// tool_invocations.
	writeEvt(aihelper.SessionStartEvent(sessionID, traceID))

	cb := func(evt aihelper.StreamEvent) {
		if evt.TraceID == "" {
			evt.TraceID = traceID
		}
		writeEvt(evt)
	}

	ctx := aihelper.WithTraceID(context.Background(), traceID)
	if _, err := helper.StreamResponse(userName, ctx, cb, userQuestion); err != nil {
		log.Println("StreamMessageToExistingSession StreamResponse error:", err)
		writeEvt(aihelper.StreamEvent{Type: "error", Data: err.Error(), TraceID: traceID})
		return code.AIModelFail
	}

	writeEvt(aihelper.StreamEvent{Type: "done", TraceID: traceID})
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
	modelType = resolveModelType(modelType)
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
