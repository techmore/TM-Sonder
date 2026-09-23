package httpapi

import (
	"context"
	"net/http"
	"strings"

	"tm-sonder/server/internal/network"
)

func (s *Server) handleNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	if s.runtimeControl == nil {
		items, err := network.Enumerate()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"interfaces": items})
		return
	}
	items, err := s.runtimeControl.InterfacesForAPI()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"interfaces": items})
}

func (s *Server) handleNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if s.runtimeControl == nil {
		writeError(w, http.StatusServiceUnavailable, "network controller is unavailable")
		return
	}
	status, err := s.runtimeControl.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version,
		"build":   Build,
		"status":  status,
	})
}

func (s *Server) handleNetworkRebind(w http.ResponseWriter, r *http.Request) {
	if s.runtimeControl == nil {
		writeError(w, http.StatusServiceUnavailable, "network controller is unavailable")
		return
	}
	var request struct {
		Mode        string `json:"mode"`
		InterfaceID string `json:"interfaceID"`
	}
	if err := jsonDecode(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid network binding request")
		return
	}
	mode := network.BindingMode(strings.TrimSpace(request.Mode))
	if mode == "" {
		writeError(w, http.StatusBadRequest, "mode is required")
		return
	}
	// The menu and CLI may send an exact interface ID as the mode shorthand,
	// e.g. {"mode":"utun4"}. Named modes use interfaceID only when the user
	// selected a particular member of that category.
	if request.InterfaceID == "" && !isNamedBindingMode(mode) {
		request.InterfaceID = string(mode)
		mode = network.ModeExact
	}
	if err := s.runtimeControl.ValidateBinding(r.Context(), mode, request.InterfaceID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	go func() {
		_, err := s.runtimeControl.Switch(context.Background(), mode, request.InterfaceID)
		if err != nil {
			s.logger.Printf("network rebind failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":    true,
		"mode":        mode,
		"interfaceID": request.InterfaceID,
		"message":     "Rebinding web + Caddy…",
	})
}

func (s *Server) handleNetworkProxy(w http.ResponseWriter, r *http.Request) {
	if s.runtimeControl == nil {
		writeError(w, http.StatusServiceUnavailable, "network controller is unavailable")
		return
	}
	var request struct {
		Enabled          *bool  `json:"enabled"`
		Domain           string `json:"publicDomain"`
		HealthURL        string `json:"publicHealthCheckURL"`
		CaddyBindAddress string `json:"caddyBindAddress"`
	}
	if err := jsonDecode(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid proxy settings request")
		return
	}
	if request.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	state, err := s.runtimeControl.ConfigureProxy(r.Context(), *request.Enabled, request.Domain, request.HealthURL, request.CaddyBindAddress)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": state})
}

func isNamedBindingMode(mode network.BindingMode) bool {
	switch mode {
	case network.ModeLoopback, network.ModeWiFiLAN, network.ModeEthernet, network.ModeVPN, network.ModePublic, network.ModeExact:
		return true
	default:
		return false
	}
}
