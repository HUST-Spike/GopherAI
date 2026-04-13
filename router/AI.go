package router

import (
	agentctl "GopherAI/controller/agent"
	"GopherAI/controller/session"
	skillctl "GopherAI/controller/skill"
	"GopherAI/controller/tts"

	"github.com/gin-gonic/gin"
)

func AIRouter(r *gin.RouterGroup) {

	// 聊天相关接口
	{
		r.GET("/chat/sessions", session.GetUserSessionsByUserName)
		r.POST("/chat/send-new-session", session.CreateSessionAndSendMessage)
		r.POST("/chat/send", session.ChatSend)
		r.POST("/chat/history", session.ChatHistory)

		r.POST("/chat/tts", tts.CreateTTSTask)
		r.GET("/chat/tts/query", tts.QueryTTSTask)

		r.POST("/chat/send-stream-new-session", session.CreateStreamSessionAndSendMessage)
		r.POST("/chat/send-stream", session.ChatStreamSend)
	}

	// Skill 管理接口
	{
		r.GET("/skills/list", skillctl.ListSkills)
		r.POST("/skills/activate", skillctl.ActivateSkill)
		r.POST("/skills/deactivate", skillctl.DeactivateSkill)
		r.POST("/skills/active", skillctl.GetActiveSkills)
	}

	// Agent 接口
	{
		r.POST("/agent/execute", agentctl.ExecuteAgent)
		r.POST("/agent/stream-execute", agentctl.StreamExecuteAgent)
	}
}
