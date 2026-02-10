package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"server/config"
	"server/pkg/core"
	rpcHandler "server/pkg/rpc"
	"server/pkg/web"

	"github.com/hashicorp/yamux"
)

func main() {
	// 1. Load Config
	config.Load()

	// 2. Start Web Server
	web.Start()

	// 3. Start TCP Listener for Clients
	port := config.GlobalConfig.Server.TcpPort
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %d: %v", port, err)
	}
	log.Printf("Server listening on :%d", port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}
		go handleClient(conn)
	}
}

func handleClient(conn net.Conn) {
	// 1. Setup Yamux
	session, err := yamux.Server(conn, nil)
	if err != nil {
		log.Println("Yamux server error:", err)
		return
	}

	// 2. Open Reverse Control Stream (Stream 1 for us, but maybe not 1 ID)
	// Strategy:
	// A. Client connects.
	// B. Client Opens Stream (Control for C->S). Server Accepts.
	// C. Server Opens Stream (Control for S->C). Client Accepts.

	// Wait for Client to Open Control Stream
	controlStream, err := session.Accept()
	if err != nil {
		log.Println("Failed to accept control stream:", err)
		return
	}

	// Open Reverse Control Stream
	reverseStream, err := session.Open()
	if err != nil {
		log.Println("Failed to open reverse control stream:", err)
		return
	}
	rpcClient := rpc.NewClient(reverseStream)

	handler := &rpcHandler.ServerRPCContext{
		Session:   session,
		RPCClient: rpcClient,
		Conn:      conn,
	}

	// Re-register for this connection specifically?
	// `rpc.NewServer()` creates a new server instance.
	// `server.RegisterName("ServerRPC", handler)`

	singleServer := rpc.NewServer()
	singleServer.RegisterName("ServerRPCContext", handler) // Match Client's call

	// Wait for handshake? No, just serve.
	// The handler's Handshake method will have access to `handler.Session`.
	// ServeConn blocks until client disconnects
	singleServer.ServeConn(controlStream)

	// Clean up on disconnect
	log.Println("Client disconnected (control stream closed)")
	// We need to know WHICH client ID to remove.
	// But at this point (main.go), we don't know the ClientID yet?
	// The ClientID is inside `handler` BUT `handler` doesn't store it until Handshake.
	// However, `core.AddClient` stores it.

	// We can add a hook or callback in handler, OR better:
	// Use the Session to look up the client in core?
	// `core.RemoveClientBySession(session)`?
	core.RemoveClientBySession(session)
}
