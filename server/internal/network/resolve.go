package network

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

func Resolve(mode BindingMode, interfaceID string, interfaces []Interface) (Binding, error) {
	mode = normalizeMode(mode)
	if mode == ModePublic {
		candidate := firstNonLoopback(interfaces)
		if candidate == nil {
			candidate = findType(interfaces, TypeLoopback)
		}
		if candidate == nil {
			return Binding{}, fmt.Errorf("no active IPv4 interface is available for public binding")
		}
		return Binding{
			Mode:           mode,
			InterfaceID:    candidate.ID,
			IPv4:           candidate.IPv4,
			WebBindAddress: "0.0.0.0",
			APIBindAddress: "127.0.0.1",
		}, nil
	}

	if mode == ModeExact {
		interfaceID = strings.TrimSpace(interfaceID)
		if interfaceID == "" {
			return Binding{}, fmt.Errorf("exact interface binding requires an interface ID")
		}
		for _, item := range interfaces {
			if item.ID == interfaceID && item.Active && item.IPv4 != "" {
				return bindingFor(mode, item), nil
			}
		}
		return Binding{}, fmt.Errorf("interface %q is not active or has no IPv4 address", interfaceID)
	}

	var candidates []Interface
	for _, item := range interfaces {
		if !item.Active || item.IPv4 == "" {
			continue
		}
		if matchesMode(mode, item.Type) {
			candidates = append(candidates, item)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	if len(candidates) == 0 {
		return Binding{}, fmt.Errorf("no active IPv4 interface matches binding mode %q", mode)
	}
	if interfaceID != "" {
		for _, item := range candidates {
			if item.ID == interfaceID {
				return bindingFor(mode, item), nil
			}
		}
		return Binding{}, fmt.Errorf("interface %q is not an active %s interface", interfaceID, mode)
	}
	return bindingFor(mode, candidates[0]), nil
}

func ResolveAndApply(state *RuntimeState, interfaces []Interface) error {
	binding, err := Resolve(state.SelectedMode, state.SelectedInterface, interfaces)
	if err != nil {
		return err
	}
	state.SelectedMode = binding.Mode
	state.SelectedInterface = binding.InterfaceID
	state.SelectedIPv4 = binding.IPv4
	state.WebBindAddress = binding.WebBindAddress
	state.APIBindAddress = binding.APIBindAddress
	state.CaddyUpstream = net.JoinHostPort(binding.IPv4, fmt.Sprint(state.WebPort))
	return nil
}

func normalizeMode(mode BindingMode) BindingMode {
	switch strings.ToLower(strings.TrimSpace(string(mode))) {
	case "loopback", "localhost", "local":
		return ModeLoopback
	case "wifi", "wi-fi", "lan", "wifi/lan":
		return ModeWiFiLAN
	case "ethernet", "wired", "usb", "thunderbolt":
		return ModeEthernet
	case "vpn", "wireguard", "utun":
		return ModeVPN
	case "public", "all":
		return ModePublic
	case "interface", "exact":
		return ModeExact
	default:
		// The CLI accepts en0, en1, utun4, bridge0, etc. as exact IDs.
		return ModeExact
	}
}

func matchesMode(mode BindingMode, kind InterfaceType) bool {
	switch mode {
	case ModeLoopback:
		return kind == TypeLoopback
	case ModeWiFiLAN:
		return kind == TypeWiFiLAN
	case ModeEthernet:
		return kind == TypeEthernet || kind == TypeUSBThunderbolt
	case ModeVPN:
		return kind == TypeVPN
	default:
		return false
	}
}

func bindingFor(mode BindingMode, item Interface) Binding {
	address := item.IPv4
	if mode == ModeLoopback {
		address = "127.0.0.1"
	}
	return Binding{
		Mode:           mode,
		InterfaceID:    item.ID,
		IPv4:           item.IPv4,
		WebBindAddress: address,
		APIBindAddress: "127.0.0.1",
	}
}

func firstNonLoopback(interfaces []Interface) *Interface {
	items := append([]Interface(nil), interfaces...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for i := range items {
		if items[i].Active && items[i].IPv4 != "" && !items[i].Loopback && items[i].Type != TypeVPN {
			return &items[i]
		}
	}
	for i := range items {
		if items[i].Active && items[i].IPv4 != "" && !items[i].Loopback {
			return &items[i]
		}
	}
	return nil
}

func findType(interfaces []Interface, kind InterfaceType) *Interface {
	for i := range interfaces {
		if interfaces[i].Active && interfaces[i].IPv4 != "" && interfaces[i].Type == kind {
			return &interfaces[i]
		}
	}
	return nil
}
