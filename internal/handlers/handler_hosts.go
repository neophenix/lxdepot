package handlers

import(
    "fmt"
    "log"
    "bytes"
    "net/http"
    "github.com/neophenix/lxdepot/internal/lxd"
)

// HostListHandler handles requests for /hosts
func HostListHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")

    hostResourceMap, err := lxd.GetHostResources("")
    if err != nil {
        log.Printf("Could not get host resource list %s\n", err.Error())
    }

    // host -> container info mapping
    hostContainerInfo := make(map[string]map[string]int)
    // Grab container info without state to see installed vs runnings
    containerInfo, err := lxd.GetContainers("", "", false)
    if err != nil {
        log.Printf("Could not get container list %s\n", err.Error())
    }

    // Check the status of each container and increment the counter, if we haven't
    // seen this host before make the map we need
    for _, container := range containerInfo {
        if hostContainerInfo[container.Host.Host] == nil {
            hostContainerInfo[container.Host.Host] = make(map[string]int)
        }
        hostContainerInfo[container.Host.Host]["total"] += 1

        if container.Container.Status == "Running" {
            hostContainerInfo[container.Host.Host]["running"] += 1
        } else {
            hostContainerInfo[container.Host.Host]["stopped"] += 1
        }
    }

    tmpl := readTemplate("host_list.tmpl")

    var out bytes.Buffer
    tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
        "Page": "hosts",
        "Conf": Conf,
        "HostResourceMap": hostResourceMap,
        "HostContainerInfo": hostContainerInfo,
    })

    fmt.Fprintf(w, string(out.Bytes()))
}
