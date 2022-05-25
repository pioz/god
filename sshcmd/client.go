// Package sshcmd allows running commands on a remote host via ssh.
package sshcmd

import (
	"bytes"
	"io/ioutil"
	"net"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// Client is a wrapped around ssh.Client to run commands on a remote host via
// ssh using a simple and easy to use APIs.
type Client struct {
	SshClient *ssh.Client
	Username  string
	Host      string
	Port      string

	privateKey []byte
}

// MakeClient returns an initialized Client.
func MakeClient(username, host, port, privateKeyPath string) (*Client, error) {
	if port == "" {
		port = "22"
	}
	client := &Client{
		Username: username,
		Host:     host,
		Port:     port,
	}

	bytes, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}
	client.privateKey = bytes
	return client, nil
}

// Connect connects the client to the remote host. After connection, the client
// is ready to run a command on the remote host.
func (c *Client) Connect() error {
	key, err := ssh.ParsePrivateKey(c.privateKey)
	if err != nil {
		return err
	}
	// Authentication
	config := &ssh.ClientConfig{
		User: c.Username,
		// https://github.com/golang/go/issues/19767
		// as clientConfig is non-permissive by default
		// you can set ssh.InsercureIgnoreHostKey to allow any host
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
		// //alternatively, you could use a password
		// Auth: []ssh.AuthMethod{ssh.Password("PASSWORD")},
	}
	// Connect
	client, err := ssh.Dial("tcp", net.JoinHostPort(c.Host, c.Port), config)
	if err != nil {
		return err
	}
	c.SshClient = client
	return nil
}

// Exec runs a command on the remote host. Returns the output of the command and
// the error if occurred.
func (c *Client) Exec(cmd string) (string, error) {
	if c.SshClient == nil {
		return "", errors.New("client is not connected")
	}
	// Create a session. It is one session per command.
	session, err := c.SshClient.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	err = session.Run(cmd)
	if err != nil {
		return stderr.String(), err
	}
	return stdout.String(), nil
}
