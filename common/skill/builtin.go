package skill

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// =================== Code Assistant Skill ===================

type CodeAssistantSkill struct{}

func (s *CodeAssistantSkill) Name() string        { return "code_assistant" }
func (s *CodeAssistantSkill) Description() string  { return "专业编程助手，擅长代码生成、审查、重构和调试" }
func (s *CodeAssistantSkill) Version() string      { return "1.0.0" }
func (s *CodeAssistantSkill) RequiredTools() []string { return []string{"run_python"} }

func (s *CodeAssistantSkill) SystemPrompt() string {
	return `你是一个专业的编程助手，擅长代码生成、审查、重构和调试。
规则：
1. 生成的代码需要包含注释说明关键逻辑
2. 发现 bug 时要说明原因和修复方案
3. 重构时要保持向后兼容
4. 代码块使用 markdown 格式化
5. 如果需要运行代码来验证，请使用 run_python 工具`
}

func (s *CodeAssistantSkill) PreProcess(_ context.Context, _ *SkillContext, messages []*schema.Message) ([]*schema.Message, error) {
	return messages, nil
}

func (s *CodeAssistantSkill) Init(_ map[string]interface{}) error { return nil }
func (s *CodeAssistantSkill) Close() error                       { return nil }

// =================== Translator Skill ===================

type TranslatorSkill struct{}

func (s *TranslatorSkill) Name() string        { return "translator" }
func (s *TranslatorSkill) Description() string  { return "专业翻译，支持中英日韩法德等多语言互译" }
func (s *TranslatorSkill) Version() string      { return "1.0.0" }
func (s *TranslatorSkill) RequiredTools() []string { return nil }

func (s *TranslatorSkill) SystemPrompt() string {
	return `你是一个专业翻译，支持中英日韩法德等多语言互译。
规则：
1. 自动识别源语言
2. 保持专业术语的准确性，必要时附注原文
3. 翻译结果保持原文的格式和排版
4. 对于多义词给出多种翻译方案`
}

func (s *TranslatorSkill) PreProcess(_ context.Context, _ *SkillContext, messages []*schema.Message) ([]*schema.Message, error) {
	return messages, nil
}

func (s *TranslatorSkill) Init(_ map[string]interface{}) error { return nil }
func (s *TranslatorSkill) Close() error                       { return nil }

// =================== Data Analyst Skill ===================

type DataAnalystSkill struct{}

func (s *DataAnalystSkill) Name() string        { return "data_analyst" }
func (s *DataAnalystSkill) Description() string  { return "数据分析师，擅长 SQL 查询、统计分析和数据解读" }
func (s *DataAnalystSkill) Version() string      { return "1.0.0" }
func (s *DataAnalystSkill) RequiredTools() []string { return []string{"run_python"} }

func (s *DataAnalystSkill) SystemPrompt() string {
	return `你是一个数据分析师，擅长 SQL 查询、数据可视化和统计分析。
规则：
1. 用户用自然语言描述数据需求时，生成对应的 SQL 查询
2. 分析结果时给出清晰的统计摘要
3. 提供数据洞察和建议
4. 注意数据安全，不要执行 DROP/DELETE 等危险操作`
}

func (s *DataAnalystSkill) PreProcess(_ context.Context, _ *SkillContext, messages []*schema.Message) ([]*schema.Message, error) {
	return messages, nil
}

func (s *DataAnalystSkill) Init(_ map[string]interface{}) error { return nil }
func (s *DataAnalystSkill) Close() error                       { return nil }

// =================== Writing Assistant Skill ===================

type WritingAssistantSkill struct{}

func (s *WritingAssistantSkill) Name() string        { return "writing_assistant" }
func (s *WritingAssistantSkill) Description() string  { return "写作助手，擅长文章润色、摘要生成、内容创作" }
func (s *WritingAssistantSkill) Version() string      { return "1.0.0" }
func (s *WritingAssistantSkill) RequiredTools() []string { return nil }

func (s *WritingAssistantSkill) SystemPrompt() string {
	return `你是一个专业的写作助手。
能力：
1. 文章润色和语法修正
2. 长文摘要生成
3. 多种风格的内容创作（正式/轻松/学术）
4. 根据大纲扩写成完整文章
请根据用户需求选择合适的写作模式。`
}

func (s *WritingAssistantSkill) PreProcess(_ context.Context, _ *SkillContext, messages []*schema.Message) ([]*schema.Message, error) {
	return messages, nil
}

func (s *WritingAssistantSkill) Init(_ map[string]interface{}) error { return nil }
func (s *WritingAssistantSkill) Close() error                       { return nil }

// RegisterBuiltinSkills registers all built-in skills to the given manager.
func RegisterBuiltinSkills(sm *SkillManager) {
	sm.Register(&CodeAssistantSkill{})
	sm.Register(&TranslatorSkill{})
	sm.Register(&DataAnalystSkill{})
	sm.Register(&WritingAssistantSkill{})
}
