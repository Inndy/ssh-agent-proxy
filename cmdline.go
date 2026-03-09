package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func getProcessCmdline(pid int) string {
	out, err := exec.Command("ps", "-o", "args=", "-p", fmt.Sprint(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
