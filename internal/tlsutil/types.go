package tlsutil

import (
	"crypto/tls"
	"time"
)

const (
	DefaultSSLDir      = "ssl"
	DefaultCertName    = "cert.pem"
	DefaultKeyName     = "key.pem"
	CertLifetime       = 7 * 24 * time.Hour
	certSkew           = time.Hour
	defaultRenewBefore = 24 * time.Hour
	defaultCheckEvery  = time.Hour
)

type ResolvedCert struct {
	Certificate tls.Certificate
	CertPath    string
	KeyPath     string
	Generated   bool
	Managed     bool
	Hosts       []string
}
