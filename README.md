# LXDepot

LXDepot is a simple UI to interface with one to many LXD servers, allowing you to start, stop, create, and delete containers.

Additionally, it can talk to third party DNS providers to automatically register and remove records, and bootstrap containers so once a user hits create, they can sit back for a moment and then be ready to SSH in and begin work.

## Usage

LXDepot has only a few command line flags to keep things simple
```
    -port       (default:8080) which port to bind to
    -config     (default:configs/config.yaml) instance config file
    -webroot    (default:web/) where our templates + static files live
    -cache_templates (default:true) more for dev work, setting to false make the service read the web templates off disk each request
```

Ex.
```
./lxdepot -port=8888 -config=/opt/lxdepot/configs/config.yaml -webroot=/opt/lxdepot/web/
```

## Config

The config file controls PKI, the hosts we talk to, DNS configuration, and bootstrapping commands.  A fully documented sample config can be found in [configs/sample.yaml](configs/sample.yaml)

## PKI

To use this you need to create a client cert and key using openssl or similar.  This cert will then need added to all the LXD hosts you want to talk to via
```
lxc config trust add CERTFILE
```

## Disabling remote management for certain containers

Sometimes you don't want people messing with your stuff.  To that end, if you do not want LXDepot to manage a container, that is to say start, stop, delete (it will still be listed and you can view info on it), add this user flag to the container.  It will tell LXDepot the container is off limits
```
lxc config set CONTAINERNAME user.lxdepot_lock true
```

LXDepot has no way to set that remotely, so an admin on the host will need to handle that.  Use unset or change the value from true to anything else for it to be unlocked

### Limitations

First, this was an experiment in learning Go, so I'm sure there are a few things that make you go ... wat

Secondly, everthing was initially developed for use at [Circonus](https://www.circonus.com) so perhaps some assumptions were made (like limiting to IPv4), and / or limitations in support of images or GCP DNS because that is what we use.

Last, tests ... haven't figured that part out yet.
