/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	tlsTestCertFile = "tls.crt"
	tlsTestKeyFile  = "tls.key"
)

// fakeRunnableAdder records the runnables a manager would be asked to run
type fakeRunnableAdder struct {
	runnables []manager.Runnable
	err       error
}

func (f *fakeRunnableAdder) Add(r manager.Runnable) error {
	if f.err != nil {
		return f.err
	}
	f.runnables = append(f.runnables, r)
	return nil
}

// generateTestKeypair returns a PEM-encoded self-signed certificate and its private key
func generateTestKeypair(t *testing.T, commonName string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// writeSecretVolume lays out certPEM and keyPEM in dir the way the kubelet projects a Secret:
// the files are symlinks through a ..data symlink that points at a timestamped directory.
// Calling it again swaps ..data atomically, as the kubelet does when the Secret is updated.
func writeSecretVolume(t *testing.T, dir string, certPEM, keyPEM []byte) {
	t.Helper()
	dataDir := fmt.Sprintf("..%d", time.Now().UnixNano())
	require.NoError(t, os.Mkdir(filepath.Join(dir, dataDir), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, dataDir, tlsTestCertFile), certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, dataDir, tlsTestKeyFile), keyPEM, 0o600))

	previous, _ := os.Readlink(filepath.Join(dir, "..data"))
	require.NoError(t, os.Symlink(dataDir, filepath.Join(dir, "..data_tmp")))
	require.NoError(t, os.Rename(filepath.Join(dir, "..data_tmp"), filepath.Join(dir, "..data")))
	if previous != "" {
		require.NoError(t, os.RemoveAll(filepath.Join(dir, previous)))
	}

	for _, name := range []string{tlsTestCertFile, tlsTestKeyFile} {
		link := filepath.Join(dir, name)
		if _, err := os.Lstat(link); errors.Is(err, os.ErrNotExist) {
			require.NoError(t, os.Symlink(filepath.Join("..data", name), link))
		}
	}
}

// startTestServer starts server on port and stops it when the test ends
func startTestServer(t *testing.T, server *Server, port int, scheme string) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- server.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		select {
		case err := <-errCh:
			// Start returns nil after a graceful shutdown
			if err != nil {
				t.Errorf("server exited with error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Log("Warning: server did not shut down within expected time")
		}
	})

	// Wait until the listener is up; any response or TLS error means it accepted the connection
	client := insecureTLSClient(0)
	require.Eventually(t, func() bool {
		resp, err := client.Get(fmt.Sprintf("%s://localhost:%d/health", scheme, port))
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 3*time.Second, 50*time.Millisecond, "server did not start listening")
}

// insecureTLSClient skips certificate verification, as Kubernetes probes do.
// A non-zero maxVersion caps the TLS version the client offers.
func insecureTLSClient(maxVersion uint16) *http.Client {
	return &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, MaxVersion: maxVersion}, //nolint:gosec // test client
			DisableKeepAlives: true,
		},
	}
}

func newTLSTestServer(t *testing.T) (*Server, int) {
	t.Helper()
	port := getTestPort()
	config := &Config{
		Port:             port,
		ReadTimeout:      time.Second,
		WriteTimeout:     time.Second,
		ShutdownTimeout:  time.Second,
		PathRegexPattern: DefaultPathRegexPattern,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(config, &MockJWTHandler{}, &MockCookieHandler{}, logger), port
}

func TestServer_ServesHTTPSHealth_WhenTLSEnabled(t *testing.T) {
	certPEM, keyPEM := generateTestKeypair(t, "authmiddleware")
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	server, port := newTLSTestServer(t)
	server.EnableTLS(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &cert, nil })
	startTestServer(t, server, port, "https")

	resp, err := insecureTLSClient(0).Get(fmt.Sprintf("https://localhost:%d/health", port))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotNil(t, resp.TLS)
	assert.Equal(t, uint16(MinTLSVersion), resp.TLS.Version)

	// Plaintext requests are refused once TLS is on
	plain, err := insecureTLSClient(0).Get(fmt.Sprintf("http://localhost:%d/health", port))
	require.NoError(t, err)
	defer func() { _ = plain.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, plain.StatusCode)
}

func TestServer_RejectsTLSBelowMinimumVersion(t *testing.T) {
	certPEM, keyPEM := generateTestKeypair(t, "authmiddleware")
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)

	server, port := newTLSTestServer(t)
	server.EnableTLS(func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return &cert, nil })
	startTestServer(t, server, port, "https")

	_, err = insecureTLSClient(tls.VersionTLS12).Get(fmt.Sprintf("https://localhost:%d/health", port))
	require.Error(t, err)
}

func TestServer_ServesHTTPOnly_WhenTLSNotEnabled(t *testing.T) {
	server, port := newTLSTestServer(t)
	startTestServer(t, server, port, "http")

	resp, err := insecureTLSClient(0).Get(fmt.Sprintf("http://localhost:%d/health", port))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Nil(t, resp.TLS)

	_, err = insecureTLSClient(0).Get(fmt.Sprintf("https://localhost:%d/health", port))
	require.Error(t, err, "TLS handshake should fail against a plaintext server")
}

func TestEnableTLS_ServesRotatedCertificateWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	firstCert, firstKey := generateTestKeypair(t, "first")
	writeSecretVolume(t, dir, firstCert, firstKey)

	server, port := newTLSTestServer(t)
	server.config.TLSCertFile = filepath.Join(dir, tlsTestCertFile)
	server.config.TLSKeyFile = filepath.Join(dir, tlsTestKeyFile)
	adder := &fakeRunnableAdder{}
	require.NoError(t, enableTLS(adder, server.config, server))
	require.Len(t, adder.runnables, 1, "certificate watcher should be registered with the manager")

	// Run the certificate watcher as the manager would
	ctx, cancel := context.WithCancel(context.Background())
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		_ = adder.runnables[0].Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-watcherDone
	})

	startTestServer(t, server, port, "https")

	servedCommonName := func() string {
		resp, err := insecureTLSClient(0).Get(fmt.Sprintf("https://localhost:%d/health", port))
		if err != nil {
			return ""
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.TLS.PeerCertificates[0].Subject.CommonName
	}
	assert.Equal(t, "first", servedCommonName())

	secondCert, secondKey := generateTestKeypair(t, "second")
	writeSecretVolume(t, dir, secondCert, secondKey)

	// fsnotify usually picks this up immediately; the watcher's 10s poll is the fallback
	require.Eventually(t, func() bool { return servedCommonName() == "second" },
		15*time.Second, 100*time.Millisecond, "rotated certificate was not served")
}

func TestEnableTLS_FailsFastOnBadKeypair(t *testing.T) {
	dir := t.TempDir()
	certPEM, _ := generateTestKeypair(t, "cert")
	_, otherKeyPEM := generateTestKeypair(t, "other")
	certPath := filepath.Join(dir, tlsTestCertFile)
	mismatchedKeyPath := filepath.Join(dir, "mismatched.key")
	require.NoError(t, os.WriteFile(certPath, certPEM, 0o600))
	require.NoError(t, os.WriteFile(mismatchedKeyPath, otherKeyPEM, 0o600))

	tests := []struct {
		name     string
		certFile string
		keyFile  string
	}{
		{name: "missing cert", certFile: filepath.Join(dir, "missing.crt"), keyFile: mismatchedKeyPath},
		{name: "missing key", certFile: certPath, keyFile: filepath.Join(dir, "missing.key")},
		{name: "mismatched keypair", certFile: certPath, keyFile: mismatchedKeyPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newTLSTestServer(t)
			server.config.TLSCertFile = tt.certFile
			server.config.TLSKeyFile = tt.keyFile
			adder := &fakeRunnableAdder{}

			err := enableTLS(adder, server.config, server)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "failed to load TLS keypair")
			assert.Empty(t, adder.runnables)
			assert.Nil(t, server.getCertificate)
		})
	}
}

func TestEnableTLS_ReturnsErrorWhenManagerRejectsWatcher(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM := generateTestKeypair(t, "authmiddleware")
	writeSecretVolume(t, dir, certPEM, keyPEM)

	server, _ := newTLSTestServer(t)
	server.config.TLSCertFile = filepath.Join(dir, tlsTestCertFile)
	server.config.TLSKeyFile = filepath.Join(dir, tlsTestKeyFile)

	err := enableTLS(&fakeRunnableAdder{err: errors.New("manager already started")}, server.config, server)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "manager already started")
	assert.Nil(t, server.getCertificate)
}
