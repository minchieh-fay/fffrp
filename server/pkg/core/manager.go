package core

import (
	"common"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"server/config"
	"sync"

	"github.com/hashicorp/yamux"
)

// ClientSession manages a connected client
type ClientSession struct {
	ID        string
	Session   *yamux.Session
	RPCClient *rpc.Client // For S->C calls
	Services  []common.TargetService

	// User Info
	Name        string
	Phone       string
	ProjectName string
	Remark      string
}

var (
	Clients        = make(map[string]*ClientSession)
	ClientsLock    sync.RWMutex
	Listeners      = make(map[int]net.Listener) // Public Port -> Listener
	ListenerLock   sync.Mutex
	OnClientUpdate func()
)

// AddClient registers a new client
func AddClient(id string, session *yamux.Session, rpcClient *rpc.Client, name, phone, projectName, remark string) *ClientSession {
	ClientsLock.Lock()
	defer ClientsLock.Unlock()

	// If exists, remove old one first?
	if old, exists := Clients[id]; exists {
		log.Printf("[Core] Client %s re-connected, closing old session", id)
		// Clean up listeners for old client!
		for _, svc := range old.Services {
			StopPublicListener(svc.RemotePort)
		}
		old.Session.Close()
		delete(Clients, id)
	}

	client := &ClientSession{
		ID:          id,
		Session:     session,
		RPCClient:   rpcClient,
		Services:    []common.TargetService{},
		Name:        name,
		Phone:       phone,
		ProjectName: projectName,
		Remark:      remark,
	}
	Clients[id] = client
	log.Printf("[Core] Client %s registered", id)

	if OnClientUpdate != nil {
		OnClientUpdate()
	}
	return client
}

// RemoveClientBySession finds and removes a client by session
func RemoveClientBySession(session *yamux.Session) {
	ClientsLock.Lock()
	defer ClientsLock.Unlock()

	var targetID string
	for id, c := range Clients {
		if c.Session == session {
			targetID = id
			break
		}
	}

	if targetID != "" {
		log.Printf("[Core] Removing client %s due to session disconnect", targetID)
		client := Clients[targetID]
		// Close all listeners
		for _, svc := range client.Services {
			StopPublicListener(svc.RemotePort)
		}
		delete(Clients, targetID)
		client.Session.Close() // Ensure closed
	}
	if OnClientUpdate != nil {
		OnClientUpdate()
	}
}

// StopPublicListener stops a listener
func StopPublicListener(port int) {
	ListenerLock.Lock()
	defer ListenerLock.Unlock()

	if ln, exists := Listeners[port]; exists {
		ln.Close()
		delete(Listeners, port)
		log.Printf("[Core] Stopped listener on port %d", port)
	}
}

// UpdateServices updates the service list for a client and manages listeners
func UpdateServices(clientID string, services []common.TargetService) {
	ClientsLock.Lock()
	client, exists := Clients[clientID]
	if !exists {
		ClientsLock.Unlock()
		return
	}

	// Preserve existing allocated ports if ID matches
	// Map ID -> Old Service
	oldServices := make(map[string]common.TargetService)
	for _, s := range client.Services {
		oldServices[s.ID] = s
	}

	// Update new services
	updatedServices := make([]common.TargetService, len(services))
	for i, svc := range services {
		if svc.RemotePort == 0 {
			// Check if we have an existing allocation for this ID
			if old, ok := oldServices[svc.ID]; ok && old.RemotePort != 0 {
				svc.RemotePort = old.RemotePort
			} else {
				// Allocate new
				port, err := AllocatePort()
				if err != nil {
					log.Printf("[Core] Failed to allocate port for service %s: %v", svc.ID, err)
					// Skip or keep 0? Keep 0 and maybe fail later or try again next time
				} else {
					svc.RemotePort = port
				}
			}
		}
		updatedServices[i] = svc
	}

	// Manage Listeners
	// 1. Close ports no longer needed
	newServiceIDs := make(map[string]bool)
	for _, svc := range updatedServices {
		newServiceIDs[svc.ID] = true
	}

	for _, oldSvc := range client.Services {
		if !newServiceIDs[oldSvc.ID] {
			log.Printf("[Core] Service %s removed, stopping listener on port %d", oldSvc.ID, oldSvc.RemotePort)
			StopPublicListener(oldSvc.RemotePort)
		}
	}

	client.Services = updatedServices
	ClientsLock.Unlock()

	// Notify Web UI
	if OnClientUpdate != nil {
		OnClientUpdate()
	}

	// 2. Open new ports
	for _, svc := range updatedServices {
		if svc.RemotePort != 0 {
			StartPublicListener(svc.RemotePort, clientID, svc.LocalIP, svc.LocalPort)
		}
	}
}

// AllocatePort finds an available port starting from config
func AllocatePort() (int, error) {
	ListenerLock.Lock()
	defer ListenerLock.Unlock()

	start := config.GlobalConfig.Server.PortStart
	if start <= 0 {
		start = 10000
	}

	for port := start; port < 65535; port++ {
		if _, exists := Listeners[port]; !exists {
			// Double check if actually available
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err == nil {
				ln.Close()
				return port, nil
			}
		}
	}
	return 0, fmt.Errorf("no available ports")
}

// StartPublicListener starts a listener on the server for a specific client target
func StartPublicListener(port int, clientID string, targetIP string, targetPort int) {
	ListenerLock.Lock()
	defer ListenerLock.Unlock()

	if _, exists := Listeners[port]; exists {
		return // Already listening
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Printf("[Core] Failed to listen on port %d: %v", port, err)
		return
	}
	Listeners[port] = ln
	log.Printf("[Core] Listening on public port %d for client %s -> %s:%d", port, clientID, targetIP, targetPort)

	go func() {
		for {
			userConn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleUserConnection(userConn, port, clientID, targetIP, targetPort)
		}
	}()
}

func handleUserConnection(userConn net.Conn, publicPort int, clientID string, targetIP string, targetPort int) {
	ClientsLock.RLock()
	client, exists := Clients[clientID]
	ClientsLock.RUnlock()

	if !exists {
		userConn.Close()
		return
	}

	// 1. Open Data Stream to Client
	stream, err := client.Session.Open()
	if err != nil {
		log.Printf("[Core] Failed to open stream to client %s: %v", clientID, err)
		userConn.Close()
		return
	}

	// 2. Send Handshake (Target IP:Port)
	// Format: "IP:Port\n"
	targetAddr := fmt.Sprintf("%s:%d", targetIP, targetPort)
	_, err = stream.Write([]byte(targetAddr + "\n"))
	if err != nil {
		log.Printf("[Core] Failed to send handshake: %v", err)
		stream.Close()
		userConn.Close()
		return
	}

	// 3. Pipe data
	go func() {
		io.Copy(userConn, stream)
		userConn.Close()
	}()
	go func() {
		io.Copy(stream, userConn)
		stream.Close()
	}()
}
