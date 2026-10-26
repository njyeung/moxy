# Prerequisites

Everything that has to be in place before the forwarding service can run. Steps are in order; the values below are examples, swap in your own.

| Thing | Value |
|---|---|
| VPS public IP | `203.0.113.10` |
| Home LAN | `192.168.1.0/24` (`vmbr0`, Proxmox host at `192.168.1.10`) |
| WireGuard subnet | `10.0.0.0/24` (Proxmox `10.0.0.1`, VPS `10.0.0.2`), UDP port `51820` on the VPS |
| Exposed VM subnet | `172.16.10.0/24` on `vmbr1`, gateway `172.16.10.1` (the Proxmox host) |

The three subnets must not overlap. Avoid `172.17.0.0/16`-`172.31.0.0/16` for the VM subnet since Docker uses those by default, and remember `127.x.x.x` is loopback, not a typo-safe neighbour of `172.x.x.x`.

## 1. WireGuard keys

```
apt install wireguard
cd /etc/wireguard
umask 077
wg genkey | tee privatekey | wg pubkey > publickey
cat publickey
```

Copy each machine's public key to the other; it goes in the other side's `[Peer]` section.

## 2. WireGuard setup on VPS

`/etc/wireguard/wg0.conf`:
```
[Interface]
Address = 10.0.0.2/24
ListenPort = 51820
MTU = 1280
PrivateKey = <VPS privatekey>

[Peer]
# proxmox
PublicKey = <Proxmox publickey>
AllowedIPs = 10.0.0.1/32, 172.16.10.0/24 # for VM subnet
```

- `AllowedIPs` includes the VM subnet so wg-quick adds a route for it. Never add the home LAN.
- If the VPS has a firewall, allow `51820/udp`.

```
systemctl enable --now wg-quick@wg0
```

## 3. WireGuard setup on Proxmox

`/etc/wireguard/wg0.conf`:
```
[Interface]
Address = 10.0.0.1/24
ListenPort = 51820
MTU = 1280
PrivateKey = <Proxmox privatekey>
Table = off # prevents proxmox from sending all traffic through VPS 

# marked packets (incoming from wg0) use table 100
# unmarked traffic still uses the main table thru vmbr0 masquerade  
PostUp = ip rule add fwmark 0x1 lookup 100 priority 100
PostUp = ip route add default dev wg0 table 100

PostUp = iptables -t mangle -A PREROUTING -i wg0 -d 172.16.10.0/24 -j CONNMARK --set-mark 0x1
PostUp = iptables -t mangle -A PREROUTING -i vmbr1 -j CONNMARK --restore-mark
PostUp   = sysctl -w net.ipv4.conf.wg0.rp_filter=2

PostDown = ip rule del fwmark 0x1 lookup 100 priority 100
PostDown = iptables -t mangle -D PREROUTING -i wg0 -d 172.16.10.0/24 -j CONNMARK --set-mark 0x1
PostDown = iptables -t mangle -D PREROUTING -i vmbr1 -j CONNMARK --restore-mark

[Peer]
# vps
PublicKey = <VPS publickey>
Endpoint = 203.0.113.10:51820
AllowedIPs = 0.0.0.0/0      # allow all src IPs to enter the VM  
PersistentKeepalive = 25
```

- When packets arrive from wg0, their dst IPs is already a VM IP (translated by the VPS), but their src IPs are from some random IP on the internet. The packet is routed to a VM, and under normal conditions, when the VM sends a reply, the dst IP is now the random IP on the internet. The Proxmox main routing table says to send that packet out to vmbr0 towards the private home router. Since we want these packets to be routed to the VPS, we need to mark their default route as wg0. 
- `PersistentKeepalive` is required because only Proxmox can start the tunnel from behind the home network. The keepalive holds the NAT mapping open and re-handshakes after a home internet outage.
- `MTU = 1280` on both sides because many home internet links drops full-size WireGuard packets.

```
sudo ufw status     # make sure no firewall 
systemctl enable --now wg-quick@wg0
```

## 4. VM bridge setup on Proxmox

In the GUI, navigate to Network and create a Linux Bridge
- Name `vmbr1`, IPv4/CIDR `172.16.10.1/24`
- Gateway and Bridge ports blank, Autostart on
- Click **Apply Configuration**

Then add the masquerade rule to the `vmbr1` block in `/etc/network/interfaces` so VMs can reach the internet through the home router:
```
auto vmbr1
iface vmbr1 inet static
    address 172.16.10.1/24
    bridge-ports none
    bridge-stp off
    bridge-fd 0
    post-up   iptables -t nat -A POSTROUTING -s 172.16.10.0/24 -o vmbr0 -j MASQUERADE
    post-down iptables -t nat -D POSTROUTING -s 172.16.10.0/24 -o vmbr0 -j MASQUERADE
```

## 5. Enable IPv4 forwarding on both machines

By default Linux drops any packet that isn't addressed to the machine itself. Both machines need to act as routers here:
- Proxmox passes traffic between `wg0` and `vmbr1`, and from `vmbr1` out `vmbr0` for the masquerade rule above.
- The VPS passes traffic arriving on its public interface into `wg0` toward the VM subnet, which is what the forwarding service relies on.

Enable forwarding:
```
echo 'net.ipv4.ip_forward=1' > /etc/sysctl.d/99-forward.conf
sysctl --system
```

## 6. SSH setup from Proxmox to the VPS

Moxy pushes forwarding rules to the VPS by piping nftables rulesets over SSH, through the WireGuard tunnel.

On the VPS, ensure you're root, or a user can run `nft` **without a password**:

On Proxmox, generate a key and record the VPS host key under the /etc/moxy/ directory:
```
mkdir -p /etc/moxy && chmod 700 /etc/moxy
ssh-keygen -t ed25519 -N "" -C moxy -f /etc/moxy/id_ed25519
ssh-keyscan -t ed25519 10.0.0.2 > /etc/moxy/known_hosts

# copy this
cat /etc/moxy/id_ed25519.pub
```

On the VPS, add the public key to `<root or your user>/.ssh/authorized_keys` as a single line:
```
# make sure you're in <root or your user>/.ssh/
nano authorized_keys
# paste your pub key
```

To test from Proxmox, you should be able to ssh into the VPS using the ssh key in /etc/moxy/:
```
ssh -i /etc/moxy/id_ed25519 <root or your user>@<10.0.0.2> "nft -h"
```

This should return the nft help output. If nft is not installed on your VPS, now would be a good time.

> If the VPS image already has a firewall with a forward chain that has `policy drop` (ufw, firewalld, or a default `inet filter` table), it will drop forwarded traffic even when the service's rules accept it. Check `nft list ruleset` and allow forwarding from the public interface into `wg0`.

## 7. DHCP on the VM bridge (optional)

We hand out IPv4 addresses to VMs in the range 172.16.10.2 to 172.16.10.99. Alternatively, manually assigning IP addresses also works too.

```
apt install dnsmasq
```

`/etc/dnsmasq.d/vmbr1.conf`:
```
interface=vmbr1
bind-dynamic

# no DNS server, just hand out public resolvers
port=0

dhcp-range=172.16.10.2,172.16.10.99,255.255.255.0,12h
dhcp-option=option:router,172.16.10.1
dhcp-option=option:dns-server,1.1.1.1,9.9.9.9
```

```
systemctl restart dnsmasq
journalctl -u dnsmasq -b --no-pager | grep 'DHCP, IP range'
```