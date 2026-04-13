package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryEntry represents a single item in the agent's memory.
type MemoryEntry struct {
	Role     string    `json:"role"`
	Content  string    `json:"content"`
	ToolName string    `json:"tool_name,omitempty"`
	Time     time.Time `json:"time"`
}

// Memory provides short-term (current task) and long-term (cross-task) storage for an Agent.
type Memory struct {
	mu         sync.RWMutex
	shortTerm  []MemoryEntry // current task context
	longTerm   []string      // persisted summaries from past tasks
	maxShort   int
}

func NewMemory() *Memory {
	return &Memory{
		shortTerm: make([]MemoryEntry, 0),
		longTerm:  make([]string, 0),
		maxShort:  50,
	}
}

// AddEntry appends to short-term memory.
func (m *Memory) AddEntry(entry MemoryEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shortTerm = append(m.shortTerm, entry)
	if len(m.shortTerm) > m.maxShort {
		m.shortTerm = m.shortTerm[len(m.shortTerm)-m.maxShort:]
	}
}

// GetShortTerm returns recent entries.
func (m *Memory) GetShortTerm() []MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MemoryEntry, len(m.shortTerm))
	copy(out, m.shortTerm)
	return out
}

// AddLongTermSummary stores a summary from a completed task.
func (m *Memory) AddLongTermSummary(summary string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.longTerm = append(m.longTerm, summary)
}

// GetSummary returns a combined summary of long-term memories.
func (m *Memory) GetSummary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.longTerm) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, s := range m.longTerm {
		sb.WriteString(fmt.Sprintf("[记忆 %d] %s\n", i+1, s))
	}
	return sb.String()
}

// Clear wipes short-term memory (call between tasks).
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shortTerm = m.shortTerm[:0]
}
