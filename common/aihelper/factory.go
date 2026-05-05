package aihelper

import (
	"context"
	"fmt"
	"sync"
)

type ModelCreator func(ctx context.Context, config map[string]interface{}) (AIModel, error)

type AIModelFactory struct {
	mu       sync.RWMutex
	creators map[string]ModelCreator
}

var (
	globalFactory *AIModelFactory
	factoryOnce   sync.Once
)

func GetGlobalFactory() *AIModelFactory {
	factoryOnce.Do(func() {
		globalFactory = &AIModelFactory{
			creators: make(map[string]ModelCreator),
		}
		globalFactory.registerBuiltinCreators()
	})
	return globalFactory
}

func (f *AIModelFactory) registerBuiltinCreators() {
	f.creators["1"] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		return NewOpenAIModel(ctx)
	}

	f.creators["2"] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		username, ok := config["username"].(string)
		if !ok {
			return nil, fmt.Errorf("RAG model requires username")
		}
		sessionID, ok := config["sessionID"].(string)
		if !ok || sessionID == "" {
			return nil, fmt.Errorf("RAG model requires sessionID")
		}
		return NewMilvusRAGModel(ctx, username, sessionID)
	}

	f.creators["3"] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		username, ok := config["username"].(string)
		if !ok {
			return nil, fmt.Errorf("MCP model requires username")
		}
		return NewMCPModel(ctx, username)
	}

	f.creators["4"] = func(ctx context.Context, config map[string]interface{}) (AIModel, error) {
		baseURL, _ := config["baseURL"].(string)
		modelName, ok := config["modelName"].(string)
		if !ok {
			return nil, fmt.Errorf("Ollama model requires modelName")
		}
		return NewOllamaModel(ctx, baseURL, modelName)
	}
}

func (f *AIModelFactory) CreateAIModel(ctx context.Context, modelType string, config map[string]interface{}) (AIModel, error) {
	f.mu.RLock()
	creator, ok := f.creators[modelType]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unsupported model type: %s", modelType)
	}
	return creator(ctx, config)
}

func (f *AIModelFactory) CreateAIHelper(ctx context.Context, modelType string, sessionID string, config map[string]interface{}) (*AIHelper, error) {
	if config == nil {
		config = make(map[string]interface{})
	}
	if _, ok := config["sessionID"]; !ok {
		config["sessionID"] = sessionID
	}
	aiModel, err := f.CreateAIModel(ctx, modelType, config)
	if err != nil {
		return nil, err
	}
	return NewAIHelper(aiModel, sessionID), nil
}

func (f *AIModelFactory) RegisterModel(modelType string, creator ModelCreator) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[modelType] = creator
}
