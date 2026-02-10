package rpc

import (
	"common"
	"errors"
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
}

type ServerRPC struct{} // Deprecated

func (r *ServerRPCContext) Handshake(args *common.HandshakeArgs, reply *common.BaseReply) error {
	log.Printf("[RPC] Handshake from %s (v%s) | User: %s, Phone: %s, Project: %s, Remark: %s",
		args.ClientID, args.Version, args.Name, args.Phone, args.ProjectName, args.Remark)
	if args.Version != common.Version {
		return errors.New("version mismatch")
	}

	// Register the client in Core
	core.AddClient(args.ClientID, r.Session, r.RPCClient, args.Name, args.Phone, args.ProjectName, args.Remark)

	reply.Success = true
	reply.Message = "Welcome"
	return nil
}

func (r *ServerRPCContext) SyncConfig(args *common.SyncConfigArgs, reply *common.BaseReply) error {
	log.Printf("[RPC] SyncConfig from %s: %d services", args.ClientID, len(args.Services))
	core.UpdateServices(args.ClientID, args.Services)
	reply.Success = true
	return nil
}

func (r *ServerRPCContext) Heartbeat(args *common.BaseArgs, reply *common.BaseReply) error {
	// log.Printf("[RPC] Heartbeat from %s", args.ClientID) // verbose
	reply.Success = true
	return nil
}
