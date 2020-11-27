















package main

import (
	"fmt"

	"github.com/ethereum/go-ethereum/log"
)







func (w *wizard) ensureVirtualHost(client *sshClient, port int, def string) (string, error) {
	proxy, _ := checkNginx(client, w.network)
	if proxy != nil {
		
		if proxy.port == port {
			fmt.Println()
			fmt.Printf("Shared port, which domain to assign? (default = %s)\n", def)
			return w.readDefaultString(def), nil
		}
	}
	
	fmt.Println()
	fmt.Println("Allow sharing the port with other services (y/n)? (default = yes)")
	if w.readDefaultYesNo(true) {
		nocache := false
		if proxy != nil {
			fmt.Println()
			fmt.Printf("Should the reverse-proxy be rebuilt from scratch (y/n)? (default = no)\n")
			nocache = w.readDefaultYesNo(false)
		}
		if out, err := deployNginx(client, w.network, port, nocache); err != nil {
			log.Error("Failed to deploy reverse-proxy", "err", err)
			if len(out) > 0 {
				fmt.Printf("%s\n", out)
			}
			return "", err
		}
		
		fmt.Println()
		fmt.Printf("Proxy deployed, which domain to assign? (default = %s)\n", def)
		return w.readDefaultString(def), nil
	}
	
	return "", nil
}
