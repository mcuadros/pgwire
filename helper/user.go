package helper

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/security"
	"github.com/mcuadros/pgwire"
)

// GetUserHashedPassword returns the hashedPassword for the given username if
// found in system.users.
func GetUserHashedPassword(ctx context.Context, executor pgwire.Executor, username string) (bool, []byte, error) {
	normalizedUsername := pgwire.NormalizeName(username)
	// Always return no password for the root user, even if someone manually inserts one.
	if normalizedUsername == security.RootUser {
		return true, nil, nil
	}

	var hashedPassword []byte
	var exists bool
	var err error

	// TODO:
	//	const getHashedPassword = `SELECT "hashedPassword" FROM system.users ` +
	//		WHERE username=$1 AND "isRole" = false`
	//	values, err := p.QueryRow(ctx, getHashedPassword, normalizedUsername)
	//	if err != nil {
	//		return errors.Errorf("error looking up user %s", normalizedUsername)
	//	}
	//	if len(values) == 0 {
	//		return nil
	//	}
	//	exists = true
	//	hashedPassword = []byte(*(values[0].(*tree.DBytes)))
	//	return nil
	//})
	return exists, hashedPassword, err
}
