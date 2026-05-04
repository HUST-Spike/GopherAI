package file

import (
	"GopherAI/common/code"
	"GopherAI/controller"
	"GopherAI/model"
	filesvc "GopherAI/service/file"
	"GopherAI/utils"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	UploadFileResponse struct {
		FilePath   string `json:"file_path,omitempty"`
		DocumentID string `json:"document_id,omitempty"`
		SessionID  string `json:"session_id,omitempty"`
		Status     string `json:"status,omitempty"`
		controller.Response
	}

	ListDocumentsResponse struct {
		Documents []model.Document `json:"documents"`
		controller.Response
	}

	GetDocumentResponse struct {
		Document *model.Document `json:"document,omitempty"`
		controller.Response
	}
)

func UploadRagFile(c *gin.Context) {
	res := new(UploadFileResponse)
	traceID := c.GetHeader("X-Trace-ID")
	if traceID == "" {
		traceID = utils.GenerateUUID()
	}
	c.Header("X-Trace-ID", traceID)

	uploadedFile, err := c.FormFile("file")
	if err != nil {
		log.Println("FormFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	username := c.GetString("userName")
	if username == "" {
		log.Println("Username not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	sessionID := c.PostForm("session_id")
	if sessionID == "" {
		sessionID = c.PostForm("sessionId")
	}

	result, err := filesvc.UploadRagFile(username, uploadedFile, sessionID, traceID)
	if err != nil {
		log.Println("UploadFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.FilePath = result.FilePath
	res.DocumentID = result.DocumentID
	res.SessionID = result.SessionID
	res.Status = result.Status
	c.JSON(http.StatusOK, res)
}

func ListDocuments(c *gin.Context) {
	res := new(ListDocumentsResponse)
	username := c.GetString("userName")
	if username == "" {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	docs, err := filesvc.ListUserDocuments(username)
	if err != nil {
		log.Println("ListDocuments fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.Documents = docs
	c.JSON(http.StatusOK, res)
}

func GetDocument(c *gin.Context) {
	res := new(GetDocumentResponse)
	username := c.GetString("userName")
	if username == "" {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	documentID := c.Param("id")
	if documentID == "" {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	doc, err := filesvc.GetUserDocument(username, documentID)
	if err != nil {
		log.Println("GetDocument fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeRecordNotFound))
		return
	}

	res.Success()
	res.Document = doc
	c.JSON(http.StatusOK, res)
}
