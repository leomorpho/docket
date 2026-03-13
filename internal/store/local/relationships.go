package local

import (
	"context"
	"fmt"
	"sort"

	"github.com/leomorpho/docket/internal/store"
	"github.com/leomorpho/docket/internal/ticket"
)

// RelationshipIndex is a runtime-only parent/child graph derived from ticket data.
// No children are persisted in markdown.
type RelationshipIndex struct {
	ByID      map[string]*ticket.Ticket
	Children  map[string][]*ticket.Ticket
	DescDepth map[string]int
}

// InvalidateRelationshipIndex clears the cached relationship index.
func (s *Store) InvalidateRelationshipIndex() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relIdx = nil
}

// BuildRelationshipIndex loads tickets and constructs a reverse parent→children map.
// This is cached in memory.
func (s *Store) BuildRelationshipIndex(ctx context.Context) (*RelationshipIndex, error) {
	s.mu.RLock()
	if s.relIdx != nil {
		defer s.mu.RUnlock()
		return s.relIdx, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Double-check if it was built while we were waiting for the lock
	if s.relIdx != nil {
		return s.relIdx, nil
	}

	all, err := s.ListTickets(ctx, store.Filter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}

	idx := &RelationshipIndex{
		ByID:      make(map[string]*ticket.Ticket, len(all)),
		Children:  make(map[string][]*ticket.Ticket),
		DescDepth: make(map[string]int, len(all)),
	}
	for _, t := range all {
		idx.ByID[t.ID] = t
	}
	for _, t := range all {
		if t.Parent == "" {
			continue
		}
		if _, ok := idx.ByID[t.Parent]; !ok {
			continue
		}
		idx.Children[t.Parent] = append(idx.Children[t.Parent], t)
	}
	for parent := range idx.Children {
		sort.Slice(idx.Children[parent], func(i, j int) bool {
			a, b := idx.Children[parent][i], idx.Children[parent][j]
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return a.CreatedAt.Before(b.CreatedAt)
		})
	}

	s.relIdx = idx
	return idx, nil
}

// Descendants returns all descendants of id in depth-first order.
func (idx *RelationshipIndex) Descendants(id string) []*ticket.Ticket {
	var out []*ticket.Ticket
	var walk func(parent string)
	walk = func(parent string) {
		children := idx.Children[parent]
		for _, c := range children {
			out = append(out, c)
			walk(c.ID)
		}
	}
	walk(id)
	return out
}

// ComputeDepth returns the ancestor depth for ticket id, where a root has depth 0.
func (idx *RelationshipIndex) ComputeDepth(id string) int {
	if d, ok := idx.DescDepth[id]; ok {
		return d
	}
	d := 0
	cur := idx.ByID[id]
	for cur != nil && cur.Parent != "" {
		parent, ok := idx.ByID[cur.Parent]
		if !ok {
			break
		}
		d++
		cur = parent
	}
	idx.DescDepth[id] = d
	return d
}

func (s *Store) validateParentRef(ctx context.Context, t *ticket.Ticket) error {
	if t.Parent == "" {
		return nil
	}
	if t.Parent == t.ID {
		return fmt.Errorf("parent cannot reference self")
	}
	parent, err := s.GetTicket(ctx, t.Parent)
	if err != nil {
		return err
	}
	if parent == nil {
		return fmt.Errorf("parent ticket %q does not exist", t.Parent)
	}

	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		return err
	}
	// Include this pending assignment in cycle detection by overriding parent in memory.
	if existing, ok := idx.ByID[t.ID]; ok {
		existing.Parent = t.Parent
	} else {
		cp := *t
		idx.ByID[t.ID] = &cp
	}

	seen := map[string]bool{t.ID: true}
	cur := t.Parent
	for cur != "" {
		if seen[cur] {
			return fmt.Errorf("parent assignment creates a cycle")
		}
		seen[cur] = true
		p, ok := idx.ByID[cur]
		if !ok {
			break
		}
		cur = p.Parent
	}
	return nil
}

func (s *Store) ParentDepth(ctx context.Context, id string) (int, error) {
	idx, err := s.BuildRelationshipIndex(ctx)
	if err != nil {
		return 0, err
	}
	return idx.ComputeDepth(id), nil
}
