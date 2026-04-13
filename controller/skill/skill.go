package skill

import (
	"GopherAI/common/code"
	"GopherAI/common/skill"
	"GopherAI/controller"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	ListSkillsResponse struct {
		controller.Response
		Skills []skill.SkillInfo `json:"skills"`
	}

	ActivateRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
		SkillName string `json:"skillName" binding:"required"`
	}

	ActiveSkillsRequest struct {
		SessionID string `json:"sessionId" binding:"required"`
	}

	ActiveSkillsResponse struct {
		controller.Response
		ActiveSkills []string `json:"activeSkills"`
	}
)

func ListSkills(c *gin.Context) {
	sm := skill.GetGlobalSkillManager()
	res := &ListSkillsResponse{}
	res.Success()
	res.Skills = sm.ListSkills()
	c.JSON(http.StatusOK, res)
}

func ActivateSkill(c *gin.Context) {
	var req ActivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, new(controller.Response).CodeOf(code.CodeInvalidParams))
		return
	}

	sm := skill.GetGlobalSkillManager()
	if err := sm.Activate(req.SessionID, req.SkillName); err != nil {
		c.JSON(http.StatusOK, new(controller.Response).CodeOf(code.CodeServerBusy))
		return
	}

	res := &controller.Response{}
	res.Success()
	c.JSON(http.StatusOK, res)
}

func DeactivateSkill(c *gin.Context) {
	var req ActivateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, new(controller.Response).CodeOf(code.CodeInvalidParams))
		return
	}

	sm := skill.GetGlobalSkillManager()
	sm.Deactivate(req.SessionID, req.SkillName)

	res := &controller.Response{}
	res.Success()
	c.JSON(http.StatusOK, res)
}

func GetActiveSkills(c *gin.Context) {
	var req ActiveSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, new(controller.Response).CodeOf(code.CodeInvalidParams))
		return
	}

	sm := skill.GetGlobalSkillManager()
	res := &ActiveSkillsResponse{}
	res.Success()
	res.ActiveSkills = sm.GetActiveSkillNames(req.SessionID)
	if res.ActiveSkills == nil {
		res.ActiveSkills = []string{}
	}
	c.JSON(http.StatusOK, res)
}
