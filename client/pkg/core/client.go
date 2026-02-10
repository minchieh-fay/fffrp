package core

import (
	"bufio"
	"common"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"strings"
	"sync"

	"github.com/hashicorp/yamux"
)

// AppState holds the global state of the client application
type AppState struct {
	ClientID    string
	Session     *yamux.Session
	RPCClient   *rpc.Client
	Services    []common.TargetService
	IsConnected bool
	Lock        sync.RWMutex

	// User Info
	Name        string
	Phone       string
	ProjectName string
	Remark      string
}

var State = &AppState{
	// ClientID will be loaded from config or generated
}

// OnUpdate is called when state changes
var OnUpdate func()

// ConnectServer establishes connection to the server
func ConnectServer(addr string) error {
	State.Lock.Lock()
	defer State.Lock.Unlock()

	if State.IsConnected {
		return nil
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}

	// 1. Setup Yamux Client
	session, err := yamux.Client(conn, nil)
	if err != nil {
		conn.Close()
		return err
	}
	State.Session = session

	// 2. Open Control Stream (Client -> Server)
	controlStream, err := session.Open()
	if err != nil {
		session.Close()
		return err
	}

	// 3. Setup RPC Client
	rpcClient := rpc.NewClient(controlStream)
	State.RPCClient = rpcClient

	// 4. Handshake
	args := &common.HandshakeArgs{
		ClientID:    State.ClientID,
		Version:     common.Version,
		Name:        State.Name,
		Phone:       State.Phone,
		ProjectName: State.ProjectName,
		Remark:      State.Remark,
	}
	var reply common.BaseReply
	err = rpcClient.Call("ServerRPCContext.Handshake", args, &reply)
	if err != nil {
		session.Close()
		return fmt.Errorf("handshake failed: %v", err)
	}
	if !reply.Success {
		session.Close()
		return fmt.Errorf("handshake rejected: %s", reply.Message)
	}
	log.Println("[Core] Handshake success:", reply.Message)

	// 5. Accept Reverse Control Stream (Server -> Client)
	// We need to do this asynchronously because `ConnectServer` might block the UI
	// But we should ensure it's ready.
	go func() {
		// Expect Server to open a stream immediately after handshake (or parallel)
		// Note: In our Server implementation, Server opens it right after Accept.
		// So it should be waiting in queue.
		revStream, err := session.Accept()
		if err != nil {
			log.Println("[Core] Failed to accept reverse stream:", err)
			return
		}

		// Serve RPC for Server -> Client calls
		// We need to register the RPC handler
		// Circular dependency: RPC handler needs access to Core?
		// We will inject the handler logic later or use a global.
		// For now, let's assume `rpc.DefaultServer` or new server.
		srv := rpc.NewServer()
		// We need to register "ClientRPC"
		// How to do this cleanly?
		// We can expose a Register function in `rpc` package.
		// For now, we will handle this in `main` or `Start`.
		// But `ConnectServer` is called from `Start`.

		// Let's defer the actual `ServeConn` to a callback or do it here if we can import `client/pkg/rpc`.
		// We can't import `client/pkg/rpc` if `client/pkg/rpc` imports `core` (cycle).
		// Solution: Define the interface here or put RPC handler in a separate package that imports core,
		// and core does NOT import RPC handler.
		// But `ConnectServer` is in `core`.

		// Let's use a callback hook.
		if OnReverseRPC != nil {
			OnReverseRPC(srv, revStream)
		}
	}()

	State.IsConnected = true

	// 6. Start Data Loop (Accept streams from Server for data forwarding)
	go acceptDataStreams(session)

	return nil
}

var OnReverseRPC func(*rpc.Server, net.Conn)

func acceptDataStreams(session *yamux.Session) {
	for {
		stream, err := session.Accept()
		if err != nil {
			if err != io.EOF {
				log.Println("[Core] Session accept error:", err)
			}
			State.Lock.Lock()
			State.IsConnected = false
			State.Lock.Unlock()
			return
		}
		// Check if it's a data stream (not the control one, which we already handled?)
		// Actually, Yamux doesn't distinguish "types" in Accept().
		// We already Accepted the FIRST stream as Reverse Control.
		// So subsequent streams MUST be Data Streams (Server requesting connection).

		go handleDataStream(stream)
	}
}

func handleDataStream(stream net.Conn) {
	// 1. Read Handshake (Target IP:Port)
	// Protocol: "IP:Port\n"
	reader := bufio.NewReader(stream)
	targetAddr, err := reader.ReadString('\n')
	if err != nil {
		log.Println("[Core] Failed to read handshake:", err)
		stream.Close()
		return
	}
	targetAddr = strings.TrimSpace(targetAddr)
	log.Printf("[Core] New data stream request for: %s", targetAddr)

	// 2. Connect to Local Target
	localConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		log.Printf("[Core] Failed to dial local target %s: %v", targetAddr, err)
		stream.Close()
		return
	}

	// 3. Pipe
	go func() {
		defer localConn.Close()
		defer stream.Close()
		// We MUST NOT use 'stream' directly if 'reader' has buffered data!
		// 'reader' is a bufio.Reader wrapping 'stream'. It may have read more bytes than just the newline.
		// We need to write any buffered bytes to localConn first, then copy the rest from stream.

		if reader.Buffered() > 0 {
			bufferedBytes, _ := reader.Peek(reader.Buffered())
			localConn.Write(bufferedBytes)
		}

		// Now copy the rest directly from stream (assuming bufio doesn't hold anything else invisible)
		// actually, mixing bufio and direct read is dangerous if we don't discard the buffered part from the reader?
		// No, Peek just looks. We need to consume it or just Write it.
		// But wait, if we use io.Copy(localConn, stream), it reads from 'stream'.
		// 'stream' cursor is ahead of 'reader' cursor if reader buffered data.
		// So we are good?
		// NO! 'stream' cursor is AHEAD. 'reader' has data in memory that 'stream' already delivered.
		// So we must write reader.Buffered() to localConn.
		// AND we must ensure subsequent reads come from 'stream'.

		io.Copy(localConn, stream)
	}()
	go func() {
		defer stream.Close()
		defer localConn.Close()
		io.Copy(stream, localConn)
	}()
}
