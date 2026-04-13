package agent

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	agentsvc "GopherAI/service/agent"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type (
	AgentExecuteRequest struct {
		Task      string `json:"task" binding:"required"`
		SessionID string `json:"sessionId"`
	}

	AgentExecuteResponse struct {
		controller.Response
		SessionID   string `json:"sessionId"`
		FinalAnswer string `json:"finalAnswer"`
		TotalSteps  int    `json:"totalSteps"`
	}
)

// ExecuteAgent runs an agent task synchronously.
func ExecuteAgent(c *gin.Context) {
	req := new(AgentExecuteRequest)
	res := new(AgentExecuteResponse)
	userName := c.GetString("userName")

	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	result, c_ := agentsvc.ExecuteTask(userName, sessionID, req.Task)
	if c_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(c_))
		return
	}

	res.Success()
	res.SessionID = sessionID
	res.FinalAnswer = result.FinalAnswer
	res.TotalSteps = result.TotalSteps
	c.JSON(http.StatusOK, res)
}

// StreamExecuteAgent runs an agent task with SSE streaming.
func StreamExecuteAgent(c *gin.Context) {
	req := new(AgentExecuteRequest)
	userName := c.GetString("userName")

	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, new(controller.Response).CodeOf(code.CodeInvalidParams))
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	// Send sessionId first
	c.Writer.WriteString(`data: {"type":"session","content":"` + sessionID + `","step":0}` + "\n\n")
	c.Writer.Flush()

	agentsvc.StreamExecuteTask(userName, sessionID, req.Task, http.ResponseWriter(c.Writer))
}
