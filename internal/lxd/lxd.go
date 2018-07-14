package lxd

import(
    "os"
    "log"
    "time"
    "math"
    "errors"
    "strings"
    "io/ioutil"
    "github.com/lxc/lxd/client"
    "github.com/lxc/lxd/shared/api"
    "github.com/neophenix/lxdepot/internal/config"
)

var Conf *config.Config
var lxdConnections = make(map[string]lxd.ContainerServer)

type ContainerInfo struct {
    Host string
    Container api.Container
    State *api.ContainerState
    Usage map[string]float64
}

type ImageInfo struct {
    Host string
    Aliases []api.ImageAlias
    Architecture string
    Fingerprint string
}

type DiscardCloser struct{}

func (DiscardCloser) Write(b []byte) (int, error) {
    return ioutil.Discard.Write(b)
}
func (DiscardCloser) Close() error {
    return nil
}

func GetContainers(host string, name string, getState bool) ([]ContainerInfo, error) {
    var containerInfo []ContainerInfo

    for _, lxdh := range Conf.LXDhosts {
        if host == "" || lxdh.Host == host {
            conn, err := getConnection(lxdh.Host)
            if err != nil {
                log.Printf("Connection error to " + lxdh.Host + " : " + err.Error())
                continue
            }

            containers, err := conn.GetContainers()
            if err != nil {
                return containerInfo, err
            }

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
                        Host: lxdh.Host,
                        Container: container,
                        State: state,
                        Usage: make(map[string]float64),
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
                    Host: lxdh.Host,
                    Aliases: i.Aliases,
                    Architecture: i.Architecture,
                    Fingerprint: i.Fingerprint,
                }

                images = append(images, tmp)
            }
        }
    }

    return images, nil
}

func CreateContainer(host string, name string, image string) error {
    conn, err := getConnection(host)
    if err != nil {
        return err
    }

    containerInfo, err := GetContainers("", "", false)
    if err != nil {
        return err
    }

    if len(containerInfo) > 0 {
        for _, c := range containerInfo {
            if c.Container.Name == name {
                return errors.New("container already exists")
            }
        }
    }

    req := api.ContainersPost{
        Name: name,
        Source: api.ContainerSource{
            Type: "image",
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

func StartContainer(host string, name string) error {
    conn, err := getConnection(host)
    if err != nil {
        return err
    }

    containerInfo, err := GetContainers(host, name, true)
    if err != nil {
        return err
    }

    if len(containerInfo) > 0 {
        for _, c := range containerInfo {
            if c.Container.Name == name && c.State.Status == "Running" {
                // our container is already running so bail
                return nil
            }
            if c.Container.ExpandedConfig["user.lxdepot_lock"] == "true" {
                return errors.New("lock flag set, remote management denied")
            }
        }
    } else {
        return errors.New("container does not exist")
    }

    reqState := api.ContainerStatePut{
        Action: "start",
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

func StopContainer(host string, name string) error {
    conn, err := getConnection(host)
    if err != nil {
        return err
    }

    containerInfo, err := GetContainers(host, name, true)
    if err != nil {
        return err
    }

    if len(containerInfo) > 0 {
        for _, c := range containerInfo {
            if c.Container.Name == name && c.State.Status == "Stopped" {
                // our container is already stopped so bail
                return nil
            }

            if c.Container.ExpandedConfig["user.lxdepot_lock"] == "true" {
                return errors.New("lock flag set, remote management denied")
            }
        }
    } else {
        return errors.New("container does not exist")
    }

    reqState := api.ContainerStatePut{
        Action: "stop",
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

func DeleteContainer(host string, name string) error {
    conn, err := getConnection(host)
    if err != nil {
        return err
    }

    containerInfo, err := GetContainers(host, name, true)
    if err != nil {
        return err
    }

    if len(containerInfo) > 0 {
        for _, c := range containerInfo {
            if c.Container.ExpandedConfig["user.lxdepot_lock"] == "true" {
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

func GetHostResources(host string) (map[string]*api.Resources, error) {
    resourceHostMap := make(map[string]*api.Resources)

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

            resourceHostMap[lxdh.Host] = resources
        }
    }

    return resourceHostMap, nil
}

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
        Mode: mode,
        Type: filetype,
    }

    err = conn.CreateContainerFile(name, path, args)
    if err != nil {
        return err
    }

    return nil
}

func ExecCommand(host string, name string, command []string) error {
    conn, err := getConnection(host)
    if err != nil {
        return err
    }

    cmd := api.ContainerExecPost{
        Command: command,
        WaitForWS: true,
        Interactive: false,
    }

    // We can't seem to get an accurate answer if the command executes or not, so
    // just going to toss the output until that changes
    var ignore DiscardCloser
    args := lxd.ContainerExecArgs{
        Stdin: os.Stdin,
        Stdout: ignore,
        Stderr: ignore,
    }

    // schedule the command to execute
    op, err := conn.ExecContainer(name, cmd, &args)
    if err != nil {
        return err
    }

    // wait for the create to finish, in testing even if there is an error in lxc this
    // doesn't report it, so ... yeah
    err = op.Wait()
    if err != nil {
        return err
    }

    return nil
}

func getConnection(host string) (lxd.ContainerServer, error) {
    if conn, ok := lxdConnections[host]; ok {
        return conn, nil
    }

    var lxdh *config.LXDhost
    for _, h := range(Conf.LXDhosts) {
        if h.Host == host {
            lxdh = h
        }
    }

    if lxdh.Host == "" {
        log.Fatal("Could not find lxdhost ["+host+"] in config\n")
    }

    args := &lxd.ConnectionArgs{
        TLSClientCert: Conf.Cert,
        TLSClientKey: Conf.Key,
        TLSServerCert: lxdh.Cert,
    }
    conn, err := lxd.ConnectLXD("https://"+lxdh.Host+":"+lxdh.Port, args)
    if err != nil {
        return conn, err
    }

    lxdConnections[host] = conn

    return conn, nil
}
