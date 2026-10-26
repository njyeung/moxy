package proxmox

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"main/config"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPveURL     = "https://127.0.0.1:8006"
	defaultPveTokenID = "moxy@pve!poll"
	vmResourcePath    = "/api2/json/cluster/resources?type=vm"
)

func Poll() (*config.Config, error) {
	prefix, err := netip.ParsePrefix(os.Getenv("VM_SUBNET"))
	if err != nil {
		return nil, fmt.Errorf("VM_SUBNET must be a CIDR like 172.16.10.0/24: %w", err)
	}
	prefix = prefix.Masked()

	endpoint := strings.TrimRight(config.EnvOr("PVE_URL", defaultPveURL), "/") + vmResourcePath
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	tokenID := config.EnvOr("PVE_TOKEN_ID", defaultPveTokenID)
	req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", tokenID, os.Getenv("PVE_TOKEN_SECRET")))

	// proxmox uses a self-signed cert by default, fine for localhost
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("statusCode not 2xx: %d: %s", resp.StatusCode, body)
	}

	var resource resource
	err = json.Unmarshal(body, &resource)
	if err != nil {
		return nil, err
	}

	// fwd-<tcp/udp>-<vps port>-<proxmox port>
	// ip-<vm IP>

	var vms []config.ExposedVM

	for _, o := range resource.Data {

		vm := config.ExposedVM{}

		vm.Name = o.Name
		vm.VmId = o.VMID
		tags := strings.Split(o.Tags, ";")

		for _, tag := range tags {
			args := strings.Split(tag, "-")

			switch args[0] {
			case "fwd":
				if len(args) < 4 {
					log.Printf("VM %d (%s): ignoring tag %q: want fwd-<tcp/udp>-<vps port>-<vm port>", o.VMID, o.Name, tag)
					continue
				}

				b := &config.Bind{}

				switch args[1] {
				case "udp":
					b.Protocol = config.UDP
				case "tcp":
					b.Protocol = config.TCP
				default:
					log.Printf("VM %d (%s): ignoring tag %q: protocol must be tcp or udp", o.VMID, o.Name, tag)
					continue
				}

				vpsPort, err := strconv.ParseUint(args[2], 10, 16)
				if err != nil || 0 == vpsPort {
					log.Printf("VM %d (%s): ignoring tag %q: bad vps port", o.VMID, o.Name, tag)
					continue
				}
				b.VpsPort = uint16(vpsPort)

				vmPort, err := strconv.ParseUint(args[3], 10, 16)
				if err != nil || 0 == vmPort {
					log.Printf("VM %d (%s): ignoring tag %q: bad vm port", o.VMID, o.Name, tag)
					continue
				}
				b.VmPort = uint16(vmPort)

				vm.Binds = append(vm.Binds, b)

			case "ip":
				if len(args) < 2 {
					log.Printf("VM %d (%s): ignoring tag %q: want ip-<vm ip>", o.VMID, o.Name, tag)
					continue
				}
				addr, err := netip.ParseAddr(args[1])
				if err != nil {
					log.Printf("VM %d (%s): ignoring tag %q: bad ip", o.VMID, o.Name, tag)
					continue
				}

				if !prefix.Contains(addr) {
					log.Printf("VM %d (%s): ignoring tag %q: ip not in VM_SUBNET %s", o.VMID, o.Name, tag, prefix)
					continue
				}

				vm.IP = addr

			default:
				// tag is irrelevant to us
			}
		}

		// ip field is required
		if !vm.IP.IsValid() || vm.Name == "" || vm.VmId <= 0 {
			if len(vm.Binds) > 0 {
				log.Printf("VM %d (%s): skipping, has fwd tags but no valid ip tag", o.VMID, o.Name)
			}
			continue
		}

		// fill in the nice-to-have VMAddrPort field now that we have the VM's IP
		for _, bind := range vm.Binds {
			bind.VmAddrPort = netip.AddrPortFrom(vm.IP, bind.VmPort)
		}
		vms = append(vms, vm)

	}

	return &config.Config{ExposedVms: vms, CreatedAt: time.Now()}, nil
}
