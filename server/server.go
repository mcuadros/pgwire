// Copyright 2015 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package server

import (
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	"github.com/mcuadros/pgwire"
	"github.com/mcuadros/pgwire/pgerror"
	"github.com/mcuadros/pgwire/pgwirebase"
	"github.com/mcuadros/pgwire/server/v3"

	"github.com/pkg/errors"
)

const (
	// ErrSSLRequired is returned when a client attempts to connect to a
	// secure server in cleartext.
	ErrSSLRequired = "node is running secure mode, SSL connection required"

	// ErrDraining is returned when a client attempts to connect to a server
	// which is not accepting client connections.
	ErrDraining = "server is not accepting clients"
)

const (
	version30  = 196608
	versionSSL = 80877103
)

// cancelMaxWait is the amount of time a draining server gives to sessions to
// react to cancellation and return before a forceful shutdown.
const cancelMaxWait = 1 * time.Second

var (
	sslSupported   = []byte{'S'}
	sslUnsupported = []byte{'N'}
)

// cancelChanMap keeps track of channels that are closed after the associated
// cancellation function has been called and the cancellation has taken place.
type cancelChanMap map[chan struct{}]context.CancelFunc

// Server implements the server side of the PostgreSQL wire protocol.
type Server struct {
	cfg      *Config
	executor pgwire.Executor

	l net.Listener

	mu struct {
		sync.Mutex
		// connCancelMap entries represent connections started when the server
		// was not draining. Each value is a function that can be called to
		// cancel the associated connection. The corresponding key is a channel
		// that is closed when the connection is done.
		connCancelMap cancelChanMap
		draining      bool
	}
}

func New(cfg *Config, e pgwire.Executor) *Server {
	s := &Server{
		cfg:      cfg,
		executor: e,
	}

	s.mu.Lock()
	s.mu.connCancelMap = make(cancelChanMap)
	s.mu.Unlock()

	return s
}

func (s *Server) Start() error {
	if err := s.initialize(); err != nil {
		return err
	}

	for {
		conn, err := s.l.Accept()
		if err != nil {
			return err
		}

		go s.ServeConn(context.TODO(), conn)
	}
}

func (s *Server) initialize() error {
	if s.l == nil {
		var err error
		s.l, err = net.Listen("tcp", s.cfg.Addr)
		return err
	}

	return nil
}

// IsDraining returns true if the server is not currently accepting
// connections.
func (s *Server) IsDraining() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mu.draining
}

// Drain prevents new connections from being served and waits for drainWait for
// open connections to terminate before canceling them.
// An error will be returned when connections that have been canceled have not
// responded to this cancellation and closed themselves in time. The server
// will remain in draining state, though open connections may continue to
// exist.
// The RFC on drain modes has more information regarding the specifics of
// what will happen to connections in different states:
// https://github.com/cockroachdb/cockroach/blob/master/docs/RFCS/20160425_drain_modes.md
func (s *Server) Drain(drainWait time.Duration) error {
	return s.drainImpl(drainWait, cancelMaxWait)
}

// Undrain switches the server back to the normal mode of operation in which
// connections are accepted.
func (s *Server) Undrain() {
	s.mu.Lock()
	s.setDrainingLocked(false)
	s.mu.Unlock()
}

// setDrainingLocked sets the server's draining state and returns whether the
// state changed (i.e. drain != s.mu.draining). s.mu must be locked.
func (s *Server) setDrainingLocked(drain bool) bool {
	if s.mu.draining == drain {
		return false
	}
	s.mu.draining = drain
	return true
}

func (s *Server) drainImpl(drainWait time.Duration, cancelWait time.Duration) error {
	// This anonymous function returns a copy of s.mu.connCancelMap if there are
	// any active connections to cancel. We will only attempt to cancel
	// connections that were active at the moment the draining switch happened.
	// It is enough to do this because:
	// 1) If no new connections are added to the original map all connections
	// will be canceled.
	// 2) If new connections are added to the original map, it follows that they
	// were added when s.mu.draining = false, thus not requiring cancellation.
	// These connections are not our responsibility and will be handled when the
	// server starts draining again.
	connCancelMap := func() cancelChanMap {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.setDrainingLocked(true) {
			// We are already draining.
			return nil
		}
		connCancelMap := make(cancelChanMap)
		for done, cancel := range s.mu.connCancelMap {
			connCancelMap[done] = cancel
		}
		return connCancelMap
	}()
	if len(connCancelMap) == 0 {
		return nil
	}

	// Spin off a goroutine that waits for all connections to signal that they
	// are done and reports it on allConnsDone. The main goroutine signals this
	// goroutine to stop work through quitWaitingForConns.
	allConnsDone := make(chan struct{})
	quitWaitingForConns := make(chan struct{})
	defer close(quitWaitingForConns)
	go func() {
		defer close(allConnsDone)
		for done := range connCancelMap {
			select {
			case <-done:
			case <-quitWaitingForConns:
				return
			}
		}
	}()

	// Wait for all connections to finish up to drainWait.
	select {
	case <-time.After(drainWait):
	case <-allConnsDone:
	}

	// Cancel the contexts of all sessions if the server is still in draining
	// mode.
	if stop := func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.mu.draining {
			return true
		}
		for _, cancel := range connCancelMap {
			// There is a possibility that different calls to SetDraining have
			// overlapping connCancelMaps, but context.CancelFunc calls are
			// idempotent.
			cancel()
		}
		return false
	}(); stop {
		return nil
	}

	select {
	case <-time.After(cancelWait):
		return errors.Errorf("some sessions did not respond to cancellation within %s", cancelWait)
	case <-allConnsDone:
	}
	return nil
}

// ServeConn serves a single connection, driving the handshake process
// and delegating to the appropriate connection type.
func (s *Server) ServeConn(ctx context.Context, conn net.Conn) error {
	s.mu.Lock()
	draining := s.mu.draining
	if !draining {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		done := make(chan struct{})
		s.mu.connCancelMap[done] = cancel
		defer func() {
			cancel()
			close(done)
			s.mu.Lock()
			delete(s.mu.connCancelMap, done)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()

	var buf pgwirebase.ReadBuffer
	_, err := buf.ReadUntypedMsg(conn)
	if err != nil {
		return err
	}

	version, err := buf.GetUint32()
	if err != nil {
		return err
	}

	errSSLRequired := false
	if version == versionSSL {
		if len(buf.Msg) > 0 {
			return errors.Errorf("unexpected data after SSLRequest: %q", buf.Msg)
		}

		if s.cfg.Insecure {
			if _, err := conn.Write(sslUnsupported); err != nil {
				return err
			}
		} else {
			if _, err := conn.Write(sslSupported); err != nil {
				return err
			}
			tlsConfig, err := s.cfg.ServerTLSConfig()
			if err != nil {
				return err
			}
			conn = tls.Server(conn, tlsConfig)
		}

		_, err := buf.ReadUntypedMsg(conn)
		if err != nil {
			return err
		}
		version, err = buf.GetUint32()
		if err != nil {
			return err
		}
	} else if !s.cfg.Insecure {
		errSSLRequired = true
	}

	if version == version30 {
		// We make a connection before anything. If there is an error
		// parsing the connection arguments, the connection will only be
		// used to send a report of that error.
		v3conn := v3.NewConn(conn, s.executor)
		defer v3conn.Finish(ctx)

		args, err := ParseOptions(ctx, buf.Msg)
		if err != nil {
			return v3conn.SendError(pgerror.NewError(pgerror.CodeProtocolViolationError, err.Error()))
		}

		if errSSLRequired {
			return v3conn.SendError(pgerror.NewError(pgerror.CodeProtocolViolationError, ErrSSLRequired))
		}

		if draining {
			return v3conn.SendError(pgerror.NewErrorf(pgerror.CodeAdminShutdownError, ErrDraining))
		}

		//TODO:v3conn.sessionArgs.User = tree.Name(v3conn.sessionArgs.User).Normalize()
		if err := v3conn.HandleAuthentication(ctx, s.cfg.Insecure); err != nil {
			return v3conn.SendError(pgerror.NewError(pgerror.CodeInvalidPasswordError, err.Error()))
		}

		session := pgwire.NewSession(ctx, args)
		err = v3conn.Serve(ctx, session, s.IsDraining)
		// If the error that closed the connection is related to an
		// administrative shutdown, relay that information to the client.
		if pgErr, ok := pgerror.GetPGCause(err); ok && pgErr.Code == pgerror.CodeAdminShutdownError {
			return v3conn.SendError(err)
		}
		return err
	}

	return errors.Errorf("unknown protocol version %d", version)
}
