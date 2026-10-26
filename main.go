package main

import (
	"log"
	"main/nftables"
	"main/proxmox"
	"os"

	"github.com/joho/godotenv"
)

var requiredEnv = []string{
	"PVE_TOKEN_SECRET",
	"VM_SUBNET",
	"VPS_WIREGUARD_IP",
	"VPS_USER",
	"VPS_WAN_IFACE",
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("could not load .env")
	}

	for _, name := range requiredEnv {
		if os.Getenv(name) == "" {
			log.Fatalf("missing required env variable: %s", name)
		}
	}

	cfg, err := proxmox.Poll()
	if err != nil {
		log.Fatalf("could not poll: %v", err)
	}
	log.Printf("found %d exposed VMs", len(cfg.ExposedVms))

	err = nftables.Update(cfg)
	if err != nil {
		log.Fatalf("could not update nftables: %v", err)
	}
}
