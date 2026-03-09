//go:build darwin

package main

import (
	"fmt"
	"net"
	"syscall"
	"unsafe"

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

	var pid int32
	var cred *unix.Xucred
	var credErr error

	err = raw.Control(func(fd uintptr) {
		pidLen := uint32(unsafe.Sizeof(pid))
		_, _, errno := syscall.Syscall6(
			syscall.SYS_GETSOCKOPT,
			fd,
			uintptr(unix.SOL_LOCAL),
			unix.LOCAL_PEERPID,
			uintptr(unsafe.Pointer(&pid)),
			uintptr(unsafe.Pointer(&pidLen)),
			0,
		)
		if errno != 0 {
			credErr = fmt.Errorf("LOCAL_PEERPID: %w", errno)
			return
		}

		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	})
	if err != nil {
		return nil, err
	}
	if credErr != nil {
		return nil, credErr
	}

	pc := &PeerCred{
		PID: int(pid),
		UID: cred.Uid,
	}
	if len(cred.Groups) > 0 {
		pc.GID = cred.Groups[0]
	}
	return pc, nil
}
