//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

var unitDir = expandTilde("~/.config/systemd/user")
var unitPath = filepath.Join(unitDir, "ssh-agent-proxy.service")

var unitTemplate = template.Must(template.New("unit").Parse(`[Unit]
Description=SSH Agent Proxy
Documentation=https://go.inndy.tw/ssh-agent-proxy

[Service]
Type=simple
ExecStart={{.BinaryPath}} service run{{if .ConfigPath}} -config {{.ConfigPath}}{{end}}
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`))

type unitData struct {
	BinaryPath string
	ConfigPath string
}

func handleServiceCommand(args []string) {
	if len(args) == 0 {
		printServiceUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "run":
		runServer(args[1:])
	case "install":
		serviceInstall(args[1:])
	case "uninstall":
		serviceUninstall()
	case "start":
		serviceStart()
	case "stop":
		serviceStop()
	default:
		fmt.Fprintf(os.Stderr, "unknown service command: %s\n", args[0])
		printServiceUsage()
		os.Exit(1)
	}
}

func printServiceUsage() {
	fmt.Fprintf(os.Stderr, "Usage: ssh-agent-proxy service <command>\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  run [flags]              Run the proxy agent (foreground)\n")
	fmt.Fprintf(os.Stderr, "  install [-config path]   Install and enable systemd user service\n")
	fmt.Fprintf(os.Stderr, "  uninstall                Disable and remove systemd user service\n")
	fmt.Fprintf(os.Stderr, "  start                    Start the service\n")
	fmt.Fprintf(os.Stderr, "  stop                     Stop the service\n")
}

func serviceInstall(args []string) {
	var configPath string
	for i := 0; i < len(args); i++ {
		if args[i] == "-config" && i+1 < len(args) {
			configPath = args[i+1]
			i++
		} else {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	if _, err := os.Stat(unitPath); err == nil {
		fmt.Fprintf(os.Stderr, "error: service already installed at %s\n", unitPath)
		fmt.Fprintf(os.Stderr, "run 'ssh-agent-proxy service uninstall' first\n")
		os.Exit(1)
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not determine binary path: %v\n", err)
		os.Exit(1)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not resolve binary path: %v\n", err)
		os.Exit(1)
	}

	data := unitData{
		BinaryPath: exe,
	}

	if configPath != "" {
		abs, err := filepath.Abs(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not resolve config path: %v\n", err)
			os.Exit(1)
		}
		data.ConfigPath = abs
	}

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not create systemd user directory: %v\n", err)
		os.Exit(1)
	}

	f, err := os.Create(unitPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not write unit file: %v\n", err)
		os.Exit(1)
	}
	if err := unitTemplate.Execute(f, data); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "error: could not render unit file: %v\n", err)
		os.Exit(1)
	}
	f.Close()

	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not reload systemd: %v\n", err)
	}

	if err := exec.Command("systemctl", "--user", "enable", "ssh-agent-proxy.service").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not enable service: %v\n", err)
	}

	if err := exec.Command("systemctl", "--user", "start", "ssh-agent-proxy.service").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not start service: %v\n", err)
		fmt.Fprintf(os.Stderr, "unit written to %s — start it manually with:\n", unitPath)
		fmt.Fprintf(os.Stderr, "  systemctl --user start ssh-agent-proxy.service\n")
		return
	}

	fmt.Printf("service installed, enabled, and started\n")
	fmt.Printf("unit: %s\n", unitPath)

	cfg, err := loadConfig(func() string {
		if configPath != "" {
			return data.ConfigPath
		}
		return defaultConfigPath()
	}())
	if err == nil {
		fmt.Printf("socket: %s\n", cfg.Listen)
	}
}

func serviceUninstall() {
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: service not installed (no unit at %s)\n", unitPath)
		os.Exit(1)
	}

	_ = exec.Command("systemctl", "--user", "stop", "ssh-agent-proxy.service").Run()
	_ = exec.Command("systemctl", "--user", "disable", "ssh-agent-proxy.service").Run()

	if err := os.Remove(unitPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not remove unit file: %v\n", err)
		os.Exit(1)
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()

	fmt.Printf("service uninstalled\n")
}

func serviceStart() {
	if err := exec.Command("systemctl", "--user", "start", "ssh-agent-proxy.service").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not start service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service started\n")
}

func serviceStop() {
	if err := exec.Command("systemctl", "--user", "stop", "ssh-agent-proxy.service").Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not stop service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service stopped\n")
}
