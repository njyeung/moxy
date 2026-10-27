package main

import (
	"errors"
	"io/fs"
	"log"
	"main/nftables"
	"main/proxmox"
	"os"
	"time"

	"github.com/joho/godotenv"
)

var requiredEnv = []string{
	"PVE_TOKEN_SECRET",
	"VM_SUBNET",
	"VPS_WIREGUARD_IP",
	"VPS_USER",
	"VPS_WAN_IFACE",
}

// set at build time with -ldflags "-X main.version=v1.2.3"
var version = "dev"

func main() {
	log.Printf("moxy %s starting", version)

	// .env is for local development; under systemd the variables come from EnvironmentFile
	err := godotenv.Load()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatalf("could not load .env: %v", err)
	}

	for _, name := range requiredEnv {
		if os.Getenv(name) == "" {
			log.Fatalf("missing required env variable: %s", name)
		}
	}

	for {
		cfg, err := proxmox.Poll()
		if err != nil {
			log.Fatalf("could not poll: %v", err)
		}
		log.Printf("found %d exposed VMs", len(cfg.ExposedVms))

		err = nftables.Update(cfg)
		if err != nil {
			log.Fatalf("could not update nftables: %v", err)
		}

		time.Sleep(3 * time.Second)
	}
}
