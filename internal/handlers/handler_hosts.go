package handlers

import(
    "fmt"
    "log"
    "bytes"
    "net/http"
    "github.com/neophenix/lxdepot/internal/lxd"
)

func HostListHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")

    hostResourceMap, err := lxd.GetHostResources("")
    if err != nil {
        log.Printf("Could not get host resource list %s\n", err.Error())
    }

    tmpl := readTemplate("host_list.tmpl")

    var out bytes.Buffer
    tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
        "Page": "hosts",
        "Conf": Conf,
        "HostResourceMap": hostResourceMap,
    })

    fmt.Fprintf(w, string(out.Bytes()))
}
