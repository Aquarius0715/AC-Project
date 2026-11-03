package maintenance

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// ---- attachments.list: the attachments an MRV report may cite (DD-A14 item 6, IR203, IR349) ----

// EvidenceAttachment is EvidenceAttachment: a ready attachment of an accepted work report, with the job's unit and
// the report's acceptance time.
type EvidenceAttachment struct {
	AttachmentRef

	UnitID     uuid.UUID `json:"unitId"`
	AcceptedAt time.Time `json:"acceptedAt"`
}

// ListAttachmentsInput is attachments.list input.
type ListAttachmentsInput struct {
	UnitIDs []uuid.UUID  `json:"unitIds"`
	From    time.Time    `json:"from"`
	To      time.Time    `json:"to"`
	Query   paging.Query `json:"query"`
}

// Validate implements ops.Validator: 1–500 distinct units and a period [from, to) of at most 366 days.
func (in *ListAttachmentsInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	seen := map[uuid.UUID]bool{}

	valid := len(in.UnitIDs) >= 1 && len(in.UnitIDs) <= 500
	for _, u := range in.UnitIDs {
		valid = valid && u != uuid.Nil && !seen[u]
		seen[u] = true
	}

	if !valid {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	switch {
	case in.From.IsZero():
		fieldErrs["from"] = apperr.KeyRequired
	case in.To.IsZero():
		fieldErrs["to"] = apperr.KeyRequired
	case !in.To.After(in.From) || in.To.Sub(in.From) > 366*24*time.Hour:
		fieldErrs["to"] = apperr.KeyRange
	}

	return fieldErrs
}

// list returns the ready attachments of accepted work reports on the units, uploaded in [from, to): the evidence an MRV
// report may cite (mrv.saveDraft keeps 0–20 of them). Newest first by default.
//
//	@Summary		attachments.list (read)
//	@ID				attachments.list
//	@Description	Authorization: admin:mrv.write | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR349 unitIds \
//	@Description	1–500 distinct, from < to within 366 days; ready attachments of accepted work reports uploaded in \
//	@Description	[from, to)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A14 · Query: filters none · sort id,createdAt,name (default createdAt desc;id asc)
//	@Tags			attachments
//	@Accept			json
//	@Produce		json
//	@Param			unitIds	query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			from	query		string		false	"input field from"
//	@Param			to		query		string		false	"input field to"
//	@Param			cursor	query		string		false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer		false	"page size 1–100, default 25"
//	@Param			sort	query		string		false	"field:direction — fields id,createdAt,name; default createdAt desc;id asc"
//	@Success		200		{object}	ops.Envelope{data=EvidenceAttachmentPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/attachments [get]
func (m Files) list(
	ctx context.Context,
	call *ops.Call,
	input *ListAttachmentsInput,
) (paging.Page[EvidenceAttachment], error) {
	err := paging.NoFilters(input.Query, "query.filters")
	if err != nil {
		return paging.Page[EvidenceAttachment]{}, apperr.Wrap(err, "paging.NoFilters")
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT a.id, a.tenant_id, a.created_at, a.updated_at, a.job_id, a.report_id, a.name, a.mime, a.size_bytes, a.status,
			j.unit_id, r.accepted_at
		FROM maintenance.attachments a
		JOIN maintenance.work_reports r ON r.id = a.report_id AND r.state = 'accepted'
		JOIN maintenance.jobs j ON j.id = a.job_id
		WHERE a.status = 'ready' AND j.unit_id = ANY($1) AND a.created_at >= $2 AND a.created_at < $3`,
		input.UnitIDs,
		input.From,
		input.To,
	)
	if err != nil {
		return paging.Page[EvidenceAttachment]{}, apperr.Wrap(err, "select maintenance.attachments")
	}

	all := []EvidenceAttachment{}

	for rows.Next() {
		var evidenceAttachment EvidenceAttachment

		err := rows.Scan(
			&evidenceAttachment.ID,
			&evidenceAttachment.TenantID,
			&evidenceAttachment.CreatedAt,
			&evidenceAttachment.UpdatedAt,
			&evidenceAttachment.JobID,
			&evidenceAttachment.ReportID,
			&evidenceAttachment.Name,
			&evidenceAttachment.Mime,
			&evidenceAttachment.Size,
			&evidenceAttachment.Status,
			&evidenceAttachment.UnitID,
			&evidenceAttachment.AcceptedAt,
		)
		if err != nil {
			rows.Close()

			return paging.Page[EvidenceAttachment]{}, apperr.Wrap(
				err,
				"select maintenance.attachments",
			)
		}

		evidenceAttachment.Version, evidenceAttachment.BlobID = 1, evidenceAttachment.ID
		all = append(all, evidenceAttachment)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[EvidenceAttachment]{}, apperr.Wrap(
			rowsErr,
			"select maintenance.attachments",
		)
	}

	sort := input.Query.Sort
	if sort == nil {
		sort = &paging.SortSpec{Field: "createdAt", Direction: "desc"}
	}

	sortSliceErr := paging.SortSlice(all, sort, map[string]func(x, y EvidenceAttachment) int{
		"id":        func(x, y EvidenceAttachment) int { return strings.Compare(x.ID.String(), y.ID.String()) },
		"createdAt": func(x, y EvidenceAttachment) int { return x.CreatedAt.Compare(y.CreatedAt) },
		"name": func(x, y EvidenceAttachment) int {
			return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		},
	})
	if sortSliceErr != nil {
		return paging.Page[EvidenceAttachment]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	window, err := paging.Resolve(input.Query, struct {
		U    []uuid.UUID
		F, T time.Time
	}{input.UnitIDs, input.From, input.To}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[EvidenceAttachment]{}, apperr.Wrap(err, "paging.Resolve")
	}

	items := []EvidenceAttachment{}
	for i := window.Offset; i < len(all) && i < window.Offset+window.Limit; i++ {
		items = append(items, all[i])
	}

	return paging.Page[EvidenceAttachment]{
		Items:           items,
		NextCursor:      window.Next(len(all)),
		Total:           len(all),
		SnapshotVersion: window.Snapshot,
	}, nil
}
