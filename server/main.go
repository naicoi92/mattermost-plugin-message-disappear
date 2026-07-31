// Package main is the server entrypoint for the Disappearing Messages plugin.
//
// It registers the Plugin with the Mattermost server via plugin.ClientMain.
package main

import (
	"github.com/mattermost/mattermost/server/public/plugin"
)

func main() {
	plugin.ClientMain(&Plugin{})
}
