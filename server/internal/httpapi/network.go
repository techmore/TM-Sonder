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
		"message":     "Rebinding web + API (Caddy optional)…",
	})
}

// handleNetworkExposure applies interface and port changes as one atomic
// operation. It returns before the managed process restart so a web client can
// keep polling /api/network/status while the listeners move.
func (s *Server) handleNetworkExposure(w http.ResponseWriter, r *http.Request) {
	if s.runtimeControl == nil {
		writeError(w, http.StatusServiceUnavailable, "network controller is unavailable")
		return
	}
	var request struct {
		Mode        string `json:"mode"`
		InterfaceID string `json:"interfaceID"`
		WebPort     *int   `json:"webPort"`
		APIPort     *int   `json:"apiPort"`
	}
	if err := jsonDecode(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid network exposure request")
		return
	}
	if request.WebPort != nil && (*request.WebPort < 1 || *request.WebPort > 65535) {
		writeError(w, http.StatusBadRequest, "webPort must be between 1 and 65535")
		return
	}
	if request.APIPort != nil && (*request.APIPort < 1 || *request.APIPort > 65535) {
		writeError(w, http.StatusBadRequest, "apiPort must be between 1 and 65535")
		return
	}
	mode := network.BindingMode(strings.TrimSpace(request.Mode))
	interfaceID := strings.TrimSpace(request.InterfaceID)
	if mode == "" {
		if interfaceID != "" {
			mode = network.ModeExact
		}
	} else if interfaceID == "" && !isNamedBindingMode(mode) {
		// Accept the same concise exact-ID form as /api/network/rebind.
		interfaceID = string(mode)
		mode = network.ModeExact
	}
	webPort, apiPort := 0, 0
	if request.WebPort != nil {
		webPort = *request.WebPort
	}
	if request.APIPort != nil {
		apiPort = *request.APIPort
	}
	target, err := s.runtimeControl.PlanExposure(r.Context(), mode, interfaceID, webPort, apiPort)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	go func() {
		_, err := s.runtimeControl.SwitchExposure(context.Background(), mode, interfaceID, webPort, apiPort)
		if err != nil {
			s.logger.Printf("network exposure change failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":  true,
		"target":    target,
		"statusURL": "/api/network/status",
		"message":   "Applying network exposure; listeners will reconnect when healthy",
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
