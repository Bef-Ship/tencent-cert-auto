package deploy

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func dialSSH(t Target) (*ssh.Client, error) {
	key, err := os.ReadFile(t.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("read private_key: %w", err)
	}

	var signer ssh.Signer
	if t.Passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(t.Passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(key)
	}
	if err != nil {
		return nil, fmt.Errorf("parse private_key: %w", err)
	}

	hostKeyCallback := ssh.InsecureIgnoreHostKey() //nolint:gosec
	if t.KnownHosts != "" {
		cb, err := knownhosts.New(t.KnownHosts)
		if err != nil {
			return nil, fmt.Errorf("known_hosts: %w", err)
		}
		hostKeyCallback = cb
	} else {
		slog.Warn("ssh known_hosts not set, host key verification disabled",
			"name", t.Name, "host", t.Host)
	}

	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", t.Host, t.Port)
	return ssh.Dial("tcp", addr, cfg)
}
