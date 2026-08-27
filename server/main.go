package main

import "github.com/mattermost/mattermost/server/public/plugin"

// clientMain is indirected so the process entrypoint can be tested without starting the RPC server.
var clientMain = plugin.ClientMain

func main() {
	clientMain(&Plugin{})
}
