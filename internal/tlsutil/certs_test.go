package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateSelfSignedValidity(t *testing.T) {
	cert, err := GenerateSelfSigned([]string{"localhost"})
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}

	parsed, err := parsedCertificate(cert)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	if !parsed.NotAfter.After(parsed.NotBefore) {
		t.Fatalf("invalid validity: notBefore=%s notAfter=%s", parsed.NotBefore, parsed.NotAfter)
	}

	lifetime := parsed.NotAfter.Sub(parsed.NotBefore)
	minLifetime := CertLifetime
	maxLifetime := CertLifetime + certSkew + time.Hour
	if lifetime < minLifetime || lifetime > maxLifetime {
		t.Fatalf("unexpected certificate lifetime: %s", lifetime)
	}
}

func TestWriteAndLoadCertificateValidity(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	generated, err := GenerateSelfSigned([]string{"localhost"})
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	if err := WriteCertificateFiles(generated, certPath, keyPath); err != nil {
		t.Fatalf("write certificate: %v", err)
	}

	loaded, err := LoadCertificate(certPath, keyPath)
	if err != nil {
		t.Fatalf("load certificate: %v", err)
	}
	if err := validateCertificate(loaded); err != nil {
		t.Fatalf("loaded certificate invalid: %v", err)
	}

	data, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read certificate file: %v", err)
	}
	block, _ := pem.Decode(data)
	parsed, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse pem certificate: %v", err)
	}
	if !parsed.NotAfter.After(parsed.NotBefore) {
		t.Fatalf("pem validity invalid: notBefore=%s notAfter=%s", parsed.NotBefore, parsed.NotAfter)
	}
}

func TestResolveCertificateRegeneratesInvalidStoredCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, DefaultCertName)
	keyPath := filepath.Join(dir, DefaultKeyName)

	now := time.Now().UTC()
	expired, err := generateCertificate(defaultHosts("localhost"), now.Add(-72*time.Hour), now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("generate expired certificate: %v", err)
	}
	if err := WriteCertificateFiles(expired, certPath, keyPath); err != nil {
		t.Fatalf("write expired certificate: %v", err)
	}

	resolved, err := ResolveCertificate("", "", dir, "localhost")
	if err != nil {
		t.Fatalf("resolve certificate: %v", err)
	}
	if !resolved.Generated {
		t.Fatal("expected invalid stored certificate to be regenerated")
	}
	if err := validateCertificate(resolved.Certificate); err != nil {
		t.Fatalf("regenerated certificate invalid: %v", err)
	}
}

func TestManagerRenewsDuringRuntime(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, DefaultCertName)
	keyPath := filepath.Join(dir, DefaultKeyName)

	now := time.Now().UTC()
	almostExpired, err := generateCertificate(defaultHosts("localhost"), now.Add(-6*24*time.Hour), now.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	if err := WriteCertificateFiles(almostExpired, certPath, keyPath); err != nil {
		t.Fatalf("write certificate: %v", err)
	}

	manager := &Manager{
		cert:        almostExpired,
		certPath:    certPath,
		keyPath:     keyPath,
		hosts:       defaultHosts("localhost"),
		managed:     true,
		renewBefore: 24 * time.Hour,
		checkEvery:  defaultCheckEvery,
		now:         func() time.Time { return now },
	}

	before := manager.Resolved().Certificate
	if err := manager.RenewIfNeeded(); err != nil {
		t.Fatalf("renew if needed: %v", err)
	}
	after := manager.Resolved().Certificate

	if len(before.Certificate) > 0 && len(after.Certificate) > 0 &&
		string(before.Certificate[0]) == string(after.Certificate[0]) {
		t.Fatal("expected certificate to be renewed")
	}
	if err := validateCertificate(after); err != nil {
		t.Fatalf("renewed certificate invalid: %v", err)
	}
}

func TestManagerDoesNotRenewCustomCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "custom.pem")
	keyPath := filepath.Join(dir, "custom.key")

	cert, err := GenerateSelfSigned([]string{"localhost"})
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	if err := WriteCertificateFiles(cert, certPath, keyPath); err != nil {
		t.Fatalf("write certificate: %v", err)
	}

	manager, err := NewManager(certPath, keyPath, dir, "localhost")
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	manager.renewBefore = CertLifetime

	before := manager.Resolved().Certificate
	if err := manager.RenewIfNeeded(); err != nil {
		t.Fatalf("renew if needed: %v", err)
	}
	after := manager.Resolved().Certificate

	if string(before.Certificate[0]) != string(after.Certificate[0]) {
		t.Fatal("custom certificate should not be auto-renewed")
	}
}

func generateCertificate(hosts []string, notBefore, notAfter time.Time) (tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "PPDM Simulator",
			Organization: []string{"PPDM Simulator"},
		},
		NotBefore:             notBefore.UTC(),
		NotAfter:              notAfter.UTC(),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  privateKey,
		Leaf:        parsed,
	}, nil
}
