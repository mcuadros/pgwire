package server

import (
	"context"
	"net/url"
	"time"

	"github.com/mcuadros/pgwire"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Base config defaults.
const (
	// DefaultPort is the default port.
	DefaultPort = "5432"
	// NetworkTimeout is the timeout used for network operations.
	NetworkTimeout = 3 * time.Second

	defaultInsecure = false
	defaultUser     = "root"
	defaultAddr     = ":" + DefaultPort
)

type Config struct {
	// Insecure specifies whether to use SSL or not.
	// This is really not recommended.
	Insecure bool
	// SSLCertsDir is the path to the certificate/key directory.
	SSLCertsDir string
	// User running this process. It could be the user under which
	// the server is running or the user passed in client calls.
	User string
	// Addr is the address the server is listening on.
	Addr string
}

func NewConfig() *Config {
	return &Config{
		Insecure: defaultInsecure,
		User:     defaultUser,
		Addr:     defaultAddr,
	}
}

func (cfg *Config) URL(user *url.Userinfo) (*url.URL, error) {
	options := url.Values{}
	if cfg.Insecure {
		options.Add("sslmode", "disable")
	}

	return &url.URL{
		Scheme:   "postgresql",
		User:     user,
		RawQuery: options.Encode(),
	}, nil
}

func didYouMeanInsecureError(err error) error {
	return errors.Wrap(err, "problem using security settings, did you mean to use --insecure?")
}

func ParseOptions(ctx context.Context, data []byte) (pgwire.SessionArgs, error) {
	var args pgwire.SessionArgs

	buf := pgwire.ReadBuffer{Msg: data}
	for {
		key, err := buf.GetString()
		if err != nil {
			return args, errors.Errorf("error reading option key: %s", err)
		}
		if len(key) == 0 {
			break
		}
		value, err := buf.GetString()
		if err != nil {
			return args, errors.Errorf("error reading option value: %s", err)
		}
		switch key {
		case "database":
			args.Database = value
		case "user":
			args.User = value
		case "application_name":
			args.ApplicationName = value
		case "client_encoding":
			args.ClientEncoding = value
		default:
			log.Warningf("unrecognized configuration parameter %q", key)
		}
	}

	return args, nil
}
