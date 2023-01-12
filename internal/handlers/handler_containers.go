package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"

	"github.com/neophenix/lxdepot/internal/lxd"
)

// ContainerListHandler handles requests for /containers
func ContainerListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	containerInfo, err := lxd.GetContainers("", "", true)
	if err != nil {
		log.Printf("Could not get container list %s\n", err.Error())
	}

	tmpl := readTemplate("container_list.tmpl")

	var out bytes.Buffer
	tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
		"Page":       "containers",
		"Containers": containerInfo,
	})

	fmt.Fprintf(w, string(out.Bytes()))
}

// ContainerHostListHandler handles requests for /containers/HOST
func ContainerHostListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	reg := regexp.MustCompile("/containers/(?P<Host>[^:]+)")
	match := reg.FindStringSubmatch(r.URL.Path)

	if len(match) != 2 {
		FourOhFourHandler(w, r)
		return
	}

	// Check that the host is actually one we have configured for use
	found := false
	for _, lxdh := range Conf.LXDhosts {
		if lxdh.Host == match[1] {
			found = true
		}
	}
	if !found {
		FourOhFourHandler(w, r)
		return
	}

	containerInfo, err := lxd.GetContainers(match[1], "", true)
	if err != nil {
		log.Printf("Could not get container list %s\n", err.Error())
	}

	tmpl := readTemplate("container_list.tmpl")

	var out bytes.Buffer
	tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
		"Page":       "containers",
		"Containers": containerInfo,
	})

	fmt.Fprintf(w, string(out.Bytes()))
}

// ContainerHandler handles requests for /container/HOST:NAME
func ContainerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	reg := regexp.MustCompile("/container/(?P<Host>[^:]+):(?P<Name>.+)")
	match := reg.FindStringSubmatch(r.URL.Path)

	if len(match) != 3 {
		FourOhFourHandler(w, r)
		return
	}

	containerInfo, err := lxd.GetContainers(match[1], match[2], true)
	if err != nil {
		log.Printf("Could not get container list %s\n", err.Error())
	}
	if len(containerInfo) == 0 {
		FourOhFourHandler(w, r)
		return
	}

	// Check to see if we have a bootstrap section and playbooks section for
	// this OS, if we do, built a list of those items for the UI to list off
	// to the user as options to run
	var playbooks []string
	os := containerInfo[0].Container.ExpandedConfig["image.os"] + containerInfo[0].Container.ExpandedConfig["image.release"]
	if pbs, ok := Conf.Playbooks[os]; ok {
		for name := range pbs {
			playbooks = append(playbooks, name)
		}
	}
	if _, ok := Conf.Bootstrap[os]; ok {
		playbooks = append(playbooks, "bootstrap")
	}

	tmpl := readTemplate("container.tmpl")

	var out bytes.Buffer
	err = tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
		"Page":      "containers",
		"Conf":      Conf,
		"Container": containerInfo[0],
		"Playbooks": playbooks,
	})
	if err != nil {
		log.Printf("%v\n", err.Error())
	}

	fmt.Fprintf(w, string(out.Bytes()))
}

// NewContainerHandler handles requests for /container/new
func NewContainerHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	// images we want to map to host -> image alias so we can use JS
	// in the template to make sure we only select an image on the selected host
	images, err := lxd.GetImages("")
	if err != nil {
		log.Printf("Could not get image list %s\n", err.Error())
	}
	imageMap := make(map[string][]string)
	for _, image := range images {
		imageMap[image.Host.Host] = append(imageMap[image.Host.Host], image.Aliases[0].Name)
	}
	imageJSON, err := json.Marshal(imageMap)
	if err != nil {
		log.Printf("Could not JSONify images: %s\n", err.Error())
	}

	// Like the images, we are going to get a mapping of host resources and then
	// convert that to JSON to give the template something to work with
	hostResourceMap, err := lxd.GetHostResources("")
	if err != nil {
		log.Printf("Could not get host resource list %s\n", err.Error())
	}

	hostResourceJSON, err := json.Marshal(hostResourceMap)
	if err != nil {
		log.Printf("Could not JSONify host resource info %s\n", err.Error())
	}

	// Now grab the list of available storage pools so we can select that on creation
	hostStoragePools, err := lxd.GetStoragePools("")
	if err != nil {
		log.Printf("Could not get storage pools %s\n", err.Error())
	}

	hostStorageJSON, err := json.Marshal(hostStoragePools)
	if err != nil {
		log.Printf("Could not JSONify storage pools %s\n", err.Error())
	}

	tmpl := readTemplate("container_new.tmpl")

	var out bytes.Buffer
	tmpl.ExecuteTemplate(&out, "base", map[string]interface{}{
		"Page":             "containers",
		"Conf":             Conf,
		"ImageJSON":        template.JS(imageJSON),
		"HostResourceJSON": template.JS(hostResourceJSON),
		"HostStorageJSON":  template.JS(hostStorageJSON),
	})

	fmt.Fprintf(w, string(out.Bytes()))
}
