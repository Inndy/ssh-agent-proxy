//go:build linux

package main

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

type PeerCred struct {
	PID int
	UID uint32
	GID uint32
}

func getPeerCred(conn net.Conn) (*PeerCred, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("not a unix connection")
	}

	raw, err := uc.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("syscall conn: %w", err)
	}

	var cred *unix.Ucred
	var credErr error

	err = raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return nil, err
	}
	if credErr != nil {
		return nil, credErr
	}

	return &PeerCred{
		PID: int(cred.Pid),
		UID: cred.Uid,
		GID: cred.Gid,
	}, nil
}
