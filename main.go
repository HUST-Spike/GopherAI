package main

import (
	"GopherAI/common/aihelper"
	"GopherAI/common/logging"
	mcpserver "GopherAI/common/mcp/server"
	"GopherAI/common/mcp/server/tools"
	"GopherAI/common/mysql"
	"GopherAI/common/rabbitmq"
	"GopherAI/common/redis"
	"GopherAI/common/skill"
	"GopherAI/config"
	"GopherAI/dao/message"
	sessiondao "GopherAI/dao/session"
	"GopherAI/router"
	"fmt"
	"log"
	"time"

	"github.com/joho/godotenv"
)

func StartServer(addr string, port int) error {
	r := router.InitRouter()
	return r.Run(fmt.Sprintf("%s:%d", addr, port))
}

// readDataFromDB loads messages from DB and rebuilds AIHelperManager state.
// It reads each session's ModelType so MCP/RAG sessions are correctly restored.
func readDataFromDB() error {
	manager := aihelper.GetGlobalManager()

	sessions, err := sessiondao.GetAllSessions()
	if err != nil {
		return fmt.Errorf("failed to load sessions: %v", err)
	}
	sessionModelType := make(map[string]string, len(sessions))
	for _, s := range sessions {
		mt := s.ModelType
		if mt == "" {
			mt = config.GetConfig().AIModelConfig.DefaultModelType
		}
		sessionModelType[s.ID] = mt
	}

	msgs, err := message.GetAllMessages()
	if err != nil {
		return fmt.Errorf("failed to load messages: %v", err)
	}

	for i := range msgs {
		m := &msgs[i]
		modelType := sessionModelType[m.SessionID]
		if modelType == "" {
			modelType = config.GetConfig().AIModelConfig.DefaultModelType
		}
		cfg := map[string]interface{}{
			"username":  m.UserName,
			"sessionID": m.SessionID,
		}

		helper, err := manager.GetOrCreateAIHelper(m.UserName, m.SessionID, modelType, cfg)
		if err != nil {
			log.Printf("[readDataFromDB] failed to create helper for user=%s session=%s: %v", m.UserName, m.SessionID, err)
			continue
		}
		helper.AddMessage(m.Content, m.UserName, m.IsUser, false)
	}

	log.Println("AIHelperManager init success")
	return nil
}

// startMCPServer launches the embedded MCP server in a goroutine if enabled.
//
// Tool registration must happen here, BEFORE the MCP server starts listening,
// otherwise clients connecting during the brief boot window would see an
// empty tools/list response.
func startMCPServer(conf *config.Config) {
	if !conf.MCPConfig.Enabled || !conf.MCPConfig.AutoStart {
		log.Println("MCP server is disabled, skipping")
		return
	}

	tools.RegisterAll(mcpserver.DefaultRegistry)
	log.Printf("MCP tools registered: %d", mcpserver.DefaultRegistry.Count())

	go func() {
		addr := conf.MCPConfig.ServerAddr
		log.Printf("Starting embedded MCP server on %s ...", addr)
		if err := mcpserver.StartServer(addr); err != nil {
			log.Printf("MCP server exited with error: %v", err)
		}
	}()

	// Give MCP server a moment to bind the port before clients connect.
	time.Sleep(500 * time.Millisecond)
	log.Println("MCP server started")
}

func main() {
	if err := logging.InitGoLogger(); err != nil {
		log.Printf("InitGoLogger warning: %v", err)
	}

	if err := godotenv.Load("config/.env"); err != nil {
		log.Printf("config/.env not loaded: %v", err)
	}

	conf := config.GetConfig()

	if err := mysql.InitMysql(); err != nil {
		log.Fatalf("InitMysql error: %v", err)
	}

	startMCPServer(conf)

	skill.RegisterBuiltinSkills(skill.GetGlobalSkillManager())
	log.Printf("Skills registered: %d", len(skill.GetGlobalSkillManager().ListSkills()))

	if err := readDataFromDB(); err != nil {
		log.Printf("readDataFromDB warning: %v", err)
	}

	redis.Init()
	log.Println("redis init success")

	rabbitmq.InitRabbitMQ()
	log.Println("rabbitmq init success")

	host := conf.MainConfig.Host
	port := conf.MainConfig.Port
	if err := StartServer(host, port); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
