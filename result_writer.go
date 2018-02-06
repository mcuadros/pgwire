package pgwire

import (
	"context"
)

// ResultsWriter is the interface used to by the Executor to produce results for
// query execution for a SQL client. The main implementer is v3Conn, which
// streams the results on a SQL network connection. There's also bufferedWriter,
// which buffers all results in memory.
//
// ResultsWriter is built with the SQL session model in mind: queries from a
// given SQL client (which we'll call the consumer to not confuse it with
// clients of this interface - the Executor) keep coming out of band and all of
// their results (generally, datum tuples) are pushed to a single ResultsWriter.
// The ResultsWriter needs to be made aware of which results pertain to which
// statement, as implementations need to split results accordingly. The
// ResultsWriter also supports the notion of a "results group": the
// ResultsWriter sequentially goes through groups of results and the group is
// the level at which a client can request for results to be dropped; a group
// can be reset, meaning that the consumer will not receive any of them. Only
// the current group can be reset; the client gives up the ability to reset a
// group the moment it closes it.  This feature is used to support the automatic
// retries that we do for SQL transactions - groups will correspond to
// transactions and, when the Executor decides to automatically retry a
// transaction, it will reset its group (as it can't automatically retry if any
// results have been sent to the consumer).
//
// Example usage:
//
//  var rw ResultsWriter
//  group := rw.NewResultsGroup()
//  defer group.Close()
//  sr := group.NewStatementResult()
//  for each result row {
//    if err := sr.AddRow(...); err != nil {
//      // send err to the client in another way
//    }
//  }
//  sr.CloseResult()
//  group.Close()
//
type ResultsWriter interface {
	// NewResultsGroup creates a new ResultGroup and indicates that future results
	// are part of a new result group.
	//
	// A single group can be ongoing on a ResultsWriter at a time; it is illegal to
	// create a new group before the previous one has been Close()d.
	NewResultsGroup() ResultsGroup

	// SetEmptyQuery is used to indicate that there are no statements to run.
	// Empty queries are different than queries with no results.
	SetEmptyQuery()
}

// ResultsGroup is used to produce a result group (see ResultsWriter).
type ResultsGroup interface {
	// Close should be called once all the results for a group have been produced
	// (i.e. after all StatementResults have been closed). It informs the
	// implementation that the results are not going to be reset any more, and
	// thus can be sent to the client.
	//
	// Close has to be called before new groups are created.  It's illegal to call
	// Close before CloseResult() has been called on all of the group's
	// StatementResults.
	Close()

	// NewStatementResult creates a new StatementResult, indicating that future
	// results are part of a new SQL query.
	//
	// A single StatementResult can be active on a ResultGroup at a time; it is
	// illegal to create a new StatementResult before CloseResult() has been
	// called on the previous one.
	NewStatementResult() StatementResult

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
