package main

import (
	"fmt"
	"os"
	"os/exec"
	"text/template"
	"time"
)

// GlobalConfig structure
type GlobalConfig struct {
	ASN      uint16
	LogLevel string
	Routers  []RouterConfig
}

// RouterConfig structure
type RouterConfig struct {
	VRF      string
	RouterID string
	Peers    []NeighborConfig
}

// NeighborConfig structure
type NeighborConfig struct {
	PeerIP      string
	PeerASN     string
	Password    string
	Description string
}

const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log syslog {{.LogLevel}}

{{- range .Routers }}
router bgp {{$.ASN}} vrf {{.VRF}}
 bgp router-id {{.RouterID}}
 bgp log-neighbor-changes
 bgp graceful-restart
 no bgp ebgp-requires-policy
 no bgp network import-check
 no bgp default ipv4-unicast

{{- range .Peers }}
 neighbor {{.PeerIP}} remote-as {{.PeerASN}}
{{- if .Password }}
 neighbor {{.PeerIP}} password {{.Password}}
{{- end }}
{{- if .Description }}
 neighbor {{.PeerIP}} description "{{.Description}}"
{{- end }}
{{- end }}

 address-family ipv4 unicast
{{- range .Peers }}
  neighbor {{.PeerIP}} activate
{{- end }}
 exit-address-family

exit
{{- end }}
`

/*
const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log syslog informational

router bgp 64512

	bgp router-id 192.168.1.1
	bgp log-neighbor-changes
	no bgp ebgp-requires-policy
	no bgp network import-check
	no bgp default ipv4-unicast
	bgp graceful-restart
	timers bgp 30 90

	address-family ipv4 unicast
	exit-address-family

	address-family ipv6 unicast
	exit-address-family

`
*/
func generateFRRConfig(config GlobalConfig, outputPath string) error {
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

	vtysh, err := os.Create("/etc/frr/vtysh.conf")
	if err != nil {
		return err
	}
	defer vtysh.Close()

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
	cmd := exec.Command("/usr/lib/frr/frr-reload.py", "--reload", "--overwrite", "/etc/frr/frr.conf")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Failed to reload FRR:", err, string(output))
		return err
	}
	fmt.Println("FRR successfully reloaded.")
	return nil
}

func main() {
	// Sample configuration
	globalConfig := GlobalConfig{
		ASN:      64512,
		LogLevel: "debug",
		Routers: []RouterConfig{
			{
				VRF:      "GOVNO",
				RouterID: "192.168.1.1",
				Peers: []NeighborConfig{
					{PeerIP: "192.168.1.2", PeerASN: "64513", Description: "DRUG GOVNA"},
					{PeerIP: "192.168.1.3", PeerASN: "64514", Password: "zalupa"},
				},
			},
			{
				VRF:      "MOCHA",
				RouterID: "192.168.2.1",
				Peers: []NeighborConfig{
					{PeerIP: "192.168.2.2", PeerASN: "64515", Description: "DRUG MOCHI"},
				},
			},
		},
	}

	// Generate FRR config and reload FRR periodically
	for {
		err := generateFRRConfig(globalConfig, "/etc/frr/frr.conf")
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
