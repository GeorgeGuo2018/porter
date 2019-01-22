package util

import (
	"log"
	"net"
	"os/exec"
)

// Get preferred outbound ip of this machine
func GetOutboundIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

func ExecIPRuleCommand(operation, eip, table string) error {
	command := "ip rule " + operation + " to " + eip + "/32" + " lookup " + table
	_, err := exec.Command("bash", "-c", command).Output()
	return err
}
