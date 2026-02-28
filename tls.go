package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/registration"
)

const certFile = "cert.pem"

type acmeUser struct {
	email        string
	registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

// obtainTLSConfig reads DNS_SAN, CLOUDFLARE_API_KEY, and TLS_PRIVKEY from the
// environment. If all three are set, it obtains a TLS certificate via ACME
// DNS-01 challenge using Cloudflare, saves it to cert.pem, and returns a
// tls.Config. Returns (nil, nil) when the env vars are not all set.
func obtainTLSConfig() (*tls.Config, error) {
	san := os.Getenv("DNS_SAN")
	cfKey := os.Getenv("CLOUDFLARE_API_KEY")
	privKeyB64 := os.Getenv("TLS_PRIVKEY")
	if san == "" || cfKey == "" || privKeyB64 == "" {
		return nil, nil
	}

	privKeyPEM, err := base64.StdEncoding.DecodeString(privKeyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode TLS_PRIVKEY: %w", err)
	}

	privKey, err := parsePEMPrivateKey(privKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TLS_PRIVKEY: %w", err)
	}

	// Reuse existing cert if it is still valid for this SAN
	if certPEM, err := os.ReadFile(certFile); err == nil {
		if tlsCfg, err := tryExistingCert(certPEM, privKeyPEM, san); err == nil {
			fmt.Printf("Using existing certificate from %s\n", certFile)
			return tlsCfg, nil
		}
	}

	fmt.Printf("Obtaining TLS certificate for %s via ACME DNS-01...\n", san)

	// ACME requires a different key for the account than the certificate
	accountKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ACME account key: %w", err)
	}

	user := &acmeUser{key: accountKey}
	config := lego.NewConfig(user)

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create ACME client: %w", err)
	}

	cfConfig := cloudflare.NewDefaultConfig()
	cfConfig.AuthToken = cfKey
	provider, err := cloudflare.NewDNSProviderConfig(cfConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloudflare DNS provider: %w", err)
	}
	if err := client.Challenge.SetDNS01Provider(provider,
		dns01.AddRecursiveNameservers([]string{"1.1.1.1:53", "8.8.8.8:53"}),
	); err != nil {
		return nil, fmt.Errorf("failed to set DNS-01 provider: %w", err)
	}

	reg, err := client.Registration.Register(registration.RegisterOptions{
		TermsOfServiceAgreed: true,
	})
	if err != nil {
		return nil, fmt.Errorf("ACME registration failed: %w", err)
	}
	user.registration = reg

	cert, err := client.Certificate.Obtain(certificate.ObtainRequest{
		Domains:    []string{san},
		Bundle:     true,
		PrivateKey: privKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to obtain certificate: %w", err)
	}

	if err := os.WriteFile(certFile, cert.Certificate, 0644); err != nil {
		return nil, fmt.Errorf("failed to save certificate: %w", err)
	}
	fmt.Printf("Certificate saved to %s\n", certFile)

	tlsCert, err := tls.X509KeyPair(cert.Certificate, cert.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS keypair: %w", err)
	}

	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, nil
}

// tryExistingCert returns a tls.Config if certPEM is valid for san and has
// more than 30 days until expiry.
func tryExistingCert(certPEM, keyPEM []byte, san string) (*tls.Config, error) {
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, err
	}
	if time.Until(leaf.NotAfter) < 30*24*time.Hour {
		return nil, fmt.Errorf("certificate expires soon")
	}
	if err := leaf.VerifyHostname(san); err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{tlsCert}}, nil
}

// parsePEMPrivateKey decodes a PEM-encoded private key (PKCS8, PKCS1, or EC).
func parsePEMPrivateKey(pemBytes []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("unsupported private key type")
}
