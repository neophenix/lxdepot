// Package lxd is our wrapper to the official lxd client
package lxd

import (
	"errors"
	"fmt"
	"github.com/lxc/lxd/client"
	"github.com/lxc/lxd/shared/api"
	"github.com/neophenix/lxdepot/internal/config"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"
	"time"
)

// config from our main function
var Conf *config.Config

// cache of connections to our LXD servers
var lxdConnections = make(map[string]lxd.ContainerServer)

// ContainerInfo is a conversion / grouping of useful container information as returned from the lxd client
type ContainerInfo struct {
	Host      *config.LXDhost     // Host details
	Container api.Container       // Container details returned from lxd.GetContainers
	State     *api.ContainerState // Container state from lxd.GetContainerState
	Usage     map[string]float64  // place to store usge conversions, like CPU usage
}

// ImageInfo like above is a grouping of useful image information for the frontend
type ImageInfo struct {
	Host         *config.LXDhost  // Host details
	Aliases      []api.ImageAlias // list of aliases this image goes by
	Architecture string           // x86_64, etc
	Fingerprint  string           // fingerprint hash of the image for comparison
}

// HostResourceInfo is a group of Host and Resources as returned by lxd
type HostResourceInfo struct {
	Host      *config.LXDhost
	Resources *api.Resources
}

// DiscardCloser is a WriteCloser that just discards data.  When we exec commands on a container
// stdout, etc need some place to go, but at the moment we don't care about the data.
type DiscardCloser struct{}

// Write just sends its data to the ioutil.Discard object
func (DiscardCloser) Write(b []byte) (int, error) {
	return ioutil.Discard.Write(b)
}

// Close does nothing and is there just to satisfy the WriteCloser interface
func (DiscardCloser) Close() error {
	return nil
}

// GetContainers asks for a list of containers from each LXD host, then optionally calls GetContainerState
// on each container to populate state information (IP, CPU / Memory / Disk usage, etc)
func GetContainers(host string, name string, getState bool) ([]ContainerInfo, error) {
	var containerInfo []ContainerInfo

	// Always try to loop over the config array of hosts so we maintain the same ordering
	for _, lxdh := range Conf.LXDhosts {
		if host == "" || lxdh.Host == host {
			conn, err := getConnection(lxdh.Host)
			if err != nil {
				log.Printf("Connection error to " + lxdh.Host + " : " + err.Error())
				continue
			}

			// annoyingly this doesn't return all the state information we want too, so we just get a list of containers
			containers, err := conn.GetContainers()
			if err != nil {
				return containerInfo, err
			}

			// Here wer are going to either just return the container asked for by name, and look to see if we wanted
			// state info as well, typically we do, which could get expensive with lots of hosts or lots of containers
			for _, container := range containers {
				if name == "" || container.Name == name {
					var state *api.ContainerState
					if getState {
						state, err = GetContainerState(lxdh.Host, container.Name)
						if err != nil {
							log.Printf("Could not get container state from " + lxdh.Host + " for " + container.Name)
							break
						}
					}

					tmp := ContainerInfo{
						Host:      lxdh,
						Container: container,
						State:     state,
						Usage:     make(map[string]float64),
					}

					if getState {
						// Using a map here so the output in the html template isn't a complete pain
						tmp.Usage["cpu"] = float64(state.CPU.Usage/1000000000) / math.Abs(time.Now().Sub(container.LastUsedAt).Seconds())
					}

					containerInfo = append(containerInfo, tmp)
				}
			}
		}
	}

	return containerInfo, nil
}

// GetContainerState calls out to our LXD host to get the state of the container.  State has data like network info,
// memory usage, cpu seconds in use, running processes etc
func GetContainerState(host string, name string) (*api.ContainerState, error) {
	conn, err := getConnection(host)
	if err != nil {
		return nil, err
	}

	state, _, err := conn.GetContainerState(name)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// GetImages calls each LXD host to get a list of images available on each
func GetImages(host string) ([]ImageInfo, error) {
	var images []ImageInfo

	for _, lxdh := range Conf.LXDhosts {
		if host == "" || lxdh.Host == host {
			conn, err := getConnection(lxdh.Host)
			if err != nil {
				log.Printf("Connection error to " + lxdh.Host + " : " + err.Error())
				continue
			}

			imgs, err := conn.GetImages()
			if err != nil {
				return images, err
			}

			for _, i := range imgs {
				tmp := ImageInfo{
					Host:         lxdh,
					Aliases:      i.Aliases,
					Architecture: i.Architecture,
					Fingerprint:  i.Fingerprint,
				}

				images = append(images, tmp)
			}
		}
	}

	return images, nil
}

// CreateContainer creates a container from the given image, with the provided name on the LXD host
func CreateContainer(host string, name string, image string) error {
	conn, err := getConnection(host)
	if err != nil {
		return err
	}

	// We are going to grab a list of containers first to make sure someone isn't trying to create a duplicate name.
	// Look at every host as we might want to move the container later, and you can't do that if there is already that
	// name on a host, so our list of managed hosts is like a fake cluster
	containerInfo, err := GetContainers("", "", false)
	if err != nil {
		return err
	}

	if len(containerInfo) > 0 {
		for _, c := range containerInfo {
			if c.Container.Name == name {
				return errors.New("container already exists on " + c.Host.Name)
			}
		}
	}

	req := api.ContainersPost{
		Name: name,
		Source: api.ContainerSource{
			Type:  "image",
			Alias: image,
		},
	}

	// schedule the create with LXD, this happens in the background
	op, err := conn.CreateContainer(req)
	if err != nil {
		return err
	}

	// wait for the create to finish
	err = op.Wait()
	if err != nil {
		return err
	}

	return nil
}

// StartContainer starts a stopped container
func StartContainer(host string, name string) error {
	conn, err := getConnection(host)
	if err != nil {
		return err
	}

	// Grab container info to make sure our container isn't already running
	containerInfo, err := GetContainers(host, name, false)
	if err != nil {
		return err
	}

	if len(containerInfo) > 0 {
		for _, c := range containerInfo {
			if c.Container.Name == name && c.Container.Status == "Running" {
				// our container is already running so bail
				return nil
			}

			// don't allow remote management of anything we have locked
			if !IsManageable(c) {
				return errors.New("lock flag set, remote management denied")
			}
		}
	} else {
		return errors.New("container does not exist")
	}

	reqState := api.ContainerStatePut{
		Action:  "start",
		Timeout: -1,
	}

	op, err := conn.UpdateContainerState(name, reqState, "")
	if err != nil {
		return err
	}

	// Like before the update is a background process, wait for it to finish
	err = op.Wait()
	if err != nil {
		return err
	}

	return nil
}

// StopContainer stops a running container
func StopContainer(host string, name string) error {
	conn, err := getConnection(host)
	if err != nil {
		return err
	}

	// Grab container info to make sure our container is actually running
	containerInfo, err := GetContainers(host, name, false)
	if err != nil {
		return err
	}

	if len(containerInfo) > 0 {
		for _, c := range containerInfo {
			if c.Container.Name == name && c.Container.Status == "Stopped" {
				// our container is already stopped so bail
				return nil
			}

			// don't allow remote management of anything we have locked
			if !IsManageable(c) {
				return errors.New("lock flag set, remote management denied")
			}
		}
	} else {
		return errors.New("container does not exist")
	}

	reqState := api.ContainerStatePut{
		Action:  "stop",
		Timeout: -1,
	}

	op, err := conn.UpdateContainerState(name, reqState, "")
	if err != nil {
		return err
	}

	// Like before the update is a background process, wait for it to finish
	err = op.Wait()
	if err != nil {
		return err
	}

	return nil
}

// DeleteContainer removes a container from a host
func DeleteContainer(host string, name string) error {
	conn, err := getConnection(host)
	if err != nil {
		return err
	}

	// Get container list to make sure we actually have a container with this name
	containerInfo, err := GetContainers(host, name, false)
	if err != nil {
		return err
	}

	if len(containerInfo) > 0 {
		for _, c := range containerInfo {
			// don't allow remote management of anything we have locked
			if !IsManageable(c) {
				return errors.New("lock flag set, remote management denied")
			}
		}
	} else {
		return errors.New("container does not exist")
	}

	op, err := conn.DeleteContainer(name)
	if err != nil {
		return err
	}

	// Like before the update is a background process, wait for it to finish
	err = op.Wait()
	if err != nil {
		return err
	}

	return nil
}

// GetHostResources grabs (the kind of limited) info about a host, available CPU cores, Memory, ...
func GetHostResources(host string) (map[string]HostResourceInfo, error) {
	resourceHostMap := make(map[string]HostResourceInfo)

	for _, lxdh := range Conf.LXDhosts {
		if host == "" || lxdh.Host == host {
			conn, err := getConnection(lxdh.Host)
			if err != nil {
				log.Printf("Connection error to " + lxdh.Host + " : " + err.Error())
				continue
			}

			resources, err := conn.GetServerResources()
			if err != nil {
				return resourceHostMap, err
			}

			resourceHostMap[lxdh.Host] = HostResourceInfo{
				Host:      lxdh,
				Resources: resources,
			}
		}
	}

	return resourceHostMap, nil
}

// CreateFile creates a file or directory on the container.  If the provided path ends in / we assume
// that we are creating a directory
func CreateFile(host string, name string, path string, mode int, contents string) error {
	conn, err := getConnection(host)
	if err != nil {
		return err
	}

	filetype := "file"
	if strings.HasSuffix(path, "/") {
		filetype = "directory"
	}

	args := lxd.ContainerFileArgs{
		Content: strings.NewReader(contents),
		Mode:    mode,
		Type:    filetype,
	}

	err = conn.CreateContainerFile(name, path, args)
	if err != nil {
		return err
	}

	return nil
}

// ExecCommand runs a command on the container and discards the output.  As further comments state,
// there doesn't seem to be an accurate return of success or not, need to look for a status code return.
// If a way is found, likely will stop discarding output and return that to the UI.  -1 is our return if
// something outside the command went wrong
func ExecCommand(host string, name string, command []string) (float64, error) {
	conn, err := getConnection(host)
	if err != nil {
		return -1, err
	}

	cmd := api.ContainerExecPost{
		Command:     command,
		WaitForWS:   true,
		Interactive: false,
	}

	// We can't seem to get an accurate answer if the command executes or not, so
	// just going to toss the output until that changes
	var ignore DiscardCloser
	args := lxd.ContainerExecArgs{
		Stdin:  os.Stdin,
		Stdout: ignore,
		Stderr: ignore,
	}

	// schedule the command to execute
	op, err := conn.ExecContainer(name, cmd, &args)
	if err != nil {
		return -1, err
	}

	// wait for the command to finish
	err = op.Wait()
	if err != nil {
		return -1, err
	}

	// Get the status of the command and convert the return value to a number
	status := op.Get()
	statuscode, ok := status.Metadata["return"].(float64)
	if !ok {
		return -1, errors.New("failed to parse return value")
	}

	return statuscode, nil
}

// MoveContainer will move (copy in lxd speak) a container from one server to another.
func MoveContainer(src_host string, dst_host string, name string) error {
	// copy works by first marking the container as ready for migration, then connecting to the
	// destination and telling it to make a copy, then probably deleting from the source
	srcconn, err := getConnection(src_host)
	if err != nil {
		return err
	}

	dstconn, err := getConnection(dst_host)
	if err != nil {
		return err
	}

	// Get container list to make sure we actually have a container with this name
	containerInfo, err := GetContainers(src_host, name, false)
	if err != nil {
		return err
	}

	if len(containerInfo) > 0 {
		for _, c := range containerInfo {
			// don't allow remote management of anything we have locked
			if !IsManageable(c) {
				return errors.New("lock flag set, remote management denied")
			}
		}
	} else {
		return errors.New("container does not exist")
	}

	// set our migration status to true
	err = toggleMigration(srcconn, name, true)
	if err != nil {
		return err
	}

	// Now on the destination, try and copy it?
	c := api.Container{
		Name: name,
	}
	args := &lxd.ContainerCopyArgs{
		Live: true,
	}
	op, err := dstconn.CopyContainer(srcconn, c, args)
	if err != nil {
		err2 := toggleMigration(srcconn, name, false)
		if err2 != nil {
			return fmt.Errorf("Error copying container (%v) error while unmigrating container (%v)", err, err2)
		}
		return err
	}

	err = op.Wait()
	if err != nil {
		err2 := toggleMigration(srcconn, name, false)
		if err2 != nil {
			return fmt.Errorf("Error copying container (%v) error while unmigrating container (%v)", err, err2)
		}
		return err
	}

	// And finally remove the container from the src, if this fails we aren't going to try to rollback anything
	err = DeleteContainer(src_host, name)
	return err
}

// toggleMigration is a helper for MoveContainer to toggle the migration flag on / off if
// we want to move it, or then later run into an error and need to flip it back
func toggleMigration(conn lxd.ContainerServer, name string, migrate bool) error {
	post := api.ContainerPost{
		Migration: migrate,
		Live:      migrate,
	}

	// like other commands, get the operation and then wait on it, just return here, later
	// if we hit an error we probably need to try to un-migrate the thing?
	op, err := conn.MigrateContainer(name, post)
	if err != nil {
		return err
	}

	// TODO : OK, so the reason this doesn't return is once you kick off the migrate you need to
	//        Then make a copy request to run at the same time.  In experimenting with this I keep
	//        getting an error "Architecture isn't supported:" which I can't find any info about
	err = op.Wait()
	return err
}

// IsManageable just checks our lock flag, user.lxdepot_lock to see if it is "true" or not
func IsManageable(c ContainerInfo) bool {
	// don't allow remote management of anything we have locked
	if c.Container.ExpandedConfig["user.lxdepot_lock"] == "true" {
		return false
	}

	return true
}

// getConnection will either return a cached connection, or reach out and make a new connection
// to the host before caching that
func getConnection(host string) (lxd.ContainerServer, error) {
	if conn, ok := lxdConnections[host]; ok {
		return conn, nil
	}

	var lxdh *config.LXDhost
	for _, h := range Conf.LXDhosts {
		if h.Host == host {
			lxdh = h
		}
	}

	if lxdh.Host == "" {
		log.Fatal("Could not find lxdhost [" + host + "] in config\n")
	}

	args := &lxd.ConnectionArgs{
		TLSClientCert: Conf.Cert,
		TLSClientKey:  Conf.Key,
		TLSServerCert: lxdh.Cert,
	}
	conn, err := lxd.ConnectLXD("https://"+lxdh.Host+":"+lxdh.Port, args)
	if err != nil {
		return conn, err
	}

	lxdConnections[host] = conn

	return conn, nil
}
