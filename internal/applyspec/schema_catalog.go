package applyspec

import _ "embed"

var (
	//go:embed schemas/ticket-apply-v1.schema.json
	ticketSchemaV1 []byte
	//go:embed schemas/backlog-apply-v1.schema.json
	backlogSchemaV1 []byte
)

func TicketSchemaJSON() []byte {
	out := make([]byte, len(ticketSchemaV1))
	copy(out, ticketSchemaV1)
	return out
}

func BacklogSchemaJSON() []byte {
	out := make([]byte, len(backlogSchemaV1))
	copy(out, backlogSchemaV1)
	return out
}
