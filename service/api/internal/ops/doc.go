package ops

import "time"

// Envelope describes the {data, meta} body of every successful operation for the API description (the swag
// annotations of the handlers say `ops.Envelope{data=…}`); the pipeline writes Result with the same shape.
type Envelope struct {
	Data any  `json:"data"`
	Meta Meta `json:"meta"`
}

// Meta is Meta of service-contracts.ts plus the operation name.
type Meta struct {
	Operation     string    `example:"jobs.get"                             json:"operation"`
	CorrelationID string    `example:"01a11edc-43b0-7502-beb5-52e1c0365354" json:"correlationId"`
	SnapshotAt    time.Time `                                               json:"snapshotAt"`  // the business clock of the call (the demo scenario clock in the demo environment)
	EventCursor   int64     `                                               json:"eventCursor"` // the tenant's event position of this snapshot (D07)
}
