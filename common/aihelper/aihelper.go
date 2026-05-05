package aihelper

import (
	"GopherAI/common/rabbitmq"
	"GopherAI/common/skill"
	"GopherAI/config"
	"GopherAI/model"
	"GopherAI/utils"
	"context"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type AIHelper struct {
	model     AIModel
	messages  []*model.Message
	mu        sync.RWMutex
	SessionID string
	saveFunc  func(*model.Message) (*model.Message, error)
}

const baseChatSystemPrompt = `你是 GopherAI，一个面向用户的中文智能助手。

回答时优先直接解决用户问题，必要时可以使用工具，但不要主动罗列、解释或暴露内部工具名称、工具注册表、函数调用参数或系统实现细节。只有当用户明确询问工具能力、调试细节或要求说明调用过程时，才简要说明相关工具行为。

如果使用了工具，请把工具结果整合成自然语言回答。`

func NewAIHelper(aiModel AIModel, sessionID string) *AIHelper {
	return &AIHelper{
		model:    aiModel,
		messages: make([]*model.Message, 0),
		saveFunc: func(msg *model.Message) (*model.Message, error) {
			data := rabbitmq.GenerateMessageMQParam(msg.SessionID, msg.Content, msg.UserName, msg.IsUser)
			err := rabbitmq.RMQMessage.Publish(data)
			return msg, err
		},
		SessionID: sessionID,
	}
}

func (a *AIHelper) AddMessage(content string, userName string, isUser bool, save bool) {
	role := model.RoleAssistant
	if isUser {
		role = model.RoleUser
	}
	a.AddMessageWithRole(content, userName, role, save)
}

func (a *AIHelper) AddMessageWithRole(content string, userName string, role string, save bool) {
	msg := model.Message{
		SessionID: a.SessionID,
		Content:   content,
		UserName:  userName,
		Role:      role,
		IsUser:    role == model.RoleUser,
	}
	a.mu.Lock()
	a.messages = append(a.messages, &msg)
	a.mu.Unlock()

	if save {
		a.saveFunc(&msg)
	}
}

func (a *AIHelper) SetSaveFunc(saveFunc func(*model.Message) (*model.Message, error)) {
	a.saveFunc = saveFunc
}

func (a *AIHelper) GetMessages() []*model.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*model.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

func (a *AIHelper) GenerateResponse(userName string, ctx context.Context, userQuestion string) (*model.Message, error) {
	a.AddMessage(userQuestion, userName, true, true)

	a.mu.RLock()
	messages := utils.ConvertToSchemaMessages(a.messages)
	a.mu.RUnlock()

	messages = a.prepareMessages(messages)

	schemaMsg, err := a.model.GenerateResponse(ctx, messages)
	if err != nil {
		return nil, err
	}

	modelMsg := utils.ConvertToModelMessage(a.SessionID, userName, schemaMsg)
	a.AddMessage(modelMsg.Content, userName, false, true)

	return modelMsg, nil
}

func (a *AIHelper) StreamResponse(userName string, ctx context.Context, cb StreamCallback, userQuestion string) (*model.Message, error) {
	a.AddMessage(userQuestion, userName, true, true)

	a.mu.RLock()
	messages := utils.ConvertToSchemaMessages(a.messages)
	a.mu.RUnlock()

	messages = a.prepareMessages(messages)

	content, err := a.model.StreamResponse(ctx, messages, cb)
	if err != nil {
		return nil, err
	}

	modelMsg := &model.Message{
		SessionID: a.SessionID,
		UserName:  userName,
		Content:   content,
		Role:      model.RoleAssistant,
		IsUser:    false,
	}
	a.AddMessage(modelMsg.Content, userName, false, true)

	return modelMsg, nil
}

func (a *AIHelper) prepareMessages(messages []*schema.Message) []*schema.Message {
	prepared := append([]*schema.Message{
		{Role: schema.System, Content: baseChatSystemPrompt},
	}, messages...)

	if config.GetConfig().SmartChat().EnableSkills {
		prepared = skill.GetGlobalSkillManager().InjectSystemPrompt(a.SessionID, prepared)
	}
	return prepared
}

func (a *AIHelper) GetModelType() string {
	return a.model.GetModelType()
}

func (a *AIHelper) Close() error {
	return a.model.Close()
}
