















package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

var (
	
	ErrServiceUnknown = errors.New("service unknown")

	
	
	ErrServiceOffline = errors.New("service offline")

	
	
	ErrServiceUnreachable = errors.New("service unreachable")

	
	
	ErrNotExposed = errors.New("service not exposed, nor proxied")
)



type containerInfos struct {
	running bool              
	envvars map[string]string 
	portmap map[string]int    
	volumes map[string]string 
}


func inspectContainer(client *sshClient, container string) (*containerInfos, error) {
	
	out, err := client.Run(fmt.Sprintf("docker inspect %s", container))
	if err != nil {
		return nil, ErrServiceUnknown
	}
	
	type inspection struct {
		State struct {
			Running bool
		}
		Mounts []struct {
			Source      string
			Destination string
		}
		Config struct {
			Env []string
		}
		HostConfig struct {
			PortBindings map[string][]map[string]string
		}
	}
	var inspects []inspection
	if err = json.Unmarshal(out, &inspects); err != nil {
		return nil, err
	}
	inspect := inspects[0]

	
	infos := &containerInfos{
		running: inspect.State.Running,
		envvars: make(map[string]string),
		portmap: make(map[string]int),
		volumes: make(map[string]string),
	}
	for _, envvar := range inspect.Config.Env {
		if parts := strings.Split(envvar, "="); len(parts) == 2 {
			infos.envvars[parts[0]] = parts[1]
		}
	}
	for portname, details := range inspect.HostConfig.PortBindings {
		if len(details) > 0 {
			port, _ := strconv.Atoi(details[0]["HostPort"])
			infos.portmap[portname] = port
		}
	}
	for _, mount := range inspect.Mounts {
		infos.volumes[mount.Destination] = mount.Source
	}
	return infos, err
}



func tearDown(client *sshClient, network string, service string, purge bool) ([]byte, error) {
	
	out, err := client.Run(fmt.Sprintf("docker rm -f %s_%s_1", network, service))
	if err != nil {
		return out, err
	}
	
	if purge {
		return client.Run(fmt.Sprintf("docker rmi %s/%s", network, service))
	}
	return nil, nil
}



func resolve(client *sshClient, network string, service string, port int) (string, error) {
	
	infos, err := inspectContainer(client, fmt.Sprintf("%s_%s_1", network, service))
	if err != nil {
		return "", err
	}
	if !infos.running {
		return "", ErrServiceOffline
	}
	
	if vhost := infos.envvars["VIRTUAL_HOST"]; vhost != "" {
		return vhost, nil
	}
	return fmt.Sprintf("%s:%d", client.server, port), nil
}


func checkPort(host string, port int) error {
	log.Trace("Verifying remote TCP connectivity", "server", host, "port", port)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
