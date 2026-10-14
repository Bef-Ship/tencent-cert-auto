package deploy

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func deployLocal(t Target, certPEM, keyPEM []byte) error {
	if err := writeLocalFile(t.CertPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := writeLocalFile(t.KeyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return runLocal(t.ReloadCmd)
}

func writeLocalFile(p string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, mode)
}

func runLocal(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		slog.Info("local reload_cmd empty, skip reload")
		return nil
	}
	c := exec.Command("sh", "-c", cmd)
	out, err := c.CombinedOutput()
	if len(out) > 0 {
		slog.Info("local reload output", "output", strings.TrimSpace(string(out)))
	}
	if err != nil {
		return fmt.Errorf("reload %q: %w", cmd, err)
	}
	return nil
}

func deploySSH(t Target, certPEM, keyPEM []byte) error {
	client, err := dialSSH(t)
	if err != nil {
		return err
	}
	defer client.Close()

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("sftp: %w", err)
	}
	defer sftpClient.Close()

	if err := putRemote(sftpClient, t.CertPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("upload cert: %w", err)
	}
	if err := putRemote(sftpClient, t.KeyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("upload key: %w", err)
	}
	return runRemote(client, t.ReloadCmd)
}

func putRemote(c *sftp.Client, remotePath string, data []byte, mode os.FileMode) error {
	dir := path.Dir(remotePath)
	if err := c.MkdirAll(dir); err != nil {
		return err
	}
	f, err := c.OpenFile(remotePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Chmod(mode)
}

func runRemote(client *ssh.Client, cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		slog.Info("remote reload_cmd empty, skip reload")
		return nil
	}
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("reload %q: %w (%s)", cmd, err, strings.TrimSpace(buf.String()))
	}
	if buf.Len() > 0 {
		slog.Info("remote reload output", "output", strings.TrimSpace(buf.String()))
	}
	return nil
}
