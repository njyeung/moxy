package proxmox

// INTERNAL

type object struct {
	Name string `json:"name"`
	VMID int    `json:"vmid"`
	Tags string `json:"tags"`
}
type resource struct {
	Data []object `json:"data"`
}
