package tls

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	"github.com/Ehco1996/ehco/pkg/log"
	"go.uber.org/zap"
)

var (
	L *zap.SugaredLogger
)

func init() {
	L = log.InfoLogger.Named("tls")
}

// pre built in tls cert
var (
	CertFileName     = os.Getenv("EHCO_CERT_FILE_NAME")
	KeyFileName      = os.Getenv("EHCO_KEY_FILE_NAME")
	DefaultTLSConfig *tls.Config
)

func InitTlsCfg() {
	if DefaultTLSConfig != nil {
		return
	}
	cert, err := genCertificate()
	if err != nil {
		panic(err)
	}
	DefaultTLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	}
}

func genCertificate() (cert tls.Certificate, err error) {
	rawCert, rawKey, err := generateKeyPair()
	if err != nil {
		return
	}
	cert, err = tls.X509KeyPair(rawCert, rawKey)
	return cert, err
}

func generateKeyPair() (rawCert, rawKey []byte, err error) {
	// Create private key and self-signed certificate
	// Adapted from https://golang.org/src/crypto/tls/generate_cert.go

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return
	}
	validFor := time.Hour * 24 * 365 * 1
	notBefore := time.Now()
	notAfter := notBefore.Add(validFor)
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, _ := rand.Int(rand.Reader, serialNumberLimit)
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"ehco"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return
	}

	rawCert = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	rawKey = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	if CertFileName != "" {
		certOut, err := os.Create(CertFileName)
		if err != nil {
			L.Fatalf("failed to open cert.pem for writing: %s", err)
		}
		if err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
			L.Info("failed to pem encode:", err)
		}
		certOut.Close()
		L.Infof("write cert to %s", CertFileName)
	}
	if KeyFileName != "" {
		keyOut, err := os.OpenFile(KeyFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			L.Info("failed to open key.pem for writing:", err)
		}
		if err = pem.Encode(keyOut, pemBlockForKey(priv)); err != nil {
			L.Info("failed to pem encode:", err)
		}
		keyOut.Close()
		L.Infof("write key to %s", KeyFileName)
	}
	return
}

func pemBlockForKey(priv interface{}) *pem.Block {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			L.Infof("Unable to marshal ECDSA private key: %v", err)
			os.Exit(2)
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}
	default:
		return nil
	}
}
