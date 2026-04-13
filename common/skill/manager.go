package skill

import (
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// SkillManager maintains a registry of all available skills
// and tracks which skills are activated per session.
type SkillManager struct {
	mu       sync.RWMutex
	registry map[string]Skill              // all registered skills
	active   map[string]map[string]struct{} // sessionID → set of active skill names
}

func NewSkillManager() *SkillManager {
	return &SkillManager{
		registry: make(map[string]Skill),
		active:   make(map[string]map[string]struct{}),
	}
}

// Register adds a skill to the global registry and initializes it.
func (sm *SkillManager) Register(s Skill) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if err := s.Init(nil); err != nil {
		return fmt.Errorf("skill %s init failed: %w", s.Name(), err)
	}
	sm.registry[s.Name()] = s
	return nil
}

// ListSkills returns metadata for all registered skills.
func (sm *SkillManager) ListSkills() []SkillInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	infos := make([]SkillInfo, 0, len(sm.registry))
	for _, s := range sm.registry {
		infos = append(infos, SkillInfo{
			Name:          s.Name(),
			Description:   s.Description(),
			Version:       s.Version(),
			RequiredTools: s.RequiredTools(),
		})
	}
	return infos
}

// Activate turns on a skill for a given session.
func (sm *SkillManager) Activate(sessionID, skillName string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if _, ok := sm.registry[skillName]; !ok {
		return fmt.Errorf("skill %q not found", skillName)
	}
	if sm.active[sessionID] == nil {
		sm.active[sessionID] = make(map[string]struct{})
	}
	sm.active[sessionID][skillName] = struct{}{}
	return nil
}

// Deactivate turns off a skill for a given session.
func (sm *SkillManager) Deactivate(sessionID, skillName string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if set, ok := sm.active[sessionID]; ok {
		delete(set, skillName)
		if len(set) == 0 {
			delete(sm.active, sessionID)
		}
	}
}

// GetActiveSkills returns the ordered list of active skills for a session.
func (sm *SkillManager) GetActiveSkills(sessionID string) []Skill {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	set, ok := sm.active[sessionID]
	if !ok {
		return nil
	}
	skills := make([]Skill, 0, len(set))
	for name := range set {
		if s, exists := sm.registry[name]; exists {
			skills = append(skills, s)
		}
	}
	return skills
}

// GetActiveSkillNames returns just the names.
func (sm *SkillManager) GetActiveSkillNames(sessionID string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	set, ok := sm.active[sessionID]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	return names
}

// BuildSystemPrompt merges all active skills' system prompts into one.
func (sm *SkillManager) BuildSystemPrompt(sessionID string) string {
	skills := sm.GetActiveSkills(sessionID)
	if len(skills) == 0 {
		return ""
	}
	prompts := make([]string, 0, len(skills))
	for _, s := range skills {
		if p := s.SystemPrompt(); p != "" {
			prompts = append(prompts, p)
		}
	}
	return strings.Join(prompts, "\n\n---\n\n")
}

// InjectSystemPrompt prepends the merged system prompt to the message list if any skills are active.
func (sm *SkillManager) InjectSystemPrompt(sessionID string, messages []*schema.Message) []*schema.Message {
	prompt := sm.BuildSystemPrompt(sessionID)
	if prompt == "" {
		return messages
	}
	return append([]*schema.Message{
		{Role: schema.System, Content: prompt},
	}, messages...)
}

// CleanupSession removes all skill state for a session.
func (sm *SkillManager) CleanupSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.active, sessionID)
}

// SkillInfo is the JSON-serializable metadata of a skill.
type SkillInfo struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Version       string   `json:"version"`
	RequiredTools []string `json:"required_tools"`
}

// Global SkillManager singleton.
var (
	globalSkillManager *SkillManager
	skillManagerOnce   sync.Once
)

func GetGlobalSkillManager() *SkillManager {
	skillManagerOnce.Do(func() {
		globalSkillManager = NewSkillManager()
	})
	return globalSkillManager
}
