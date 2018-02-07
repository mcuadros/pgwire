package pgwire

import (
	"context"
)

// StatementResult is used to produce results for a single query (see
// ResultsWriter).
type StatementResult interface {
	// BeginResult should be called prior to any of the other methods.
	// TODO(andrei): remove BeginResult and SetColumns, and have
	// NewStatementResult() take in a tree.Statement
	BeginResult(typ StatementType, tag StatementTag)
	// GetPGTag returns the PGTag of the statement passed into BeginResult.
	StatementTag() StatementTag
	// GetStatementType returns the StatementType that corresponds to the type of
	// results that should be sent to this interface.
	StatementType() StatementType
	// SetColumns should be called after BeginResult and before AddRow if the
	// StatementType is tree.Rows.
	SetColumns(columns ResultColumns)
	// AddRow takes the passed in row and adds it to the current result.
	AddRow(ctx context.Context, row Datums) error
	// IncrementRowsAffected increments a counter by n. This is used for all
	// result types other than tree.Rows.
	IncrementRowsAffected(n int)
	// RowsAffected returns either the number of times AddRow was called, or the
	// sum of all n passed into IncrementRowsAffected.
	RowsAffected() int
	// CloseResult ends the current result. The v3Conn will send control codes to
	// the client informing it that the result for a statement is now complete.
	//
	// CloseResult cannot be called unless there's a corresponding BeginResult
	// prior.
	CloseResult() error
	// SetError allows an error to be stored on the StatementResult.
	SetError(err error)
	// Err returns the error previously set with SetError(), if any.
	Err() error
}
