// Package network describes the active local network interfaces Sonder may
// bind to. It deliberately works from the addresses currently assigned to an
// interface: a configured-but-disconnected adapter is never offered as a
// binding target.
package network

import (
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
)

type BindingMode string

const (
	ModeLoopback BindingMode = "loopback"
	ModeWiFiLAN  BindingMode = "wifi/lan"
	ModeEthernet BindingMode = "ethernet"
	ModeVPN      BindingMode = "vpn"
	ModePublic   BindingMode = "public"
	ModeExact    BindingMode = "interface"
)

type InterfaceType string

const (
	TypeLoopback       InterfaceType = "loopback"
	TypeWiFiLAN        InterfaceType = "wifi/lan"
	TypeEthernet       InterfaceType = "ethernet"
	TypeVPN            InterfaceType = "vpn"
	TypeUSBThunderbolt InterfaceType = "usb/thunderbolt"
	TypeOther          InterfaceType = "other"
)

// Interface is the JSON-safe description shown by the CLI, menu bar, and
// network API. IPv4 is the address Sonder would use as an upstream target.
type Interface struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Type         InterfaceType `json:"type"`
	HardwarePort string        `json:"hardwarePort,omitempty"`
	IPv4         string        `json:"ipv4"`
	Up           bool          `json:"up"`
	Loopback     bool          `json:"loopback"`
	Active       bool          `json:"active"`
	BindingModes []BindingMode `json:"bindingModes"`
}

type Binding struct {
	Mode           BindingMode `json:"mode"`
	InterfaceID    string      `json:"interfaceID"`
	IPv4           string      `json:"ipv4"`
	WebBindAddress string      `json:"webBindAddress"`
	APIBindAddress string      `json:"apiBindAddress"`
}

// Enumerate returns only interfaces that are up and have at least one IPv4
// address. Loopback is retained intentionally so the secure default can be
// selected explicitly and tested without a LAN.
func Enumerate() ([]Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate network interfaces: %w", err)
	}
	return collect(ifs, func(name string) ([]net.Addr, error) {
		for _, item := range ifs {
			if item.Name == name {
				return item.Addrs()
			}
		}
		return nil, fmt.Errorf("interface %q disappeared", name)
	}), nil
}

// collect is kept separate from Enumerate so tests can exercise interface
// selection without depending on the host's Wi-Fi or VPN state.
func collect(ifs []net.Interface, addrs func(string) ([]net.Addr, error)) []Interface {
	ports := hardwarePorts()
	out := make([]Interface, 0, len(ifs))
	for _, ni := range ifs {
		if ni.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, err := addrs(ni.Name)
		if err != nil {
			continue
		}
		ip := firstIPv4(addresses)
		if ip == "" {
			continue
		}
		kind := classify(ni.Name, ni.Flags, ports[ni.Name])
		item := Interface{
			ID:           ni.Name,
			Name:         displayName(ni.Name, ports[ni.Name]),
			Type:         kind,
			HardwarePort: ports[ni.Name],
			IPv4:         ip,
			Up:           true,
			Loopback:     kind == TypeLoopback,
			Active:       true,
		}
		item.BindingModes = modesFor(kind)
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return typeRank(out[i].Type) < typeRank(out[j].Type)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func firstIPv4(addrs []net.Addr) string {
	for _, addr := range addrs {
		var ip net.IP
		switch value := addr.(type) {
		case *net.IPNet:
			ip = value.IP
		case *net.IPAddr:
			ip = value.IP
		default:
			text := addr.String()
			if host, _, err := net.ParseCIDR(text); err == nil {
				ip = host
			} else if parsed := net.ParseIP(text); parsed != nil {
				ip = parsed
			}
		}
		if ip4 := ip.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

func classify(id string, flags net.Flags, hardware string) InterfaceType {
	name := strings.ToLower(strings.TrimSpace(id))
	port := strings.ToLower(strings.TrimSpace(hardware))
	if flags&net.FlagLoopback != 0 || strings.HasPrefix(name, "lo") {
		return TypeLoopback
	}
	if strings.HasPrefix(name, "utun") || strings.HasPrefix(name, "tun") || strings.HasPrefix(name, "tap") || strings.HasPrefix(name, "ppp") {
		return TypeVPN
	}
	if strings.Contains(port, "usb") || strings.Contains(port, "thunderbolt") {
		return TypeUSBThunderbolt
	}
	if strings.Contains(port, "wi-fi") || strings.Contains(port, "wifi") || strings.Contains(port, "airport") {
		return TypeWiFiLAN
	}
	if strings.Contains(port, "ethernet") || strings.Contains(port, "lan") || strings.HasPrefix(name, "bridge") {
		return TypeEthernet
	}
	// macOS normally exposes Wi-Fi as en0 and wired adapters as en1/en2.
	// Without networksetup metadata, en* is still a useful LAN target; the
	// exact interface ID remains available when the user needs precision.
	if strings.HasPrefix(name, "en") || strings.HasPrefix(name, "awdl") || strings.HasPrefix(name, "llw") {
		return TypeWiFiLAN
	}
	return TypeOther
}

func displayName(id, hardware string) string {
	if strings.TrimSpace(hardware) == "" {
		return id
	}
	return hardware
}

func modesFor(kind InterfaceType) []BindingMode {
	switch kind {
	case TypeLoopback:
		return []BindingMode{ModeLoopback, ModeExact}
	case TypeWiFiLAN:
		return []BindingMode{ModeWiFiLAN, ModeExact, ModePublic}
	case TypeEthernet, TypeUSBThunderbolt:
		return []BindingMode{ModeEthernet, ModeExact, ModePublic}
	case TypeVPN:
		return []BindingMode{ModeVPN, ModeExact, ModePublic}
	default:
		return []BindingMode{ModeExact, ModePublic}
	}
}

func typeRank(kind InterfaceType) int {
	switch kind {
	case TypeLoopback:
		return 0
	case TypeWiFiLAN:
		return 1
	case TypeEthernet, TypeUSBThunderbolt:
		return 2
	case TypeVPN:
		return 3
	default:
		return 4
	}
}

// hardwarePorts uses macOS's built-in networksetup when available. Failure is
// harmless: interface IDs and IPv4 addresses remain fully usable, and the
// deterministic name-prefix fallback classifies the common adapters.
func hardwarePorts() map[string]string {
	result := map[string]string{}
	cmd := exec.Command("networksetup", "-listallhardwareports")
	data, err := cmd.Output()
	if err != nil {
		return result
	}
	var current string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Hardware Port:"):
			current = strings.TrimSpace(strings.TrimPrefix(line, "Hardware Port:"))
		case strings.HasPrefix(line, "Device:") && current != "":
			device := strings.TrimSpace(strings.TrimPrefix(line, "Device:"))
			result[device] = current
			current = ""
		}
	}
	return result
}
