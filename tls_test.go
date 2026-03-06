package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func generateTestKeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

func generateTestRSAKeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func generateTestPKCS8KeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func generateSelfSignedCert(t *testing.T, keyPEM []byte, san string, notAfter time.Time) []byte {
	t.Helper()
	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: san},
		DNSNames:     []string{san},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	pubKey := key.(*ecdsa.PrivateKey).Public()
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, pubKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
}

func TestParsePEMPrivateKeyEC(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parsePEMPrivateKey: %v", err)
	}
	if _, ok := key.(*ecdsa.PrivateKey); !ok {
		t.Errorf("expected *ecdsa.PrivateKey, got %T", key)
	}
}

func TestParsePEMPrivateKeyRSA(t *testing.T) {
	keyPEM := generateTestRSAKeyPEM(t)
	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parsePEMPrivateKey: %v", err)
	}
	if _, ok := key.(*rsa.PrivateKey); !ok {
		t.Errorf("expected *rsa.PrivateKey, got %T", key)
	}
}

func TestParsePEMPrivateKeyPKCS8(t *testing.T) {
	keyPEM := generateTestPKCS8KeyPEM(t)
	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("parsePEMPrivateKey: %v", err)
	}
	if key == nil {
		t.Error("expected non-nil key")
	}
}

func TestParsePEMPrivateKeyInvalid(t *testing.T) {
	_, err := parsePEMPrivateKey([]byte("not a pem block"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestParsePEMPrivateKeyEmpty(t *testing.T) {
	_, err := parsePEMPrivateKey([]byte{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParsePEMPrivateKeyBadDER(t *testing.T) {
	badPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")})
	_, err := parsePEMPrivateKey(badPEM)
	if err == nil {
		t.Fatal("expected error for bad DER data")
	}
}

func TestTryExistingCertValid(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	certPEM := generateSelfSignedCert(t, keyPEM, "test.example.com", time.Now().Add(90*24*time.Hour))

	cfg, err := tryExistingCert(certPEM, keyPEM, "test.example.com")
	if err != nil {
		t.Fatalf("tryExistingCert: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls.Config")
	}
	if len(cfg.Certificates) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(cfg.Certificates))
	}
}

func TestTryExistingCertExpiringSoon(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	certPEM := generateSelfSignedCert(t, keyPEM, "test.example.com", time.Now().Add(15*24*time.Hour))

	_, err := tryExistingCert(certPEM, keyPEM, "test.example.com")
	if err == nil {
		t.Fatal("expected error for expiring cert")
	}
}

func TestTryExistingCertWrongSAN(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	certPEM := generateSelfSignedCert(t, keyPEM, "other.example.com", time.Now().Add(90*24*time.Hour))

	_, err := tryExistingCert(certPEM, keyPEM, "test.example.com")
	if err == nil {
		t.Fatal("expected error for wrong SAN")
	}
}

func TestTryExistingCertBadKeyPair(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	certPEM := generateSelfSignedCert(t, keyPEM, "test.example.com", time.Now().Add(90*24*time.Hour))

	otherKey := generateTestKeyPEM(t)
	_, err := tryExistingCert(certPEM, otherKey, "test.example.com")
	if err == nil {
		t.Fatal("expected error for mismatched key pair")
	}
}

func TestTryExistingCertInvalidPEM(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	_, err := tryExistingCert([]byte("not a cert"), keyPEM, "test.example.com")
	if err == nil {
		t.Fatal("expected error for invalid cert PEM")
	}
}

func TestObtainTLSConfigNoEnvVars(t *testing.T) {
	t.Setenv("DNS_SAN", "")
	t.Setenv("CLOUDFLARE_API_KEY", "")
	t.Setenv("TLS_PRIVKEY", "")

	cfg, err := obtainTLSConfig()
	if err != nil {
		t.Fatalf("obtainTLSConfig: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when env vars are not set")
	}
}

func TestObtainTLSConfigPartialEnvVars(t *testing.T) {
	t.Setenv("DNS_SAN", "example.com")
	t.Setenv("CLOUDFLARE_API_KEY", "")
	t.Setenv("TLS_PRIVKEY", "")

	cfg, err := obtainTLSConfig()
	if err != nil {
		t.Fatalf("obtainTLSConfig: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when not all env vars are set")
	}
}

func TestObtainTLSConfigBadBase64(t *testing.T) {
	t.Setenv("DNS_SAN", "example.com")
	t.Setenv("CLOUDFLARE_API_KEY", "some-key")
	t.Setenv("TLS_PRIVKEY", "not-valid-base64!!!")

	_, err := obtainTLSConfig()
	if err == nil {
		t.Fatal("expected error for bad base64 in TLS_PRIVKEY")
	}
}

func TestObtainTLSConfigBadPEM(t *testing.T) {
	t.Setenv("DNS_SAN", "example.com")
	t.Setenv("CLOUDFLARE_API_KEY", "some-key")
	t.Setenv("TLS_PRIVKEY", base64.StdEncoding.EncodeToString([]byte("not a PEM key")))

	_, err := obtainTLSConfig()
	if err == nil {
		t.Fatal("expected error for bad PEM in TLS_PRIVKEY")
	}
}

func TestObtainTLSConfigCachedCert(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	certPEM := generateSelfSignedCert(t, keyPEM, "cached.example.com", time.Now().Add(90*24*time.Hour))

	// Write the cached cert to a temp dir and run from there
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.WriteFile("cert.pem", certPEM, 0644)

	t.Setenv("DNS_SAN", "cached.example.com")
	t.Setenv("CLOUDFLARE_API_KEY", "some-key")
	t.Setenv("TLS_PRIVKEY", base64.StdEncoding.EncodeToString(keyPEM))

	cfg, err := obtainTLSConfig()
	if err != nil {
		t.Fatalf("obtainTLSConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config from cached cert")
	}
}

func TestObtainTLSConfigExpiredCachedCert(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	// Cert expires in 10 days — below 30-day threshold
	certPEM := generateSelfSignedCert(t, keyPEM, "expiring.example.com", time.Now().Add(10*24*time.Hour))

	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.WriteFile("cert.pem", certPEM, 0644)

	t.Setenv("DNS_SAN", "expiring.example.com")
	t.Setenv("CLOUDFLARE_API_KEY", "some-key")
	t.Setenv("TLS_PRIVKEY", base64.StdEncoding.EncodeToString(keyPEM))

	// This will try to obtain a new cert via ACME, which will fail without
	// a real Cloudflare API key — but it should get past the cached cert check.
	_, err := obtainTLSConfig()
	if err == nil {
		t.Fatal("expected error when cached cert is expiring and ACME is not available")
	}
}

func TestTLSConfigWithServer(t *testing.T) {
	keyPEM := generateTestKeyPEM(t)
	key, _ := parsePEMPrivateKey(keyPEM)
	ecKey := key.(*ecdsa.PrivateKey)

	// Generate a cert with both DNS and IP SANs for httptest compatibility
	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, ecKey.Public(), ecKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewUnstartedServer(securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secure"))
	}), false))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	srv.StartTLS()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.TLS == nil {
		t.Error("expected TLS connection")
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers not applied over TLS")
	}
}
