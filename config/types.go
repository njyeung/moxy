package config

import (
	"net/netip"
	"time"
)

type Protocol string

const (
	TCP Protocol = "tcp"
	UDP Protocol = "udp"
)

// Corresponds directly to DNAT rules
type Bind struct {
	Protocol   Protocol       `json:"protocol"`
	VpsPort    uint16         `json:"vps_port"`
	VmPort     uint16         `json:"vm_port"`
	VmAddrPort netip.AddrPort `json:"vm_addr_port"`
}

// Extra info about each VM as well as its Binds
type ExposedVM struct {
	Name string     `json:"name"`
	VmId int        `json:"vm_id"`
	IP   netip.Addr `json:"ip"`

	Binds []*Bind `json:"binds"`
}

type Config struct {
	ExposedVms []ExposedVM `json:"exposed_vms"`
	CreatedAt  time.Time   `json:"created_at"`
}
