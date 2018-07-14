package main

import(
    "flag"
    "fmt"
    "log"
    "net/http"
    "github.com/neophenix/lxdepot/internal/config"
    "github.com/neophenix/lxdepot/internal/lxd"
    "github.com/neophenix/lxdepot/internal/handlers"
    "github.com/neophenix/lxdepot/internal/handlers/ws"
)

var port string
var conf string
var webroot string
var cacheTemplates bool
var Conf *config.Config

func main() {
    flag.StringVar(&port, "port", "8080", "port number to listen on")
    flag.StringVar(&conf, "config", "configs/config.yaml", "config file")
    flag.StringVar(&webroot, "webroot", "web/", "path of webroot (templates, static, etc)")
    flag.BoolVar(&cacheTemplates, "cache_templates", true, "cache templates or read from disk each time")
    flag.Parse()

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
    handlers.AddRoute("/container/new$", handlers.NewContainerHandler)
    handlers.AddRoute("/container/.*", handlers.ContainerHandler)
    handlers.AddRoute("/images$", handlers.ImageListHandler)
    handlers.AddRoute("/hosts$", handlers.HostListHandler)
    handlers.AddRoute("/ws$", ws.Handler)

    http.HandleFunc("/", handlers.RootHandler)

    log.Fatal(http.ListenAndServe(":" + port, nil))
}
