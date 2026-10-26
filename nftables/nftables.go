package nftables

import (
	"fmt"
	"main/config"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	defaultKeyPath        = "/etc/moxy/id_ed25519"
	defaultKnownHostsPath = "/etc/moxy/known_hosts"
	defaultSSHPort        = "22"
	defaultWgIface        = "wg0"
)

func Update(cfg *config.Config) error {
	user := os.Getenv("VPS_USER")
	wanIface := os.Getenv("VPS_WAN_IFACE")
	vpsIP := os.Getenv("VPS_WIREGUARD_IP")
	sshPort := config.EnvOr("VPS_SSH_PORT", defaultSSHPort)
	wgIface := config.EnvOr("VPS_WG_IFACE", defaultWgIface)
	keyPath := config.EnvOr("MOXY_SSH_KEY", defaultKeyPath)
	knownHostsPath := config.EnvOr("MOXY_KNOWN_HOSTS", defaultKnownHostsPath)

	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read ssh key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return fmt.Errorf("parse ssh key: %w", err)
	}
	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return fmt.Errorf("read known_hosts: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		// known_hosts only has the ed25519 key (ssh-keyscan -t ed25519)
		// don't let the VPS offer ECDSA/RSA, which knownhosts would report as a key mismatch
		HostKeyAlgorithms: []string{ssh.KeyAlgoED25519},
		Timeout:           10 * time.Second,
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(vpsIP, sshPort), sshConfig)
	if err != nil {
		return err
	}
	defer client.Close()

	// non-root users need passwordless sudo for nft
	nftCmd := "nft -f -"
	if user != "root" {
		nftCmd = "sudo -n " + nftCmd
	}

	// check VPS for its current nft hash
	currHash, err := config.Hash(*cfg)
	if err != nil {
		return err
	}

	// a missing table or missing hash just means we push
	out, _ := runNft(client, nftCmd, "list table ip moxy\n")
	prevHash := config.ReadHash(out)

	// push updated to VPS if vm tags changed
	if currHash == prevHash {
		return nil
	}
	ruleset, err := config.NftString(*cfg, wanIface, wgIface)
	if err != nil {
		return err
	}
	out, err = runNft(client, nftCmd, ruleset)
	if err != nil {
		return err
	}

	return nil
}

func runNft(client *ssh.Client, nftCmd, input string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	session.Stdin = strings.NewReader(input)
	out, err := session.CombinedOutput(nftCmd)
	return string(out), err
}
