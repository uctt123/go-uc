















package main

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"path/filepath"
	"strconv"

	"github.com/ethereum/go-ethereum/log"
)



var nginxDockerfile = `FROM jwilder/nginx-proxy`




var nginxComposefile = `
version: '2'
services:
  nginx:
    build: .
    image: {{.Network}}/nginx
    container_name: {{.Network}}_nginx_1
    ports:
      - "{{.Port}}:80"
    volumes:
      - /var/run/docker.sock:/tmp/docker.sock:ro
    logging:
      driver: "json-file"
      options:
        max-size: "1m"
        max-file: "10"
    restart: always
`




func deployNginx(client *sshClient, network string, port int, nocache bool) ([]byte, error) {
	log.Info("Deploying nginx reverse-proxy", "server", client.server, "port", port)

	
	workdir := fmt.Sprintf("%d", rand.Int63())
	files := make(map[string][]byte)

	dockerfile := new(bytes.Buffer)
	template.Must(template.New("").Parse(nginxDockerfile)).Execute(dockerfile, nil)
	files[filepath.Join(workdir, "Dockerfile")] = dockerfile.Bytes()

	composefile := new(bytes.Buffer)
	template.Must(template.New("").Parse(nginxComposefile)).Execute(composefile, map[string]interface{}{
		"Network": network,
		"Port":    port,
	})
	files[filepath.Join(workdir, "docker-compose.yaml")] = composefile.Bytes()

	
	if out, err := client.Upload(files); err != nil {
		return out, err
	}
	defer client.Run("rm -rf " + workdir)

	
	if nocache {
		return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s build --pull --no-cache && docker-compose -p %s up -d --force-recreate --timeout 60", workdir, network, network))
	}
	return nil, client.Stream(fmt.Sprintf("cd %s && docker-compose -p %s up -d --build --force-recreate --timeout 60", workdir, network))
}



type nginxInfos struct {
	port int
}



func (info *nginxInfos) Report() map[string]string {
	return map[string]string{
		"Shared listener port": strconv.Itoa(info.port),
	}
}



func checkNginx(client *sshClient, network string) (*nginxInfos, error) {
	
	infos, err := inspectContainer(client, fmt.Sprintf("%s_nginx_1", network))
	if err != nil {
		return nil, err
	}
	if !infos.running {
		return nil, ErrServiceOffline
	}
	
	return &nginxInfos{
		port: infos.portmap["80/tcp"],
	}, nil
}
