package main

import (
	"client/config"
	"client/pkg/core"
	"common"
	"context"
	"fmt"
	"net"
	"net/rpc"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ClientRPC handles Server -> Client calls
type ClientRPC struct{}

// App struct
type App struct {
	ctx        context.Context
	isLoggedIn bool
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize State (Load ClientID)
	core.InitState()

	// Register update callback to notify frontend
	core.OnUpdate = func() {
		// Emit event to frontend
		// We need to send the new status
		status := a.GetStatus()
		runtime.EventsEmit(a.ctx, "state-update", status)
	}

	// Register RPC handler
	core.OnReverseRPC = func(server *rpc.Server, conn net.Conn) {
		server.RegisterName("ClientRPC", &ClientRPC{})
		server.ServeConn(conn)
	}

	// Don't start connection loop automatically
	// go a.startConnectionLoop()
}

// Login sets the user info and attempts to connect
func (a *App) Login(name, phone, projectName, remark string) error {
	core.State.Lock.Lock()
	core.State.Name = name
	core.State.Phone = phone
	core.State.ProjectName = projectName
	core.State.Remark = remark
	core.State.Lock.Unlock()

	serverAddr := config.GlobalConfig.ServerAddr
	fmt.Println("Login: Connecting to", serverAddr)
	err := core.ConnectServer(serverAddr)
	if err != nil {
		fmt.Printf("Login failed: %v\n", err)
		return err
	}

	// Save user info
	config.Save(name, phone, projectName, remark)

	a.isLoggedIn = true
	// Start the KeepAlive/Reconnection loop
	go a.startConnectionLoop()

	return nil
}

func (a *App) startConnectionLoop() {
	serverAddr := config.GlobalConfig.ServerAddr
	ticker := time.NewTicker(5 * time.Second)
	// Don't stop ticker, we loop forever

	for {
		<-ticker.C
		if !a.isLoggedIn {
			// If logged out (future feature), stop loop
			continue
		}
		a.connect(serverAddr)
	}
}

func (a *App) connect(addr string) {
	core.State.Lock.RLock()
	connected := core.State.IsConnected
	core.State.Lock.RUnlock()

	if connected {
		return
	}

	fmt.Println("Connecting to", addr)
	err := core.ConnectServer(addr)
	if err != nil {
		fmt.Printf("Connect failed: %v\n", err)
		runtime.EventsEmit(a.ctx, "connection-state", false)
	} else {
		fmt.Println("Connected!")
		runtime.EventsEmit(a.ctx, "connection-state", true)

		// Sync local services to server
		a.syncLocalServices()
	}
}

func (a *App) syncLocalServices() {
	core.State.Lock.RLock()
	client := core.State.RPCClient
	connected := core.State.IsConnected
	myID := core.State.ClientID
	currentServices := make([]common.TargetService, len(core.State.Services))
	copy(currentServices, core.State.Services)
	core.State.Lock.RUnlock()

	if connected && client != nil {
		fmt.Printf("Syncing %d services to server...\n", len(currentServices))
		args := &common.SyncConfigArgs{
			ClientID: myID,
			Services: currentServices,
		}
		var reply common.BaseReply
		go func() {
			err := client.Call("ServerRPCContext.SyncConfig", args, &reply)
			if err != nil {
				fmt.Printf("SyncConfig failed: %v\n", err)
			}
		}()
	}
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// GetStatus returns the current connection status
func (a *App) GetStatus() map[string]interface{} {
	core.State.Lock.RLock()
	defer core.State.Lock.RUnlock()
	return map[string]interface{}{
		"connected": core.State.IsConnected,
		"client_id": core.State.ClientID,
		"server":    config.GlobalConfig.ServerAddr,
		"services":  core.State.Services,
		"user": map[string]string{
			"name":         config.GlobalConfig.User.Name,
			"phone":        config.GlobalConfig.User.Phone,
			"project_name": config.GlobalConfig.User.ProjectName,
			"remark":       config.GlobalConfig.User.Remark,
		},
	}
}

// AddTarget adds a new target service locally (and syncs to server)
func (a *App) AddTarget(localIP string, localPort int, remotePort int, remark string) string {
	// 1. Create Service
	// Generate unique ID to avoid collision if RemotePort is 0
	svcID := fmt.Sprintf("%s-%d-%d", localIP, localPort, time.Now().UnixNano())
	svc := common.TargetService{
		ID:         svcID,
		LocalIP:    localIP,
		LocalPort:  localPort,
		RemotePort: remotePort,
		Remark:     remark,
	}

	// 2. Add to Local State
	core.State.Lock.Lock()
	core.State.Services = append(core.State.Services, svc)
	core.State.Lock.Unlock()

	// 3. Sync to Server (if connected)
	// We need to call SyncConfig RPC
	// This requires access to RPCClient which is in Core State.
	go func() {
		core.State.Lock.RLock()
		client := core.State.RPCClient
		connected := core.State.IsConnected
		myID := core.State.ClientID
		// Copy services
		currentServices := make([]common.TargetService, len(core.State.Services))
		copy(currentServices, core.State.Services)
		core.State.Lock.RUnlock()

		if connected && client != nil {
			args := &common.SyncConfigArgs{
				ClientID: myID,
				Services: currentServices, // Send full list
			}

			var reply common.BaseReply
			client.Call("ServerRPCContext.SyncConfig", args, &reply)
		}
	}()

	return "Added"
}

// RemoveTarget removes a target service locally (and syncs to server)
func (a *App) RemoveTarget(id string) string {
	// 1. Remove from Local State
	core.State.Lock.Lock()
	newServices := []common.TargetService{}
	for _, s := range core.State.Services {
		if s.ID != id {
			newServices = append(newServices, s)
		}
	}
	core.State.Services = newServices
	core.State.Lock.Unlock()

	// 2. Sync to Server (if connected)
	go func() {
		core.State.Lock.RLock()
		client := core.State.RPCClient
		connected := core.State.IsConnected
		myID := core.State.ClientID
		// Copy services
		currentServices := make([]common.TargetService, len(core.State.Services))
		copy(currentServices, core.State.Services)
		core.State.Lock.RUnlock()

		if connected && client != nil {
			args := &common.SyncConfigArgs{
				ClientID: myID,
				Services: currentServices, // Send full list
			}

			var reply common.BaseReply
			client.Call("ServerRPCContext.SyncConfig", args, &reply)
		}
	}()

	return "Removed"
}

// PushConfig updates local services from server
func (r *ClientRPC) PushConfig(args *common.PushConfigArgs, reply *common.BaseReply) error {
	core.State.Lock.Lock()
	core.State.Services = args.Services
	core.State.Lock.Unlock()

	if core.OnUpdate != nil {
		core.OnUpdate()
	}

	reply.Success = true
	return nil
}
