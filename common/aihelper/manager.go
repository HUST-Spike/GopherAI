package aihelper

import (
	"context"
	"log"
	"sync"
)

type AIHelperManager struct {
	helpers map[string]map[string]*AIHelper // map[userName]map[sessionID]*AIHelper
	mu      sync.RWMutex
}

func NewAIHelperManager() *AIHelperManager {
	return &AIHelperManager{
		helpers: make(map[string]map[string]*AIHelper),
	}
}

func (m *AIHelperManager) GetOrCreateAIHelper(userName string, sessionID string, modelType string, config map[string]interface{}) (*AIHelper, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		userHelpers = make(map[string]*AIHelper)
		m.helpers[userName] = userHelpers
	}

	helper, exists := userHelpers[sessionID]
	if exists {
		return helper, nil
	}

	factory := GetGlobalFactory()
	ctx := context.Background()
	helper, err := factory.CreateAIHelper(ctx, modelType, sessionID, config)
	if err != nil {
		return nil, err
	}

	userHelpers[sessionID] = helper
	return helper, nil
}

func (m *AIHelperManager) GetAIHelper(userName string, sessionID string) (*AIHelper, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return nil, false
	}
	helper, exists := userHelpers[sessionID]
	return helper, exists
}

func (m *AIHelperManager) RemoveAIHelper(userName string, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return
	}

	if helper, ok := userHelpers[sessionID]; ok {
		if err := helper.Close(); err != nil {
			log.Printf("failed to close AIHelper for session %s: %v", sessionID, err)
		}
	}
	delete(userHelpers, sessionID)

	if len(userHelpers) == 0 {
		delete(m.helpers, userName)
	}
}

func (m *AIHelperManager) GetUserSessions(userName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return []string{}
	}

	sessionIDs := make([]string, 0, len(userHelpers))
	for sessionID := range userHelpers {
		sessionIDs = append(sessionIDs, sessionID)
	}
	return sessionIDs
}

var (
	globalManager *AIHelperManager
	managerOnce   sync.Once
)

func GetGlobalManager() *AIHelperManager {
	managerOnce.Do(func() {
		globalManager = NewAIHelperManager()
	})
	return globalManager
}
