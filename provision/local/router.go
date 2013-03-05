package local

import (
	"fmt"
	"github.com/globocom/config"
	"os/exec"
)

// route represents an route.
type route struct {
	name string
	ip   string
}

func AddRoute(name, ip string) error {
	domain, err := config.GetString("local:domain")
	if err != nil {
		return err
	}
	routesPath, err := config.GetString("local:routes-path")
	if err != nil {
		return err
	}
	file, _ := filesystem().Open(routesPath + "/" + name)
	defer file.Close()
	template := `server {
	listen 80;
	%s.%s;
	location / {
		proxy_pass http://%s;
	}
}`
	template = fmt.Sprintf(template, name, domain, ip)
	data := []byte(template)
	_, err = file.Write(data)
	return err
}

func RestartRouter() error {
	cmd := exec.Command("sudo", "service", "nginx", "restart")
	return cmd.Run()
}
