package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

// GlobalConfig structure
type GlobalConfig struct {
	ASN         uint16
	LogLevel    string
	Routers     []RouterConfig
	VRFs        []VRFConfig
	PrefixLists []PrefixListConfig
	RouteMaps   []RouteMapConfig
	EVPNs       []EVPNConfig
}

type EVPNConfig struct {
	VRF string
}

// PrefixListConfig structure
type PrefixListConfig struct {
	Name   string
	Seq    int
	Permit bool
	Prefix string
}

// RouteMapConfig structure
type RouteMapConfig struct {
	Name       string
	Permit     bool
	Seq        int
	PrefixList string
	MatchType  string
}

// StaticRouteConfig structure
type StaticRouteConfig struct {
	VRF         string
	Destination string
	NextHop     string
}

// VRFConfig structure
type VRFConfig struct {
	Name         string
	VNI          uint32
	StaticRoutes []StaticRouteConfig
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
	RouteMapIn  string
	RouteMapOut string
	Fabric      bool
}

const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log syslog {{.LogLevel}}

!{{- range $v:= .VRFs }}
vrf {{$v.Name}}
{{- if $v.StaticRoutes }}
{{- range $v.StaticRoutes }}
 ip route {{.Destination}} {{.NextHop}}
{{- end }}
{{- end }}
exit-vrf
{{- end }}

{{- range .PrefixLists }}
ip prefix-list {{.Name}} seq {{.Seq}} {{if .Permit}}permit{{else}}deny{{end}} {{.Prefix}}
{{- end }}

{{- range .RouteMaps }}
route-map {{.Name}} {{if .Permit}}permit{{else}}deny{{end}} {{.Seq}}
{{- if .PrefixList }}
 match ip address {{.PrefixList}}
{{- end }}
 exit
{{- end }}

{{- range .Routers }}
router bgp {{$.ASN}}{{if .VRF}} vrf {{.VRF}}{{end}}
 bgp router-id {{.RouterID}}
 bgp log-neighbor-changes
 bgp graceful-restart
 no bgp ebgp-requires-policy
 no bgp network import-check
 no bgp default ipv4-unicast

 {{- range .Peers }}
  neighbor {{.PeerIP}} remote-as {{.PeerASN}}
 {{- if and .Password (ne .Password "") }}
  neighbor {{.PeerIP}} password {{.Password}}
 {{- end }}
 {{- if .Description }}
  neighbor {{.PeerIP}} description "{{.Description}}"
 {{- end }}
 {{- if .RouteMapIn }}
  neighbor {{.PeerIP}} route-map {{.RouteMapIn}} in
 {{- end }}
 {{- if .RouteMapOut }}
  neighbor {{.PeerIP}} route-map {{.RouteMapOut}} out
 {{- end }}

  address-family ipv4 unicast
   neighbor {{.PeerIP}} activate
  exit-address-family

{{- if .Fabric}}
  address-family l2vpn evpn
   neighbor {{.PeerIP}} activate
   advertise-all-vni
   advertise-svi-ip
  exit-address-family
{{- end }}
 {{- end }}

exit
{{- end }}

{{- if .EVPNs }}
{{- range .EVPNs }}
router bgp {{$.ASN}} vrf {{.VRF}}
 address-family l2vpn evpn
  advertise ipv4 unicast
 exit-address-family
exit
{{- end }}
{{- end }}
`

func updateVRFs(vrfs []VRFConfig, routers []RouterConfig) error {
	for _, vrf := range vrfs {
		fmt.Printf("Configuring VRF: %s with VNI: %d\n", vrf.Name, vrf.VNI)

		// Find the RouterID for this VRF
		var routerID string
		for _, router := range routers {
			if router.VRF == vrf.Name {
				routerID = router.RouterID
				break
			}
		}

		// Check if the VRF exists
		checkCmd := exec.Command("ip", "link", "show", vrf.Name)
		output, err := checkCmd.CombinedOutput()
		if err == nil && strings.Contains(string(output), vrf.Name) {
			fmt.Printf("VRF %s already exists, bringing it up\n", vrf.Name)
			cmd := exec.Command("ip", "link", "set", vrf.Name, "up")
			if err := cmd.Run(); err != nil {
				fmt.Printf("Error bringing up VRF %s: %v\n", vrf.Name, err)
				return err
			}
			// Assign IP if RouterID exists
			if routerID != "" {
				cmd := exec.Command("ip", "addr", "add", routerID+"/24", "dev", vrf.Name)
				if err := cmd.Run(); err != nil {
					fmt.Printf("Error assigning IP %s to VRF %s: %v\n", routerID, vrf.Name, err)
					return err
				}
			}
			continue
		}

		// Create VRF if it does not exist
		fmt.Printf("Creating VRF %s\n", vrf.Name)
		cmd := exec.Command("ip", "link", "add", vrf.Name, "type", "vrf", "table", fmt.Sprintf("%d", vrf.VNI))
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error creating VRF %s: %v\n", vrf.Name, err)
			return err
		}

		// Bring VRF up
		cmd = exec.Command("ip", "link", "set", vrf.Name, "up")
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error bringing up VRF %s: %v\n", vrf.Name, err)
			return err
		}

		// Assign IP if RouterID exists
		if routerID != "" {
			cmd := exec.Command("ip", "addr", "add", routerID+"/24", "dev", vrf.Name)
			if err := cmd.Run(); err != nil {
				fmt.Printf("Error assigning IP %s to VRF %s: %v\n", routerID, vrf.Name, err)
				return err
			}
		}
	}
	return nil
}

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

	// This is a lifehack since vtysh.conf is required for FRR to start but its not present
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
				RouterID: "172.30.1.5",
				Peers: []NeighborConfig{
					{PeerIP: "172.30.1.1", PeerASN: "64513", Description: "spine1", Fabric: true},
				},
			},
			{
				VRF:      "hedge",
				RouterID: "192.168.1.1",
				Peers: []NeighborConfig{
					{PeerIP: "192.168.1.2", PeerASN: "64513", Description: "hedge's friend"},
					{PeerIP: "192.168.1.3", PeerASN: "64514", Password: "nothedge"},
				},
			},
			{
				VRF:      "hog",
				RouterID: "192.168.2.1",
				Peers: []NeighborConfig{
					{PeerIP: "192.168.2.2", PeerASN: "64515", Description: "hog's friend", RouteMapIn: "test", RouteMapOut: "test"},
				},
			},
		},
		VRFs: []VRFConfig{
			{Name: "hedge", VNI: 100,
				StaticRoutes: []StaticRouteConfig{
					{VRF: "hedge", Destination: "0.0.0.0/0", NextHop: "192.168.1.3"},
				}},
			{Name: "hog", VNI: 200},
		},
		PrefixLists: []PrefixListConfig{
			{Name: "test", Seq: 10, Permit: true, Prefix: "10.10.0.0/16"},
		},
		RouteMaps: []RouteMapConfig{
			{Name: "test", Permit: true, Seq: 10, PrefixList: "test", MatchType: "ip"},
		},
		EVPNs: []EVPNConfig{
			{VRF: "hedge"},
			{VRF: "hog"},
		},
	}

	// Generate FRR config and reload FRR periodically
	for {
		// Update VRFs
		err := updateVRFs(globalConfig.VRFs, globalConfig.Routers)
		if err != nil {
			fmt.Println("Error updating VRFs:", err)
		}

		err = generateFRRConfig(globalConfig, "/etc/frr/frr.conf")
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
