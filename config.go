package main

import (
	"fmt"
	"os"
	"os/exec"
	"text/template"
	"time"
)

// BGP Configuration structure
type BGPConfig struct {
	MyASN       uint32
	RouterID    string
	Peers       []BGPPeer
	Networks    []Network
	RouteMaps   []RouteMap
	PrefixLists []PrefixList
	VRFs        []VRF
}

// BGP Peer structure
type BGPPeer struct {
	PeerASN     uint32
	PeerIP      string
	Password    string
	Description string
	RouteMapIn  string
	RouteMapOut string
	VRF         string
}

// Network structure for VRF support
type Network struct {
	Prefix string
	VRF    string
}

// Route Map structure
type RouteMap struct {
	Name   string
	Permit bool
	Seq    int
	Prefix string
}

// Prefix List structure
type PrefixList struct {
	Name   string
	Seq    int
	Permit bool
	Prefix string
}

// VRF structure
type VRF struct {
	Name string
	RD   string // Route Distinguisher
}

const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log file /var/log/frr.log

{% for vrf in .VRFs %}
vrf {{vrf.Name}}
 rd {{vrf.RD}}
exit
{% endfor %}

router bgp {{.MyASN}}
 bgp router-id {{.RouterID}}

{% for vrf in .VRFs %}
 address-family ipv4 vrf {{vrf.Name}}
 exit-address-family
{% endfor %}

{% for peer in .Peers %}
{% if peer.VRF != "" %}
 address-family ipv4 vrf {{peer.VRF}}
{% endif %}
 neighbor {{peer.PeerIP}} remote-as {{peer.PeerASN}}
{% if peer.Password != "" %}
 neighbor {{peer.PeerIP}} password {{peer.Password}}
{% endif %}
{% if peer.Description != "" %}
 neighbor {{peer.PeerIP}} description "{{peer.Description}}"
{% endif %}
{% if peer.RouteMapIn != "" %}
 neighbor {{peer.PeerIP}} route-map {{peer.RouteMapIn}} in
{% endif %}
{% if peer.RouteMapOut != "" %}
 neighbor {{peer.PeerIP}} route-map {{peer.RouteMapOut}} out
{% endif %}
{% if peer.VRF != "" %}
 exit-address-family
{% endif %}
{% endfor %}

{% for network in .Networks %}
{% if network.VRF != "" %}
 address-family ipv4 vrf {{network.VRF}}
{% endif %}
 network {{network.Prefix}}
{% if network.VRF != "" %}
 exit-address-family
{% endif %}
{% endfor %}

exit
`

func generateFRRConfig(config BGPConfig, outputPath string) error {
	// Parse the template
	tmpl, err := template.New("frrConfig").Parse(frrConfigTemplate)
	if err != nil {
		return err
	}

	// Create or overwrite the configuration file
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Execute template and write to file
	err = tmpl.Execute(file, config)
	if err != nil {
		return err
	}

	fmt.Println("FRR configuration generated successfully at", outputPath)
	return nil
}

func reloadFRR() error {
	fmt.Println("Reloading FRR configuration...")
	cmd := exec.Command("/usr/lib/frr/frr-reload.py", "--reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Failed to reload FRR:", err, string(output))
		return err
	}
	fmt.Println("FRR successfully reloaded.")
	return nil
}

func main() {
	// Sample BGP configuration
	bgpConfig := BGPConfig{
		MyASN:    64512,
		RouterID: "192.168.1.1",
		Peers: []BGPPeer{
			{PeerASN: 64513, PeerIP: "192.168.1.2", Description: "Peer Router 1", RouteMapIn: "RM-IN", RouteMapOut: "RM-OUT", VRF: "CUSTOMER_A"},
			{PeerASN: 64514, PeerIP: "192.168.1.3", Password: "securepass", VRF: "CUSTOMER_B"},
		},
		Networks: []Network{
			{Prefix: "10.1.1.0/24", VRF: "CUSTOMER_A"},
			{Prefix: "192.168.100.0/24", VRF: "CUSTOMER_B"},
		},
		PrefixLists: []PrefixList{
			{Name: "PL-IN", Seq: 10, Permit: true, Prefix: "10.1.1.0/24"},
			{Name: "PL-OUT", Seq: 20, Permit: false, Prefix: "192.168.100.0/24"},
		},
		RouteMaps: []RouteMap{
			{Name: "RM-IN", Permit: true, Seq: 10, Prefix: "PL-IN"},
			{Name: "RM-OUT", Permit: false, Seq: 20, Prefix: "PL-OUT"},
		},
		VRFs: []VRF{
			{Name: "CUSTOMER_A", RD: "100:1"},
			{Name: "CUSTOMER_B", RD: "200:1"},
		},
	}

	// Generate FRR config and reload FRR periodically
	for {
		err := generateFRRConfig(bgpConfig, "/etc/frr/frr.conf")
		if err != nil {
			fmt.Println("Error generating FRR config:", err)
		} else {
			err = reloadFRR()
			if err != nil {
				fmt.Println("Error reloading FRR:", err)
			}
		}
		time.Sleep(30 * time.Second) // Refresh config every 30s
	}
}
