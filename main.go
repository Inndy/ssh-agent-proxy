package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/crypto/ssh/agent"
)

var version = "dev"

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "service" {
		handleServiceCommand(os.Args[2:])
		return
	}

	fmt.Fprintf(os.Stderr, "Usage: %s service <command>\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  service run       Run the proxy agent (foreground)\n")
	fmt.Fprintf(os.Stderr, "  service install    Install system service (launchd/systemd)\n")
	fmt.Fprintf(os.Stderr, "  service uninstall  Remove system service\n")
	fmt.Fprintf(os.Stderr, "  service start      Start the service\n")
	fmt.Fprintf(os.Stderr, "  service stop       Stop the service\n")
	fmt.Fprintf(os.Stderr, "\nFlags for 'service run':\n")
	fmt.Fprintf(os.Stderr, "  -config string   config file path\n")
	fmt.Fprintf(os.Stderr, "  -listen string   override listen socket path\n")
	fmt.Fprintf(os.Stderr, "  -log string      override log file path\n")
	fmt.Fprintf(os.Stderr, "  -debug           enable debug-level logging\n")
	fmt.Fprintf(os.Stderr, "  -version         print version and exit\n")
	os.Exit(1)
}

func runServer(args []string) {
	fs := flag.NewFlagSet("service run", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s service run [flags]\n\nFlags:\n", os.Args[0])
		fs.PrintDefaults()
	}
	configPath := fs.String("config", defaultConfigPath(), "config file path")
	listenOverride := fs.String("listen", "", "override listen socket path")
	logOverride := fs.String("log", "", "override log file path")
	debug := fs.Bool("debug", false, "enable debug-level logging")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Parse(args)

	if *showVersion {
		fmt.Println("ssh-agent-proxy", version)
		os.Exit(0)
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *listenOverride != "" {
		cfg.Listen = expandTilde(*listenOverride)
	}
	if *logOverride != "" {
		cfg.Log.Enabled = true
		cfg.Log.File = expandTilde(*logOverride)
	}
	if *debug {
		cfg.Log.Enabled = true
		cfg.Log.Level = "debug"
	}

	logger, err := newLogger(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	upstreams := make([]*UpstreamAgent, len(cfg.Upstreams))
	for i := range cfg.Upstreams {
		upstreams[i] = NewUpstreamAgent(&cfg.Upstreams[i], logger)
		logger.Info("configured upstream", "name", cfg.Upstreams[i].Name, "socket", cfg.Upstreams[i].Socket, "cache", string(cfg.Upstreams[i].Cache))
	}

	proxy := NewProxyAgent(upstreams, logger)

	if _, err := proxy.List(); err != nil {
		logger.Warn("failed to pre-cache keys", "error", err)
	}

	os.Remove(cfg.Listen)
	ln, err := net.Listen("unix", cfg.Listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Chmod(cfg.Listen, 0600)

	logger.Info("listening", "socket", cfg.Listen)
	fmt.Fprintf(os.Stderr, "ssh-agent-proxy listening on %s\n", cfg.Listen)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGUSR1:
				logger.Info("received SIGUSR1, invalidating caches")
				proxy.InvalidateAllCaches()
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("shutting down", "signal", sig.String())
				ln.Close()
				os.Remove(cfg.Listen)
				os.Exit(0)
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Error("accept error", "error", err)
			break
		}
		go handleConn(conn, proxy, logger)
	}
}

func handleConn(conn net.Conn, proxy *ProxyAgent, logger *Logger) {
	defer conn.Close()

	cred, err := getPeerCred(conn)
	if err != nil {
		logger.Info("client connected", "peer_error", err.Error())
	} else {
		attrs := []any{"pid", cred.PID, "uid", cred.UID, "gid", cred.GID}
		if cmdline := getProcessCmdline(cred.PID); cmdline != "" {
			attrs = append(attrs, "cmdline", cmdline)
		}
		logger.Info("client connected", attrs...)
	}

	if err := agent.ServeAgent(proxy, conn); err != nil {
		logger.Debug("client disconnected", "error", err.Error())
	} else {
		logger.Info("client disconnected")
	}
}
