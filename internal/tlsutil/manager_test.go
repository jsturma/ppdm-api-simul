package tlsutil

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerAutoRenewLoop(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, DefaultCertName)
	keyPath := filepath.Join(dir, DefaultKeyName)

	now := time.Now().UTC()
	expiring, err := generateCertificate(defaultHosts("localhost"), now.Add(-6*24*time.Hour), now.Add(12*time.Hour))
	if err != nil {
		t.Fatalf("generate expiring certificate: %v", err)
	}
	if err := WriteCertificateFiles(expiring, certPath, keyPath); err != nil {
		t.Fatalf("write certificate: %v", err)
	}

	manager := &Manager{
		cert:        expiring,
		certPath:    certPath,
		keyPath:     keyPath,
		hosts:       defaultHosts("localhost"),
		managed:     true,
		renewBefore: 24 * time.Hour,
		checkEvery:  20 * time.Millisecond,
		now:         func() time.Time { return now },
	}

	initial := manager.Resolved().Certificate

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager.StartAutoRenew(ctx)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		current := manager.Resolved().Certificate
		if len(initial.Certificate) > 0 && len(current.Certificate) > 0 &&
			string(initial.Certificate[0]) != string(current.Certificate[0]) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatal("expected runtime auto-renewal to replace certificate")
}
