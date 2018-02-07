package server

import (
	"context"
	"crypto/tls"
	"net/url"
	"sync"
	"time"

	"github.com/cockroachdb/cockroach/pkg/security"
	"github.com/cockroachdb/cockroach/pkg/sql"
	"github.com/cockroachdb/cockroach/pkg/util/stop"
	"github.com/mcuadros/pgwire/pgwirebase"
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
	defaultUser     = security.RootUser
	defaultAddr     = ":" + DefaultPort
)

type Config struct {
	// Insecure specifies whether to use SSL or not.
	// This is really not recommended.
	Insecure bool
	// SSLCAKey is used to sign new certs.
	SSLCAKey string
	// SSLCertsDir is the path to the certificate/key directory.
	SSLCertsDir string
	// User running this process. It could be the user under which
	// the server is running or the user passed in client calls.
	User string
	// Addr is the address the server is listening on.
	Addr string

	// The certificate manager. Must be accessed through GetCertificateManager.
	certificateManager lazyCertificateManager
}

type lazyCertificateManager struct {
	once sync.Once
	cm   *security.CertificateManager
	err  error
}

func NewConfig() *Config {
	return &Config{
		Insecure:           defaultInsecure,
		User:               defaultUser,
		Addr:               defaultAddr,
		certificateManager: lazyCertificateManager{},
	}
}

// ClientCertPaths returns the paths to the client cert and key.
func (cfg *Config) ClientCertPaths(user string) (string, string, error) {
	cm, err := cfg.CertificateManager()
	if err != nil {
		return "", "", err
	}
	return cm.GetClientCertPaths(user)
}

// CACertPath returns the path to the CA certificate.
func (cfg *Config) CACertPath() (string, error) {
	cm, err := cfg.CertificateManager()
	if err != nil {
		return "", err
	}
	return cm.GetCACertPath()
}

// ClientHasValidCerts returns true if the specified client has a valid client cert and key.
func (cfg *Config) ClientHasValidCerts(user string) bool {
	_, _, err := cfg.ClientCertPaths(user)
	return err == nil
}

func (cfg *Config) URL(user *url.Userinfo) (*url.URL, error) {
	options := url.Values{}
	if cfg.Insecure {
		options.Add("sslmode", "disable")
	} else {
		// Fetch CA cert. This is required.
		caCertPath, err := cfg.CACertPath()
		if err != nil {
			return nil, didYouMeanInsecureError(err)
		}
		options.Add("sslmode", "verify-full")
		options.Add("sslrootcert", caCertPath)

		// Fetch certs, but don't fail, we may be using a password.
		certPath, keyPath, err := cfg.ClientCertPaths(user.Username())
		if err == nil {
			options.Add("sslcert", certPath)
			options.Add("sslkey", keyPath)
		}
	}

	return &url.URL{
		Scheme:   "postgresql",
		User:     user,
		RawQuery: options.Encode(),
	}, nil
}

// CertificateManager returns the certificate manager, initializing it
// on the first call.
func (cfg *Config) CertificateManager() (*security.CertificateManager, error) {
	cfg.certificateManager.once.Do(func() {
		cfg.certificateManager.cm, cfg.certificateManager.err =
			security.NewCertificateManager(cfg.SSLCertsDir)
	})
	return cfg.certificateManager.cm, cfg.certificateManager.err
}

// InitializeNodeTLSConfigs tries to load client and server-side TLS configs.
// It also enables the reload-on-SIGHUP functionality on the certificate manager.
// This should be called early in the life of the server to make sure there are no
// issues with TLS configs.
// Returns the certificate manager if successfully created and in secure mode.
func (cfg *Config) InitializeNodeTLSConfigs(
	stopper *stop.Stopper,
) (*security.CertificateManager, error) {
	if cfg.Insecure {
		return nil, nil
	}

	if _, err := cfg.ServerTLSConfig(); err != nil {
		return nil, err
	}
	if _, err := cfg.ClientTLSConfig(); err != nil {
		return nil, err
	}

	cm, err := cfg.CertificateManager()
	if err != nil {
		return nil, err
	}
	cm.RegisterSignalHandler(stopper)
	return cm, nil
}

// ClientTLSConfig returns the client TLS config, initializing it if needed.
// If Insecure is true, return a nil config, otherwise ask the certificate
// manager for a TLS config using certs for the config.User.
func (cfg *Config) ClientTLSConfig() (*tls.Config, error) {
	// Early out.
	if cfg.Insecure {
		return nil, nil
	}

	cm, err := cfg.CertificateManager()
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}

	tlsCfg, err := cm.GetClientTLSConfig(cfg.User)
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}
	return tlsCfg, nil
}

// ServerTLSConfig returns the server TLS config, initializing it if needed.
// If Insecure is true, return a nil config, otherwise ask the certificate
// manager for a server TLS config.
func (cfg *Config) ServerTLSConfig() (*tls.Config, error) {
	// Early out.
	if cfg.Insecure {
		return nil, nil
	}

	cm, err := cfg.CertificateManager()
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}

	tlsCfg, err := cm.GetServerTLSConfig()
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}

	return tlsCfg, nil
}

func didYouMeanInsecureError(err error) error {
	return errors.Wrap(err, "problem using security settings, did you mean to use --insecure?")
}

func ParseOptions(ctx context.Context, data []byte) (sql.SessionArgs, error) {
	args := sql.SessionArgs{}
	buf := pgwirebase.ReadBuffer{Msg: data}
	for {
		key, err := buf.GetString()
		if err != nil {
			return sql.SessionArgs{}, errors.Errorf("error reading option key: %s", err)
		}
		if len(key) == 0 {
			break
		}
		value, err := buf.GetString()
		if err != nil {
			return sql.SessionArgs{}, errors.Errorf("error reading option value: %s", err)
		}
		switch key {
		case "database":
			args.Database = value
		case "user":
			args.User = value
		case "application_name":
			args.ApplicationName = value
		default:
			log.Warningf("unrecognized configuration parameter %q", key)
		}
	}
	return args, nil
}
