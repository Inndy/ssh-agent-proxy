ssh-agent-proxy: *.go
	go build -trimpath -o $@ .
