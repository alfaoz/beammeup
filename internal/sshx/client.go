package sshx

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Target struct {
	Host     string
	Port     int
	User     string
	Password string
}

type Client struct {
	sshClient *ssh.Client
}

func Connect(t Target) (*Client, error) {
	if t.Port == 0 {
		t.Port = 22
	}
	cfg := &ssh.ClientConfig{
		User:            t.User,
		Auth:            []ssh.AuthMethod{ssh.Password(t.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}
	addr := net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port))
	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, err
	}
	return &Client{sshClient: c}, nil
}

func (c *Client) Close() error {
	if c == nil || c.sshClient == nil {
		return nil
	}
	return c.sshClient.Close()
}

func (c *Client) RunCombined(command string) (string, error) {
	session, err := c.sshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.CombinedOutput(command)
	return string(out), err
}

func (c *Client) Upload(content []byte, remotePath string, mode os.FileMode) error {
	sftpClient, err := sftp.NewClient(c.sshClient)
	if err != nil {
		return err
	}
	defer sftpClient.Close()

	f, err := sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
