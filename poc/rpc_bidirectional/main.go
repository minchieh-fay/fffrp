package main

import (
	"log"
	"net"
	"net/rpc"
	"time"

	"github.com/hashicorp/yamux"
)

// Shared Args/Reply for RPC
type Args struct {
	Message string
}
type Reply struct {
	Response string
}

// ---------------- Server Logic ----------------
type ServerService struct{}

func (s *ServerService) Hello(args *Args, reply *Reply) error {
	log.Printf("[Server] Received: %s", args.Message)
	reply.Response = "Server says hello back to " + args.Message
	return nil
}

func startServer() {
	// 1. Listen TCP
	listener, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Server listening on :9999")

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		// 2. Setup Yamux Server
		session, err := yamux.Server(conn, nil)
		if err != nil {
			log.Println(err)
			continue
		}

		// 3. Accept the Control Stream (first stream)
		stream, err := session.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		log.Println("[Server] Accepted control stream")

		// --- Bidirectional Setup on SAME Stream ---
		// A. Serve RPC requests (Client -> Server)
		rpcServer := rpc.NewServer()
		rpcServer.Register(new(ServerService))
		go rpcServer.ServeConn(stream)

		// B. Create RPC Client (Server -> Client)
		// Problem: net/rpc/jsonrpc usually consumes the connection.
		// Standard net/rpc (gob) might also fight for reading.
		// Let's see if we can just create a client on the SAME stream.
		// NOTE: This is the experiment.
		go func() {
			// Wait a bit for client to be ready
			time.Sleep(2 * time.Second)
			client := rpc.NewClient(stream)

			// Try calling Client method
			var reply Reply
			err = client.Call("ClientService.Hello", &Args{Message: "Server"}, &reply)
			if err != nil {
				log.Printf("[Server] Call ClientService.Hello failed: %v", err)
			} else {
				log.Printf("[Server] Got reply from client: %s", reply.Response)
			}
		}()
	}
}

// ---------------- Client Logic ----------------
type ClientService struct{}

func (c *ClientService) Hello(args *Args, reply *Reply) error {
	log.Printf("[Client] Received: %s", args.Message)
	reply.Response = "Client says hello back to " + args.Message
	return nil
}

func startClient() {
	conn, err := net.Dial("tcp", "127.0.0.1:9999")
	if err != nil {
		log.Fatal(err)
	}

	// 1. Setup Yamux Client
	session, err := yamux.Client(conn, nil)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Open Control Stream
	stream, err := session.Open()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("[Client] Opened control stream")

	// --- Bidirectional Setup on SAME Stream ---

	// A. Register Client Service
	rpcServer := rpc.NewServer()
	rpcServer.Register(new(ClientService))

	// B. Create Client to call Server
	// We need a way to multiplex RPC messages if we use the SAME stream.
	// Standard net/rpc does NOT support concurrent ServeConn and NewClient on the exact same io.ReadWriteCloser
	// because both will try to Read() from it indefinitely.

	// Let's demonstrate the "Two Stream" approach for comparison,
	// OR try to see if the user's question "Can we do it?" fails.

	// Attempt 1: Just do it (This is expected to FAIL or BLOCK)
	go rpcServer.ServeConn(stream)

	client := rpc.NewClient(stream)
	var reply Reply
	err = client.Call("ServerService.Hello", &Args{Message: "Client"}, &reply)
	if err != nil {
		log.Printf("[Client] Call ServerService.Hello failed: %v", err)
	} else {
		log.Printf("[Client] Got reply from server: %s", reply.Response)
	}

	select {}
}

func main() {
	// This POC is to verify if ONE stream can support TWO-WAY RPC.
	// Answer prediction: NO.
	// Because `rpc.ServeConn` runs a loop reading from the stream.
	// `rpc.NewClient` also needs to read responses from the stream.
	// They will race for bytes.

	go startServer()
	time.Sleep(1 * time.Second)
	startClient()
}
