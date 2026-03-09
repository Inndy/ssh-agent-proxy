//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const serviceLabel = "tw.inndy.ssh-agent-proxy"

var plistPath = expandTilde("~/Library/LaunchAgents/tw.inndy.ssh-agent-proxy.plist")

var plistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>tw.inndy.ssh-agent-proxy</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.BinaryPath}}</string>
		<string>service</string>
		<string>run</string>
{{- if .ConfigPath}}
		<string>-config</string>
		<string>{{.ConfigPath}}</string>
{{- end}}
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>{{.LogPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{.LogPath}}</string>
</dict>
</plist>
`))

type plistData struct {
	BinaryPath string
	ConfigPath string
	LogPath    string
}

func guiDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func serviceTarget() string {
	return fmt.Sprintf("%s/%s", guiDomain(), serviceLabel)
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
	fmt.Fprintf(os.Stderr, "  install [-config path]   Install and load launchd service\n")
	fmt.Fprintf(os.Stderr, "  uninstall                Unload and remove launchd service\n")
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

	if _, err := os.Stat(plistPath); err == nil {
		fmt.Fprintf(os.Stderr, "error: service already installed at %s\n", plistPath)
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

	data := plistData{
		BinaryPath: exe,
		LogPath:    expandTilde("~/Library/Logs/ssh-agent-proxy.log"),
	}

	if configPath != "" {
		abs, err := filepath.Abs(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: could not resolve config path: %v\n", err)
			os.Exit(1)
		}
		data.ConfigPath = abs
	}

	var buf bytes.Buffer
	if err := plistTemplate.Execute(&buf, data); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not render plist: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not create LaunchAgents directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(plistPath, buf.Bytes(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not write plist: %v\n", err)
		os.Exit(1)
	}

	if err := exec.Command("launchctl", "bootstrap", guiDomain(), plistPath).Run(); err != nil {
		if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load service: %v\n", err)
			fmt.Fprintf(os.Stderr, "plist written to %s — load it manually with:\n", plistPath)
			fmt.Fprintf(os.Stderr, "  launchctl load %s\n", plistPath)
			return
		}
	}

	fmt.Printf("service installed and loaded\n")
	fmt.Printf("plist: %s\n", plistPath)

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
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: service not installed (no plist at %s)\n", plistPath)
		os.Exit(1)
	}

	if err := exec.Command("launchctl", "bootout", serviceTarget()).Run(); err != nil {
		_ = exec.Command("launchctl", "unload", plistPath).Run()
	}

	if err := os.Remove(plistPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not remove plist: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("service uninstalled\n")
}

func serviceStart() {
	if err := exec.Command("launchctl", "kickstart", serviceTarget()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not start service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service started\n")
}

func serviceStop() {
	if err := exec.Command("launchctl", "kill", "SIGTERM", serviceTarget()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not stop service: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("service stopped\n")
}
