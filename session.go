package pgwire

import (
	"context"
	"time"
)

// SessionArgs contains arguments for creating a new Session with NewSession().
type SessionArgs struct {
	Database        string
	User            string
	ApplicationName string
	ClientEncoding  string
}

// Session contains the state of a SQL client connection.
type Session interface {
	// Ctx returns the current context for the session
	Ctx() context.Context
	// ClientAddr is the client's IP address and port.
	ClientAddr() string
	// User is the username requested by the user.
	User() string
	// Database is the database name requested by the user.
	Database() string
	// ApplicationName is the application name provided by the client.
	ApplicationName() string
	// ClientEncoding
	ClientEncoding() string
	// Location exports the location session variable.
	Location() *time.Location
}

// NewSession returns a new Session containing the state of SQL client connection.
func NewSession(ctx context.Context, args SessionArgs) Session {
	l := time.Now().Location()
	return &session{
		ctx:      ctx,
		args:     args,
		location: *l,
	}
}

type session struct {
	ctx      context.Context
	rw       ResultsWriter
	args     SessionArgs
	location time.Location
}

// ClientAddr implements Session interface.
func (s *session) ClientAddr() string {
	return s.args.User
}

// User implements Session interface.
func (s *session) User() string {
	return s.args.User
}

// Database implements Session interface.
func (s *session) Database() string {
	return s.args.Database
}

// ApplicationName implements Session interface.
func (s *session) ApplicationName() string {
	return s.args.ApplicationName
}

// ClientEncoding implements Session interface.
func (s *session) ClientEncoding() string {
	return s.args.ClientEncoding
}

// Location implements Session interface.
func (s *session) Location() *time.Location {
	return &s.location
}

// Ctx implements Session interface.
func (s *session) Ctx() context.Context {
	return context.Background()
}

type Executor interface {
	ExecuteStatements(s Session, rw ResultsWriter, stmts string) error
	RecordError(err error)
}
