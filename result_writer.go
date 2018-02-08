package pgwire

import "fmt"

// WireFailureError is used when sending data over pgwire fails.
type WireFailureError struct {
	err error
}

func (e WireFailureError) Error() string {
	return fmt.Sprintf("WireFailureError: %s", e.err.Error())
}

// NewWireFailureError returns a new WireFailureError which wraps err.
func NewWireFailureError(err error) error {
	return WireFailureError{err}
}

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
