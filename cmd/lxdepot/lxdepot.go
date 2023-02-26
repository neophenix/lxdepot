package main

/*  LXDepot is a simple UI that lets one manage containers across multiple LXD hosts
 *
 *  Usage (highlightling default values):
 *    ./lxdepot -port=8080 -config=configs/config.yaml -webroot=web/
 *
 *  See README.md for more detailed information, and configs/sample.yaml for more
 *  details on what a config should look like
 */

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/neophenix/lxdepot/internal/config"
	"github.com/neophenix/lxdepot/internal/handlers"
	"github.com/neophenix/lxdepot/internal/handlers/ws"
	"github.com/neophenix/lxdepot/internal/lxd"
)

// All our command line params and config
var port string
var conf string
var webroot string
var cacheTemplates bool

// Conf is our main config
var Conf *config.Config

func main() {
	// Pull in all the command line params
	flag.StringVar(&port, "port", "8080", "port number to listen on")
	flag.StringVar(&conf, "config", "configs/config.yaml", "config file")
	flag.StringVar(&webroot, "webroot", "web/", "path of webroot (templates, static, etc)")
	flag.BoolVar(&cacheTemplates, "cache_templates", true, "cache templates or read from disk each time")
	flag.Parse()

	// Decided that printing out our "running config" was useful in the event things went awry
	fmt.Printf("webroot: " + webroot + "\n")
	fmt.Printf("config: " + conf + "\n")
	fmt.Printf("Listening on " + port + "\n")

	Conf = config.ParseConfig(conf)

	// Hand out our settings / config to everyone
	lxd.Conf = Conf
	handlers.Conf = Conf
	ws.Conf = Conf

	handlers.WebRoot = webroot
	handlers.CacheTemplates = cacheTemplates

	// Static file server
	fs := http.FileServer(http.Dir(webroot + "/static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	http.Handle("/favicon.ico", fs)

	// Setup our routing
	handlers.AddRoute("/containers$", handlers.ContainerListHandler)
	handlers.AddRoute("/containers/.*$", handlers.ContainerHostListHandler)
	handlers.AddRoute("/container/new$", handlers.NewContainerHandler)
	handlers.AddRoute("/container/.*$", handlers.ContainerHandler)
	handlers.AddRoute("/images$", handlers.ImageListHandler)
	handlers.AddRoute("/hosts$", handlers.HostListHandler)
	handlers.AddRoute("/ws$", ws.Handler)

	// The root handler does all the route checking and handoffs
	http.HandleFunc("/", handlers.RootHandler)

	// our websocket maintenance function to clear out old buffers
	ws.ManageBuffers()

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
