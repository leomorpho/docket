package store

import (
	"context"

	"github.com/leoaudibert/docket/internal/ticket"
)

// Filter specifies which tickets to return from ListTickets.
type Filter struct {
	States          []ticket.State // empty = all non-archived
	Labels          []string       // empty = no filter
	MaxPriority     int            // 0 = no filter; N = only priority <= N
	OnlyUnblocked   bool
	IncludeArchived bool
}

// Backend is the interface all storage implementations must satisfy.
// The local markdown store is the default. Future: JIRA, Linear, Confluence.
type Backend interface {
	// CreateTicket persists a new ticket. ID and Seq must already be set.
	CreateTicket(ctx context.Context, t *ticket.Ticket) error

	// UpdateTicket overwrites an existing ticket's mutable fields.
	// Implementations must preserve comments and append-only sections.
	UpdateTicket(ctx context.Context, t *ticket.Ticket) error

	// GetTicket returns a single ticket by ID. Returns nil, nil if not found.
	GetTicket(ctx context.Context, id string) (*ticket.Ticket, error)

	// ListTickets returns tickets matching the filter, sorted by priority then created_at.
	ListTickets(ctx context.Context, f Filter) ([]*ticket.Ticket, error)

	// AddComment appends a comment to a ticket. Never edits existing content.
	AddComment(ctx context.Context, id string, c ticket.Comment) error

	// LinkCommit appends a commit SHA to a ticket's linked_commits.
	LinkCommit(ctx context.Context, id string, sha string) error

	// NextID generates the next sequential ticket ID. Mutates the counter.
	NextID(ctx context.Context) (id string, seq int, err error)

	// Validate checks whether a ticket is schema-valid for this backend.
	// For local: checks markdown structure. For remote: checks API constraints.
	Validate(ctx context.Context, id string) ([]ValidationError, error)
}

// ValidationError describes a single schema violation found in a ticket.
type ValidationError struct {
	Field   string // e.g. "state", "blocked_by[0]", "frontmatter"
	Message string // human-readable description
}
