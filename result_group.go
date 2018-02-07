package pgwire

import "context"

// ResultsGroup is used to produce a result group (see ResultsWriter).
type ResultsGroup interface {
	// NewStatementResult creates a new StatementResult, indicating that future
	// results are part of a new SQL query.
	//
	// A single StatementResult can be active on a ResultGroup at a time; it is
	// illegal to create a new StatementResult before CloseResult() has been
	// called on the previous one.
	NewStatementResult() StatementResult
	// Close should be called once all the results for a group have been produced
	// (i.e. after all StatementResults have been closed). It informs the
	// implementation that the results are not going to be reset any more, and
	// thus can be sent to the client.
	//
	// Close has to be called before new groups are created.  It's illegal to call
	// Close before CloseResult() has been called on all of the group's
	// StatementResults.
	Close()
	// Flush informs the ResultsGroup that the caller relinquishes the capability
	// to Reset() the results that have been already been accumulated on this
	// group. This means that future Reset() calls will only reset up to the
	// current point in the stream - only future results will be discarded. This
	// is used to ensure that some results are always sent to the client even if
	// further statements are retried automatically; it supports the statements
	// run in the AutoRetry state: these statements are not executed again when
	// doing an automatic retry, and so their results shouldn't be reset.
	//
	// It is illegal to call this while any StatementResults on this group are
	// open.
	//
	// Like StatementResult.AddRow(), Flush returns communication errors, if any.
	// TODO(andrei): provide guidance on handling these errors.
	Flush(context.Context) error
	// ResultsSentToClient returns true if any results pertaining to this group
	// beyond the last Flush() point have been sent to the consumer.
	// Remember that the implementation is free to buffer or send results to the
	// client whenever it pleases. This method checks to see if the implementation
	// has in fact sent anything so far.
	//
	// TODO(andrei): add a note about the synchronous nature of the implementation
	// imposed by this interface.
	ResultsSentToClient() bool
	// Reset discards all the accumulated results from the last Flush() call
	// onwards (or from the moment the group was created if Flush() was never
	// called).
	// It is illegal to call Reset if any results have already been sent to the
	// consumer; this can be tested with ResultsSentToClient().
	Reset(context.Context)
}
