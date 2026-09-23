package library

import (
	"fmt"
	"strings"
	"time"
)

// BookList is a user-curated, ordered reference list. ItemIDs are references
// into the catalog, so a title can safely appear in many recommendation lists.
// ItemTags are list-specific labels, not metadata copied onto the book.
type BookList struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	ItemIDs     []string            `json:"itemIDs"`
	ItemTags    map[string][]string `json:"itemTags,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

func cloneBookList(in BookList) BookList {
	out := in
	if in.Tags != nil {
		out.Tags = append([]string{}, in.Tags...)
	}
	if in.ItemIDs != nil {
		out.ItemIDs = append([]string{}, in.ItemIDs...)
	}
	if in.ItemTags != nil {
		out.ItemTags = make(map[string][]string, len(in.ItemTags))
		for id, tags := range in.ItemTags {
			out.ItemTags[id] = append([]string(nil), tags...)
		}
	}
	return out
}

func normalizeListText(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	return out
}

func newBookList(name, description string, tags []string) BookList {
	now := time.Now().UTC()
	return BookList{
		ID: fmt.Sprintf("list-%d", now.UnixNano()), Name: strings.TrimSpace(name),
		Description: strings.TrimSpace(description), Tags: normalizeListText(tags),
		ItemIDs: []string{}, ItemTags: map[string][]string{}, CreatedAt: now, UpdatedAt: now,
	}
}

// Lists returns private copies of all curated lists.
func (s *Store) Lists() []BookList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]BookList, len(s.lists))
	for i, list := range s.lists {
		out[i] = cloneBookList(list)
	}
	return out
}

func (s *Store) CreateList(name, description string, tags []string) (BookList, bool) {
	if strings.TrimSpace(name) == "" {
		return BookList{}, false
	}
	list := newBookList(name, description, tags)
	s.mu.Lock()
	s.lists = append(s.lists, list)
	s.gen++
	s.mu.Unlock()
	return cloneBookList(list), true
}

func (s *Store) UpdateList(id, name, description string, tags []string) (BookList, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.lists {
		if s.lists[i].ID != id {
			continue
		}
		if strings.TrimSpace(name) != "" {
			s.lists[i].Name = strings.TrimSpace(name)
		}
		s.lists[i].Description = strings.TrimSpace(description)
		s.lists[i].Tags = normalizeListText(tags)
		s.lists[i].UpdatedAt = time.Now().UTC()
		s.gen++
		return cloneBookList(s.lists[i]), true
	}
	return BookList{}, false
}

func (s *Store) DeleteList(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.lists {
		if s.lists[i].ID == id {
			s.lists = append(s.lists[:i], s.lists[i+1:]...)
			s.gen++
			return true
		}
	}
	return false
}

func (s *Store) AddListItem(listID, itemID string, position int, tags []string) (BookList, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[itemID]; !ok {
		return BookList{}, false
	}
	for i := range s.lists {
		list := &s.lists[i]
		if list.ID != listID {
			continue
		}
		if list.ItemTags == nil {
			list.ItemTags = map[string][]string{}
		}
		for _, existing := range list.ItemIDs {
			if existing == itemID {
				if len(tags) > 0 {
					list.ItemTags[itemID] = normalizeListText(tags)
				}
				list.UpdatedAt = time.Now().UTC()
				s.gen++
				return cloneBookList(*list), true
			}
		}
		if position < 0 || position > len(list.ItemIDs) {
			position = len(list.ItemIDs)
		}
		list.ItemIDs = append(list.ItemIDs, "")
		copy(list.ItemIDs[position+1:], list.ItemIDs[position:])
		list.ItemIDs[position] = itemID
		if len(tags) > 0 {
			list.ItemTags[itemID] = normalizeListText(tags)
		}
		list.UpdatedAt = time.Now().UTC()
		s.gen++
		return cloneBookList(*list), true
	}
	return BookList{}, false
}

func (s *Store) RemoveListItem(listID, itemID string) (BookList, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.lists {
		list := &s.lists[i]
		if list.ID != listID {
			continue
		}
		for j, existing := range list.ItemIDs {
			if existing == itemID {
				list.ItemIDs = append(list.ItemIDs[:j], list.ItemIDs[j+1:]...)
				delete(list.ItemTags, itemID)
				list.UpdatedAt = time.Now().UTC()
				s.gen++
				return cloneBookList(*list), true
			}
		}
	}
	return BookList{}, false
}

func (s *Store) ReorderList(listID string, itemIDs []string) (BookList, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.lists {
		list := &s.lists[i]
		if list.ID != listID {
			continue
		}
		allowed := map[string]bool{}
		for _, id := range list.ItemIDs {
			allowed[id] = true
		}
		ordered := make([]string, 0, len(list.ItemIDs))
		seen := map[string]bool{}
		for _, id := range itemIDs {
			if allowed[id] && !seen[id] {
				ordered = append(ordered, id)
				seen[id] = true
			}
		}
		for _, id := range list.ItemIDs {
			if !seen[id] {
				ordered = append(ordered, id)
			}
		}
		list.ItemIDs = ordered
		list.UpdatedAt = time.Now().UTC()
		s.gen++
		return cloneBookList(*list), true
	}
	return BookList{}, false
}
