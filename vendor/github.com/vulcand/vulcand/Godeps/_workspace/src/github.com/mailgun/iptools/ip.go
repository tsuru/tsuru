package iptools

import "net"

// Ranges of addresses allocated by IANA for private internets, as per RFC1918.
var PrivateNetworks = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// GetHostIPs returns a list of IP addresses of all host's interfaces.
func GetHostIPs() ([]net.IP, error) {
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	for _, iface := range ifaces {
		if ipnet, ok := iface.(*net.IPNet); ok {
			ips = append(ips, ipnet.IP)
		}
	}

	return ips, nil
}

// GetPrivateHostIPs returns a list of host's private IP addresses.
func GetPrivateHostIPs() ([]net.IP, error) {
	ips, err := GetHostIPs()
	if err != nil {
		return nil, err
	}

	var privateIPs []net.IP
	for _, ip := range ips {
		// skip loopback, non-IPv4 and non-private addresses
		if ip.IsLoopback() || ip.To4() == nil || !IsPrivate(ip) {
			continue
		}
		privateIPs = append(privateIPs, ip)
	}

	return privateIPs, nil
}

// IsPrivate determines whether a passed IP address is from one of private blocks or not.
func IsPrivate(ip net.IP) bool {
	for _, network := range PrivateNetworks {
		_, ipnet, _ := net.ParseCIDR(network)
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}
