package session

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/code"
	"GopherAI/common/skill"
	"GopherAI/config"
	messagedao "GopherAI/dao/message"
	"GopherAI/dao/session"
	"GopherAI/model"
	"context"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

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
	dbSessions, err := session.GetSessionsByUserName(userName)
	if err != nil {
		return nil, err
	}

	sessionIDs := make([]string, 0, len(dbSessions))
	for _, s := range dbSessions {
		sessionIDs = append(sessionIDs, s.ID)
	}

	firstUserMessage := make(map[string]string, len(dbSessions))
	lastActivity := make(map[string]time.Time, len(dbSessions))
	if messages, err := messagedao.GetMessagesBySessionIDs(sessionIDs); err == nil {
		for _, msg := range messages {
			if msg.IsUser && firstUserMessage[msg.SessionID] == "" {
				firstUserMessage[msg.SessionID] = msg.Content
			}
			if msg.CreatedAt.After(lastActivity[msg.SessionID]) {
				lastActivity[msg.SessionID] = msg.CreatedAt
			}
		}
	} else {
		log.Println("GetUserSessionsByUserName GetMessagesBySessionIDs error:", err)
	}

	activityAt := func(s model.Session) time.Time {
		if t := lastActivity[s.ID]; !t.IsZero() {
			return t
		}
		if !s.UpdatedAt.IsZero() {
			return s.UpdatedAt
		}
		return s.CreatedAt
	}

	sort.SliceStable(dbSessions, func(i, j int) bool {
		return activityAt(dbSessions[i]).After(activityAt(dbSessions[j]))
	})

	infos := make([]model.SessionInfo, 0, len(dbSessions))
	for _, s := range dbSessions {
		titleSource := s.Title
		if strings.TrimSpace(titleSource) == "" || titleSource == s.ID {
			titleSource = firstUserMessage[s.ID]
		}
		infos = append(infos, model.SessionInfo{
			SessionID: s.ID,
			Title:     makeSessionTitle(titleSource),
			UpdatedAt: activityAt(s),
		})
	}
	return infos, nil
}

func makeSessionTitle(content string) string {
	const maxRunes = 32
	title := strings.Join(strings.Fields(content), " ")
	title = strings.Trim(title, " \t\r\n#*_`-")
	if title == "" {
		return "新会话"
	}
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	return string(runes[:maxRunes]) + "..."
}

func activateInitialSkills(sessionID string, activeSkills []string) {
	if len(activeSkills) == 0 {
		return
	}
	sm := skill.GetGlobalSkillManager()
	seen := make(map[string]struct{}, len(activeSkills))
	for _, name := range activeSkills {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if err := sm.Activate(sessionID, name); err != nil {
			log.Printf("activate initial skill %q for session %s failed: %v", name, sessionID, err)
		}
	}
}

func hydrateHelperFromDB(userName, sessionID string, helper *aihelper.AIHelper) {
	if helper == nil || len(helper.GetMessages()) > 0 {
		return
	}
	messages, err := messagedao.GetMessagesBySessionID(sessionID)
	if err != nil {
		log.Println("hydrateHelperFromDB GetMessagesBySessionID error:", err)
		return
	}
	for _, msg := range messages {
		msgUserName := msg.UserName
		if msgUserName == "" {
			msgUserName = userName
		}
		helper.AddMessageWithRole(msg.Content, msgUserName, msg.GetRole(), false)
	}
}

func CreateSessionAndSendMessage(userName string, userQuestion string, modelType string, activeSkills []string) (string, string, code.Code) {
	modelType = resolveModelType(modelType)
	newSession := &model.Session{
		ID:        uuid.New().String(),
		UserName:  userName,
		Title:     makeSessionTitle(userQuestion),
		ModelType: modelType,
	}
	createdSession, err := session.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		return "", "", code.CodeServerBusy
	}
	activateInitialSkills(createdSession.ID, activeSkills)

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
	hydrateHelperFromDB(userName, createdSession.ID, helper)

	ctx := context.Background()
	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("CreateSessionAndSendMessage GenerateResponse error:", err)
		return "", "", code.AIModelFail
	}

	return createdSession.ID, aiResponse.Content, code.CodeSuccess
}

func CreateStreamSessionOnly(userName string, userQuestion string, modelType string, activeSkills []string) (string, code.Code) {
	modelType = resolveModelType(modelType)
	newSession := &model.Session{
		ID:        uuid.New().String(),
		UserName:  userName,
		Title:     makeSessionTitle(userQuestion),
		ModelType: modelType,
	}
	createdSession, err := session.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	activateInitialSkills(createdSession.ID, activeSkills)
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
	hydrateHelperFromDB(userName, sessionID, helper)

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
	sessionID, c := CreateStreamSessionOnly(userName, userQuestion, modelType, nil)
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
	hydrateHelperFromDB(userName, sessionID, helper)

	ctx := context.Background()
	aiResponse, err := helper.GenerateResponse(userName, ctx, userQuestion)
	if err != nil {
		log.Println("ChatSend GenerateResponse error:", err)
		return "", code.AIModelFail
	}

	return aiResponse.Content, code.CodeSuccess
}

func GetChatHistory(userName string, sessionID string) ([]model.History, code.Code) {
	dbSession, err := session.GetSessionByID(sessionID)
	if err != nil {
		log.Println("GetChatHistory GetSessionByID error:", err)
		return nil, code.CodeRecordNotFound
	}
	if dbSession.UserName != userName {
		return nil, code.CodeForbidden
	}

	dbMessages, err := messagedao.GetMessagesBySessionID(sessionID)
	if err == nil && len(dbMessages) > 0 {
		history := make([]model.History, 0, len(dbMessages))
		for _, msg := range dbMessages {
			history = append(history, model.History{
				IsUser:   msg.IsUser,
				Role:     msg.GetRole(),
				Content:  msg.Content,
				ToolName: msg.ToolName,
			})
		}
		return history, code.CodeSuccess
	}
	if err != nil {
		log.Println("GetChatHistory GetMessagesBySessionID error:", err)
	}

	manager := aihelper.GetGlobalManager()
	helper, exists := manager.GetAIHelper(userName, sessionID)
	if !exists {
		return []model.History{}, code.CodeSuccess
	}

	helperMessages := helper.GetMessages()
	history := make([]model.History, 0, len(helperMessages))
	for _, msg := range helperMessages {
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
