package pgwire

import (
	"crypto/tls"
	"net/url"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/cockroachdb/cockroach/pkg/security"
	"github.com/cockroachdb/cockroach/pkg/util/stop"
)

// Base config defaults.
const (
	defaultInsecure = false
	defaultUser     = security.RootUser
	httpScheme      = "http"
	httpsScheme     = "https"

	// From IANA Service Name and Transport Protocol Port Number Registry. See
	// https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml?search=cockroachdb
	DefaultPort = "26257"

	// NB: net.JoinHostPort is not a constant.
	defaultAddr = ":" + DefaultPort

	// NetworkTimeout is the timeout used for network operations.
	NetworkTimeout = 3 * time.Second

	// DefaultCertsDirectory is the default value for the cert directory flag.
	DefaultCertsDirectory = "${HOME}/.cockroach-certs"
)

type lazyCertificateManager struct {
	once sync.Once
	cm   *security.CertificateManager
	err  error
}

// Config is embedded by server.Config. A base config is not meant to be used
// directly, but embedding configs should call cfg.InitDefaults().
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

	// AdvertiseAddr is the address advertised by the server to other nodes
	// in the cluster. It should be reachable by all other nodes and should
	// route to an interface that Addr is listening on.
	AdvertiseAddr string

	// HTTPAddr is server's public HTTP address.
	//
	// This is temporary, and will be removed when grpc.(*Server).ServeHTTP
	// performance problems are addressed upstream.
	//
	// See https://github.com/grpc/grpc-go/issues/586.
	HTTPAddr string

	// The certificate manager. Must be accessed through GetCertificateManager.
	certificateManager lazyCertificateManager

	// HistogramWindowInterval is used to determine the approximate length of time
	// that individual samples are retained in in-memory histograms. Currently,
	// it is set to the arbitrary length of six times the Metrics sample interval.
	// See the comment in server.Config for more details.
	HistogramWindowInterval time.Duration
}

func didYouMeanInsecureError(err error) error {
	return errors.Wrap(err, "problem using security settings, did you mean to use --insecure?")
}

// InitDefaults sets up the default values for a config.
// This is also used in tests to reset global objects.
func (cfg *Config) InitDefaults() {
	cfg.Insecure = defaultInsecure
	cfg.User = defaultUser
	cfg.Addr = defaultAddr
	cfg.AdvertiseAddr = cfg.Addr
	cfg.SSLCertsDir = DefaultCertsDirectory
	cfg.certificateManager = lazyCertificateManager{}
}

// HTTPRequestScheme returns "http" or "https" based on the value of Insecure.
func (cfg *Config) HTTPRequestScheme() string {
	if cfg.Insecure {
		return httpScheme
	}
	return httpsScheme
}

// AdminURL returns the URL for the admin UI.
func (cfg *Config) AdminURL() *url.URL {
	return &url.URL{
		Scheme: cfg.HTTPRequestScheme(),
		Host:   cfg.HTTPAddr,
	}
}

// GetClientCertPaths returns the paths to the client cert and key.
func (cfg *Config) GetClientCertPaths(user string) (string, string, error) {
	cm, err := cfg.GetCertificateManager()
	if err != nil {
		return "", "", err
	}
	return cm.GetClientCertPaths(user)
}

// GetCACertPath returns the path to the CA certificate.
func (cfg *Config) GetCACertPath() (string, error) {
	cm, err := cfg.GetCertificateManager()
	if err != nil {
		return "", err
	}
	return cm.GetCACertPath()
}

// ClientHasValidCerts returns true if the specified client has a valid client cert and key.
func (cfg *Config) ClientHasValidCerts(user string) bool {
	_, _, err := cfg.GetClientCertPaths(user)
	return err == nil
}

// PGURL returns the URL for the postgres endpoint.
func (cfg *Config) PGURL(user *url.Userinfo) (*url.URL, error) {
	options := url.Values{}
	if cfg.Insecure {
		options.Add("sslmode", "disable")
	} else {
		// Fetch CA cert. This is required.
		caCertPath, err := cfg.GetCACertPath()
		if err != nil {
			return nil, didYouMeanInsecureError(err)
		}
		options.Add("sslmode", "verify-full")
		options.Add("sslrootcert", caCertPath)

		// Fetch certs, but don't fail, we may be using a password.
		certPath, keyPath, err := cfg.GetClientCertPaths(user.Username())
		if err == nil {
			options.Add("sslcert", certPath)
			options.Add("sslkey", keyPath)
		}
	}
	options.Add("application_name", "cockroach")

	return &url.URL{
		Scheme:   "postgresql",
		User:     user,
		Host:     cfg.AdvertiseAddr,
		RawQuery: options.Encode(),
	}, nil
}

// GetCertificateManager returns the certificate manager, initializing it
// on the first call.
func (cfg *Config) GetCertificateManager() (*security.CertificateManager, error) {
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

	if _, err := cfg.GetServerTLSConfig(); err != nil {
		return nil, err
	}
	if _, err := cfg.GetClientTLSConfig(); err != nil {
		return nil, err
	}

	cm, err := cfg.GetCertificateManager()
	if err != nil {
		return nil, err
	}
	cm.RegisterSignalHandler(stopper)
	return cm, nil
}

// GetClientTLSConfig returns the client TLS config, initializing it if needed.
// If Insecure is true, return a nil config, otherwise ask the certificate
// manager for a TLS config using certs for the config.User.
func (cfg *Config) GetClientTLSConfig() (*tls.Config, error) {
	// Early out.
	if cfg.Insecure {
		return nil, nil
	}

	cm, err := cfg.GetCertificateManager()
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}

	tlsCfg, err := cm.GetClientTLSConfig(cfg.User)
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}
	return tlsCfg, nil
}

// GetServerTLSConfig returns the server TLS config, initializing it if needed.
// If Insecure is true, return a nil config, otherwise ask the certificate
// manager for a server TLS config.
func (cfg *Config) GetServerTLSConfig() (*tls.Config, error) {
	// Early out.
	if cfg.Insecure {
		return nil, nil
	}

	cm, err := cfg.GetCertificateManager()
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}

	tlsCfg, err := cm.GetServerTLSConfig()
	if err != nil {
		return nil, didYouMeanInsecureError(err)
	}
	return tlsCfg, nil
}
