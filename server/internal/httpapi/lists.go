package httpapi

import (
	"net/http"
	"strings"

	"tm-sonder/server/internal/library"
)

type listMutationRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type listItemRequest struct {
	ItemID   string   `json:"itemID"`
	Position int      `json:"position"`
	Tags     []string `json:"tags"`
}

type listReorderRequest struct {
	ItemIDs []string `json:"itemIDs"`
}

func (s *Server) handleLists(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		lists := s.store.Lists()
		out := make([]map[string]any, 0, len(lists))
		for _, list := range lists {
			out = append(out, s.listResponse(list))
		}
		writeJSON(w, http.StatusOK, map[string]any{"lists": out})
	case http.MethodPost:
		var req listMutationRequest
		if err := jsonDecode(w, r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
			writeError(w, http.StatusBadRequest, "List name is required")
			return
		}
		list, ok := s.store.CreateList(req.Name, req.Description, req.Tags)
		if !ok {
			writeError(w, http.StatusBadRequest, "Could not create list")
			return
		}
		s.mutationSaved()
		writeJSON(w, http.StatusCreated, s.listResponse(list))
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("listID")
	if r.Method == http.MethodPatch {
		var req listMutationRequest
		if err := jsonDecode(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid list update")
			return
		}
		list, ok := s.store.UpdateList(id, req.Name, req.Description, req.Tags)
		if !ok {
			writeError(w, http.StatusNotFound, "List not found")
			return
		}
		s.mutationSaved()
		writeJSON(w, http.StatusOK, s.listResponse(list))
		return
	}
	if r.Method == http.MethodDelete {
		if !s.store.DeleteList(id) {
			writeError(w, http.StatusNotFound, "List not found")
			return
		}
		s.mutationSaved()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("listID")
	if r.Method == http.MethodPost {
		var req listItemRequest
		if err := jsonDecode(w, r, &req); err != nil || req.ItemID == "" {
			writeError(w, http.StatusBadRequest, "itemID is required")
			return
		}
		list, ok := s.store.AddListItem(id, req.ItemID, req.Position, req.Tags)
		if !ok {
			writeError(w, http.StatusNotFound, "List or item not found")
			return
		}
		s.mutationSaved()
		writeJSON(w, http.StatusOK, s.listResponse(list))
		return
	}
	if r.Method == http.MethodDelete {
		list, ok := s.store.RemoveListItem(id, r.PathValue("itemID"))
		if !ok {
			writeError(w, http.StatusNotFound, "List item not found")
			return
		}
		s.mutationSaved()
		writeJSON(w, http.StatusOK, s.listResponse(list))
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
}

func (s *Server) handleListReorder(w http.ResponseWriter, r *http.Request) {
	var req listReorderRequest
	if err := jsonDecode(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid item order")
		return
	}
	list, ok := s.store.ReorderList(r.PathValue("listID"), req.ItemIDs)
	if !ok {
		writeError(w, http.StatusNotFound, "List not found")
		return
	}
	s.mutationSaved()
	writeJSON(w, http.StatusOK, s.listResponse(list))
}

func (s *Server) mutationSaved() {
	if s.onMutation != nil {
		s.onMutation()
	}
}

func (s *Server) listResponse(list library.BookList) map[string]any {
	items := make([]any, 0, len(list.ItemIDs))
	for _, id := range list.ItemIDs {
		if item, ok := s.store.Get(id); ok {
			items = append(items, map[string]any{"item": item.MediaItem, "tags": list.ItemTags[id]})
		}
	}
	return map[string]any{"id": list.ID, "name": list.Name, "description": list.Description, "tags": list.Tags, "itemIDs": list.ItemIDs, "items": items, "createdAt": list.CreatedAt, "updatedAt": list.UpdatedAt}
}
