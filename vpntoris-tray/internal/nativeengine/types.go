package nativeengine

import "time"

type MutationKind string

const (
	MutationInterface MutationKind = "interface"
	MutationRoute     MutationKind = "route"
	MutationDNS       MutationKind = "dns"
	MutationProcess   MutationKind = "process"
)

type MutationState string

const (
	MutationPending MutationState = "pending"
	MutationApplied MutationState = "applied"
	MutationUndone  MutationState = "undone"
	MutationFailed  MutationState = "failed"
)

type Mutation struct {
	ID        string            `json:"id"`
	Kind      MutationKind      `json:"kind"`
	State     MutationState     `json:"state"`
	Resource  string            `json:"resource"`
	Interface string            `json:"interface,omitempty"`
	CIDR      string            `json:"cidr,omitempty"`
	Domain    string            `json:"domain,omitempty"`
	PID       int               `json:"pid,omitempty"`
	Values    map[string]string `json:"values,omitempty"`
	Error     string            `json:"error,omitempty"`
}
type TransactionState string

const (
	TransactionPreparing   TransactionState = "preparing"
	TransactionActive      TransactionState = "active"
	TransactionRollingBack TransactionState = "rolling_back"
	TransactionFailed      TransactionState = "failed"
)

type Transaction struct {
	Version   int              `json:"version"`
	ID        string           `json:"id"`
	Owner     string           `json:"owner"`
	Profile   string           `json:"profile"`
	Platform  string           `json:"platform"`
	State     TransactionState `json:"state"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Mutations []Mutation       `json:"mutations"`
}
type Plan struct {
	Profile   string
	Mutations []Mutation
}
