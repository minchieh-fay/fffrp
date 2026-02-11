package rpc

import (
	"common"
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"server/pkg/core"

	"github.com/hashicorp/yamux"
)

// ServerRPCContext holds context for a specific client connection
type ServerRPCContext struct {
	Session   *yamux.Session
	RPCClient *rpc.Client
	Conn      net.Conn
	ClientID  string // Stored after handshake
}

type ServerRPC struct{} // Deprecated

func (r *ServerRPCContext) Handshake(args *common.HandshakeArgs, reply *common.BaseReply) error {
	log.Printf("[RPC] Handshake from %s (v%s) | User: %s, Phone: %s, Project: %s, Remark: %s",
		args.ClientID, args.Version, args.Name, args.Phone, args.ProjectName, args.Remark)
	if args.Version != common.Version {
		return errors.New("version mismatch")
	}

	// Register the client in Core
	// Append IP to ID to allow multiple clients with same config to coexist
	host, _, _ := net.SplitHostPort(r.Conn.RemoteAddr().String())
	finalID := fmt.Sprintf("%s@%s", args.ClientID, host)

	log.Printf("[RPC] Registering client as: %s", finalID)
	r.ClientID = finalID // Store for later use

	core.AddClient(finalID, r.Session, r.RPCClient, args.Name, args.Phone, args.ProjectName, args.Remark)

	reply.Success = true
	reply.Message = "Welcome"
	return nil
}

func (r *ServerRPCContext) SyncConfig(args *common.SyncConfigArgs, reply *common.BaseReply) error {
	// Use the ID we stored, ignore what client sent (because we modified it)
	targetID := r.ClientID
	if targetID == "" {
		// Fallback if Handshake wasn't called (should not happen)
		targetID = args.ClientID
	}

	log.Printf("[RPC] SyncConfig from %s (mapped from %s): %d services", targetID, args.ClientID, len(args.Services))
	core.UpdateServices(targetID, args.Services)
	reply.Success = true
	return nil
}

func (r *ServerRPCContext) Heartbeat(args *common.BaseArgs, reply *common.BaseReply) error {
	// log.Printf("[RPC] Heartbeat from %s", args.ClientID) // verbose
	reply.Success = true
	return nil
}
