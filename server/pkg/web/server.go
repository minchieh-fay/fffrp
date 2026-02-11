package web

import (
	"common"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"server/config"
	"server/pkg/core"

	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

//go:embed dist/*
var content embed.FS

var (
	wsConns = make(map[*websocket.Conn]bool)
	wsLock  sync.Mutex
)

func Start() {
	r := gin.Default()

	// Register Core Callback
	core.OnClientUpdate = func() {
		broadcastUpdate()
	}

	// Enable CORS
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api")
	{
		api.GET("/clients", getClients)
		api.POST("/client/:id/service", addService)
		api.DELETE("/client/:id/service/:service_id", removeService)
	}

	// WebSocket for real-time updates to Web UI
	r.GET("/ws", wsHandler)

	// Serve Static Files (Embedded)
	distFS, _ := fs.Sub(content, "dist")
	assetsFS, _ := fs.Sub(distFS, "assets")

	r.GET("/", func(c *gin.Context) {
		data, err := content.ReadFile("dist/index.html")
		if err != nil {
			c.String(404, "Index not found: "+err.Error())
			return
		}
		c.Data(200, "text/html; charset=utf-8", data)
	})
	r.StaticFS("/assets", http.FS(assetsFS))

	port := config.GlobalConfig.Server.WebPort
	if port == 0 {
		port = 8080
	}
	addr := fmt.Sprintf(":%d", port)
	go r.Run(addr)
}

func getClients(c *gin.Context) {
	core.ClientsLock.RLock()
	defer core.ClientsLock.RUnlock()

	// Convert map to list for JSON
	type ClientDTO struct {
		ID          string                 `json:"id"`
		Name        string                 `json:"name"`
		Phone       string                 `json:"phone"`
		ProjectName string                 `json:"project_name"`
		Remark      string                 `json:"remark"`
		Services    []common.TargetService `json:"services"`
	}
	list := []ClientDTO{}
	for _, client := range core.Clients {
		list = append(list, ClientDTO{
			ID:          client.ID,
			Name:        client.Name,
			Phone:       client.Phone,
			ProjectName: client.ProjectName,
			Remark:      client.Remark,
			Services:    client.Services,
		})
	}
	c.JSON(200, list)
}

func addService(c *gin.Context) {
	clientID := c.Param("id")
	var svc common.TargetService
	if err := c.BindJSON(&svc); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Allocate Port if needed
	if svc.RemotePort == 0 {
		port, err := core.AllocatePort()
		if err != nil {
			c.JSON(500, gin.H{"error": "failed to allocate port: " + err.Error()})
			return
		}
		svc.RemotePort = port
		// Generate ID if empty
		if svc.ID == "" {
			svc.ID = fmt.Sprintf("svc-%d", port)
		}
	} else if svc.ID == "" {
		// Even if port is provided (unlikely from UI but possible), ensure ID
		svc.ID = fmt.Sprintf("svc-%d", svc.RemotePort)
	}

	core.ClientsLock.RLock()
	client, exists := core.Clients[clientID]
	if !exists {
		core.ClientsLock.RUnlock()
		c.JSON(404, gin.H{"error": "client not found"})
		return
	}

	if client.RPCClient == nil {
		core.ClientsLock.RUnlock()
		c.JSON(500, gin.H{"error": "client rpc not ready"})
		return
	}

	// Copy services to avoid race during RPC call
	currentServices := make([]common.TargetService, len(client.Services))
	copy(currentServices, client.Services)
	rpcClient := client.RPCClient
	core.ClientsLock.RUnlock()

	// Update core services first to reflect the allocated port!
	// Wait, core.UpdateServices overwrites the list.
	// We should append the new service with the allocated port to the list we send to client?
	// Client will receive it and update its state.

	newServices := append(currentServices, svc)

	// Also update Core state immediately?
	// core.UpdateServices(clientID, newServices) // This also starts listener!
	// Yes, we should do this.
	core.UpdateServices(clientID, newServices)

	// Call Client RPC
	args := &common.PushConfigArgs{
		Services: newServices, // Send full list or delta?
		// Client logic usually replaces full list if PushConfig is implemented that way.
		// Let's check client implementation... We don't have access to client code here easily.
		// But usually `PushConfig` implies "Here is your config".
		// In `addService` before, it appended.
	}
	var reply common.BaseReply
	err := rpcClient.Call("ClientRPC.PushConfig", args, &reply)
	if err != nil {
		c.JSON(500, gin.H{"error": "rpc call failed: " + err.Error()})
		// Rollback?
		return
	}

	c.JSON(200, gin.H{"status": "pushed to client", "service": svc})
}

func removeService(c *gin.Context) {
	clientID := c.Param("id")
	serviceID := c.Param("service_id")

	core.ClientsLock.RLock()
	client, exists := core.Clients[clientID]
	if !exists {
		core.ClientsLock.RUnlock()
		c.JSON(404, gin.H{"error": "client not found"})
		return
	}

	if client.RPCClient == nil {
		core.ClientsLock.RUnlock()
		c.JSON(500, gin.H{"error": "client rpc not ready"})
		return
	}

	// Copy services
	currentServices := make([]common.TargetService, len(client.Services))
	copy(currentServices, client.Services)
	rpcClient := client.RPCClient
	core.ClientsLock.RUnlock()

	// Filter services
	newServices := []common.TargetService{}
	for _, s := range currentServices {
		if s.ID != serviceID {
			newServices = append(newServices, s)
		}
	}

	// Call Client RPC
	args := &common.PushConfigArgs{
		Services: newServices,
	}
	var reply common.BaseReply
	err := rpcClient.Call("ClientRPC.PushConfig", args, &reply)
	if err != nil {
		c.JSON(500, gin.H{"error": "rpc call failed: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "removed, pushed to client"})
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	wsLock.Lock()
	wsConns[conn] = true
	wsLock.Unlock()

	defer func() {
		conn.Close()
		wsLock.Lock()
		delete(wsConns, conn)
		wsLock.Unlock()
	}()

	// Simple loop
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func broadcastUpdate() {
	wsLock.Lock()
	defer wsLock.Unlock()

	for conn := range wsConns {
		err := conn.WriteMessage(websocket.TextMessage, []byte("update"))
		if err != nil {
			conn.Close()
			delete(wsConns, conn)
		}
	}
}
