package db

import (
	"io"
	"log"
	"net"
	"strconv"
	"sync"

	"golang.org/x/crypto/ssh"
)

type Tunnel struct {
	sshClient     *ssh.Client
	localListener net.Listener
	wg            sync.WaitGroup
}

func NewTunnel(sshUser, sshPassword, sshHost string, sshPort int, localAddr, remoteAddr string) (*Tunnel, error) {
	sshConfig := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{ssh.Password(sshPassword)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Ubah port int ke string dengan strconv.Itoa
    sshAddr := net.JoinHostPort(sshHost, strconv.Itoa(sshPort))

    client, err := ssh.Dial("tcp", sshAddr, sshConfig)
    if err != nil {
        return nil, err
    }

    listener, err := net.Listen("tcp", localAddr)
    if err != nil {
        client.Close()
        return nil, err
    }

    t := &Tunnel{
        sshClient:     client,
        localListener: listener,
    }

    t.wg.Add(1)
    go t.forward(remoteAddr)

    return t, nil
}

func (t *Tunnel) forward(remoteAddr string) {
	defer t.wg.Done()
	for {
		localConn, err := t.localListener.Accept()
		if err != nil {
			log.Println("Local accept error:", err)
			continue
		}

		remoteConn, err := t.sshClient.Dial("tcp", remoteAddr)
		if err != nil {
			log.Println("SSH dial to remote failed:", err)
			localConn.Close()
			continue
		}

		go func() {
			defer localConn.Close()
			defer remoteConn.Close()
			go io.Copy(remoteConn, localConn)
			io.Copy(localConn, remoteConn)
		}()
	}
}

func (t *Tunnel) Close() {
	t.localListener.Close()
	t.sshClient.Close()
	t.wg.Wait()
}
