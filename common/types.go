package common

// Version is the protocol version
const Version = "1.0.0"

// TargetService represents a service to be exposed
type TargetService struct {
	ID         string `json:"id"`
	LocalIP    string `json:"local_ip"`
	LocalPort  int    `json:"local_port"`
	RemotePort int    `json:"remote_port"` // The public port on server
	Remark     string `json:"remark"`
}

// ---------------- RPC Args & Reply ----------------

// BaseArgs for simple requests
type BaseArgs struct {
	ClientID string
}

// BaseReply for simple responses
type BaseReply struct {
	Success bool
	Message string
}

// HandshakeArgs for initial connection
type HandshakeArgs struct {
	ClientID    string
	Version     string
	Name        string
	Phone       string
	ProjectName string
	Remark      string
}

// SyncConfigArgs for syncing target services
type SyncConfigArgs struct {
	ClientID string
	Services []TargetService
}

// PushConfigArgs for Server -> Client sync
type PushConfigArgs struct {
	Services []TargetService
}

// ---------------- Constants ----------------

// Stream Types
const (
	StreamTypeControl = 0 // Handled by logic
	StreamTypeData    = 1 // Handled by logic
)
