package tlsutil

import (
	"context"
	"crypto/tls"
	"log"
	"path/filepath"
	"sync"
	"time"
)

type Manager struct {
	mu          sync.RWMutex
	cert        tls.Certificate
	certPath    string
	keyPath     string
	hosts       []string
	managed     bool
	renewBefore time.Duration
	checkEvery  time.Duration
	now         func() time.Time
}

func NewManager(certFile, keyFile, sslDir, host string) (*Manager, error) {
	resolved, err := ResolveCertificate(certFile, keyFile, sslDir, host)
	if err != nil {
		return nil, err
	}

	managed := certFile == "" && keyFile == ""
	manager := &Manager{
		cert:        resolved.Certificate,
		certPath:    resolved.CertPath,
		keyPath:     resolved.KeyPath,
		hosts:       resolved.Hosts,
		managed:     managed,
		renewBefore: defaultRenewBefore,
		checkEvery:  defaultCheckEvery,
		now:         time.Now,
	}

	if managed {
		if err := manager.RenewIfNeeded(); err != nil {
			return nil, err
		}
	}

	return manager, nil
}

func (m *Manager) Resolved() ResolvedCert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ResolvedCert{
		Certificate: m.cert,
		CertPath:    m.certPath,
		KeyPath:     m.keyPath,
		Managed:     m.managed,
		Hosts:       append([]string(nil), m.hosts...),
	}
}

func (m *Manager) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			m.mu.RLock()
			defer m.mu.RUnlock()
			cert := m.cert
			return &cert, nil
		},
	}
}

func (m *Manager) StartAutoRenew(ctx context.Context) {
	if !m.managed {
		return
	}

	go func() {
		ticker := time.NewTicker(m.checkEvery)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.RenewIfNeeded(); err != nil {
					log.Printf("tls certificate renewal failed: %v", err)
				}
			}
		}
	}()
}

func (m *Manager) RenewIfNeeded() error {
	if !m.managed {
		return nil
	}

	m.mu.RLock()
	needsRenewal := m.needsRenewalLocked()
	m.mu.RUnlock()
	if !needsRenewal {
		return nil
	}

	return m.renew()
}

func (m *Manager) needsRenewalLocked() bool {
	parsed, err := parsedCertificate(m.cert)
	if err != nil {
		return true
	}
	if err := validateParsedCertificate(parsed, m.now()); err != nil {
		return true
	}
	return time.Until(parsed.NotAfter) <= m.renewBefore
}

func (m *Manager) renew() error {
	cert, err := GenerateSelfSigned(m.hosts)
	if err != nil {
		return err
	}
	if err := WriteCertificateFiles(cert, m.certPath, m.keyPath); err != nil {
		return err
	}

	parsed, err := parsedCertificate(cert)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.cert = cert
	m.mu.Unlock()

	log.Printf(
		"renewed self-signed TLS certificate in %s (valid until %s)",
		filepath.Dir(m.certPath),
		parsed.NotAfter.UTC().Format(time.RFC3339),
	)
	return nil
}
