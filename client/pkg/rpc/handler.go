package rpc

import (
	"common"
	"log"
)

type ClientRPC struct{}

func (r *ClientRPC) PushConfig(args *common.PushConfigArgs, reply *common.BaseReply) error {
	log.Printf("[RPC] Received PushConfig: %d services", len(args.Services))
	// Update local state (logic in Core?)
	// We can't import Core if Core imports us.
	// But Core imports common.
	// We can use a callback or event bus.
	// For simplicity, let's just log for now, or use a global in a third package.
	// Or, pass a handler to `ClientRPC` struct.
	
	// TODO: Update UI/State
	reply.Success = true
	return nil
}
