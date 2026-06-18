package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func LoadCertificate(certFile, keyFile string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(certFile, keyFile)
}

func GenerateSelfSigned(hosts []string) (tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate private key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial number: %w", err)
	}

	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}

	now := time.Now().UTC()
	notBefore := now.Add(-certSkew)
	notAfter := now.Add(CertLifetime)
	if !notAfter.After(notBefore) {
		return tls.Certificate{}, fmt.Errorf("invalid certificate validity window")
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "PPDM Simulator",
			Organization: []string{"PPDM Simulator"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{},
		IPAddresses:           []net.IP{},
	}

	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
			continue
		}
		template.DNSNames = append(template.DNSNames, host)
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}

	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse certificate: %w", err)
	}
	if err := validateParsedCertificate(parsed, now); err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  privateKey,
		Leaf:        parsed,
	}, nil
}

func WriteCertificateFiles(cert tls.Certificate, certFile, keyFile string) error {
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("certificate has no DER data")
	}

	if err := os.MkdirAll(filepath.Dir(certFile), 0o700); err != nil {
		return fmt.Errorf("create certificate directory: %w", err)
	}

	certOut, err := os.OpenFile(certFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}); err != nil {
		_ = certOut.Close()
		return fmt.Errorf("write cert file: %w", err)
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("close cert file: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}

	keyOut, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		_ = keyOut.Close()
		return fmt.Errorf("write key file: %w", err)
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("close key file: %w", err)
	}

	return nil
}

func ResolveCertificate(certFile, keyFile, sslDir, host string) (ResolvedCert, error) {
	hosts := defaultHosts(host)

	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return ResolvedCert{}, fmt.Errorf("both -cert and -key must be provided together")
		}
		cert, err := LoadCertificate(certFile, keyFile)
		if err != nil {
			return ResolvedCert{}, err
		}
		if err := validateCertificate(cert); err != nil {
			return ResolvedCert{}, fmt.Errorf("invalid certificate %s: %w", certFile, err)
		}
		return ResolvedCert{
			Certificate: cert,
			CertPath:    certFile,
			KeyPath:     keyFile,
			Hosts:       hosts,
		}, nil
	}

	if sslDir == "" {
		sslDir = DefaultSSLDir
	}
	storedCert := filepath.Join(sslDir, DefaultCertName)
	storedKey := filepath.Join(sslDir, DefaultKeyName)

	if fileExists(storedCert) && fileExists(storedKey) {
		cert, err := LoadCertificate(storedCert, storedKey)
		if err == nil && validateCertificate(cert) == nil {
			return ResolvedCert{
				Certificate: cert,
				CertPath:    storedCert,
				KeyPath:     storedKey,
				Managed:     true,
				Hosts:       hosts,
			}, nil
		}
	}

	cert, err := GenerateSelfSigned(hosts)
	if err != nil {
		return ResolvedCert{}, err
	}
	if err := WriteCertificateFiles(cert, storedCert, storedKey); err != nil {
		return ResolvedCert{}, err
	}

	return ResolvedCert{
		Certificate: cert,
		CertPath:    storedCert,
		KeyPath:     storedKey,
		Generated:   true,
		Managed:     true,
		Hosts:       hosts,
	}, nil
}

func validateCertificate(cert tls.Certificate) error {
	parsed, err := parsedCertificate(cert)
	if err != nil {
		return err
	}
	return validateParsedCertificate(parsed, time.Now().UTC())
}

func validateParsedCertificate(parsed *x509.Certificate, now time.Time) error {
	if parsed == nil {
		return fmt.Errorf("certificate is nil")
	}
	if !parsed.NotAfter.After(parsed.NotBefore) {
		return fmt.Errorf("notBefore (%s) is not before notAfter (%s)", parsed.NotBefore.UTC(), parsed.NotAfter.UTC())
	}
	now = now.UTC()
	if now.Before(parsed.NotBefore) {
		return fmt.Errorf("certificate is not yet valid (starts %s)", parsed.NotBefore.UTC())
	}
	if !now.Before(parsed.NotAfter) {
		return fmt.Errorf("certificate expired at %s", parsed.NotAfter.UTC())
	}
	return nil
}

func parsedCertificate(cert tls.Certificate) (*x509.Certificate, error) {
	if cert.Leaf != nil {
		return cert.Leaf, nil
	}
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("no certificate data")
	}
	return x509.ParseCertificate(cert.Certificate[0])
}

func defaultHosts(host string) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if host != "" && host != "0.0.0.0" && host != "::" {
		hosts = append(hosts, host)
	}
	return hosts
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
