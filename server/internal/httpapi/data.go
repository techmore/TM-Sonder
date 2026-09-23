package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"tm-sonder/server/internal/config"
	"tm-sonder/server/internal/library"
)

const maxBundleBody = 512 << 20

type dataImportRequest struct {
	// Bundle allows callers to wrap the exported document as {"bundle": ...}
	// while the flattened fields keep the raw export directly importable.
	Bundle       *library.DataBundle `json:"bundle,omitempty"`
	Format       string              `json:"format"`
	Version      int                 `json:"version"`
	ExportedAt   time.Time           `json:"exportedAt"`
	Libraries    []config.Library    `json:"libraries"`
	Snapshot     library.Snapshot    `json:"snapshot"`
	Mode         string              `json:"mode"`
	PathMappings map[string]string   `json:"pathMappings"`
}

func (s *Server) handleDataExport(w http.ResponseWriter, r *http.Request) {
	bundle := s.store.ExportBundle(s.cfg().Libraries)
	filename := fmt.Sprintf("sonder-data-%s.json", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	if err := json.NewEncoder(w).Encode(bundle); err != nil {
		s.logger.Printf("data export failed: %v", err)
	}
}

func (s *Server) handleDataImport(w http.ResponseWriter, r *http.Request) {
	var req dataImportRequest
	if err := decodeLargeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Sonder data bundle: "+err.Error())
		return
	}
	bundle := library.DataBundle{
		Format:     req.Format,
		Version:    req.Version,
		ExportedAt: req.ExportedAt,
		Libraries:  req.Libraries,
		Snapshot:   req.Snapshot,
	}
	if req.Bundle != nil {
		bundle = *req.Bundle
	}
	pathsRemapped, err := library.RemapBundlePaths(&bundle, s.cfg().Libraries, req.PathMappings)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid Sonder data bundle: "+err.Error())
		return
	}
	result, err := s.store.ImportBundle(bundle, req.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Could not import Sonder data: "+err.Error())
		return
	}
	result.PathsRemapped = pathsRemapped
	s.mutationSaved()
	writeJSON(w, http.StatusOK, map[string]any{
		"imported":    result,
		"scanStarted": false,
		"message":     "Catalog data imported. Media was not rescanned; run an explicit scan when you want to reconcile files.",
	})
}

func decodeLargeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBundleBody))
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing data after JSON body")
	}
	return nil
}
