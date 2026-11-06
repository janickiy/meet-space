package app

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/domain/settings"
)

func TestSettingsSMTPChild(t *testing.T) {
	if os.Getenv("MEETSPACE_SMTP_TEST_CHILD") != "1" {
		t.Skip("isolated TLS subprocess helper")
	}
	port, _ := strconv.Atoi(os.Getenv("MEETSPACE_SMTP_TEST_PORT"))
	cfg := &mailSettingsFixture{values: settings.Values{SMTPEnabled: true}, smtp: settings.SMTPConfig{Enabled: true, Host: "127.0.0.1", Port: port, Username: "fixture-user", Password: "fixture-password", From: "MeetSpace <fixture@example.invalid>", TLSMode: os.Getenv("MEETSPACE_SMTP_TEST_MODE")}}
	provider := settingsEmail{settings: cfg, fallbackMode: "noop"}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := provider.Send(ctx, integrations.EmailMessage{IdempotencyKey: "smtp-fixture", To: "recipient@example.invalid", Subject: "Synthetic verification", Text: "Fixture delivery"}); err != nil {
		t.Fatal(err)
	}
}
func TestSettingsSMTPUsesVerifiedTLSAndAuthentication(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS uses system Keychain roots; this isolated trust-store test runs on Linux")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "SMTP fixture"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ca := filepath.Join(directory, "ca.pem")
	if err = os.WriteFile(ca, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"tls", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			result := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					result <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
				encrypted := mode == "tls"
				if encrypted {
					secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
					if err = secured.Handshake(); err != nil {
						result <- err
						return
					}
					conn = secured
				}
				reader := bufio.NewReader(conn)
				writer := bufio.NewWriter(conn)
				reply := func(text string) { _, _ = writer.WriteString(text + "\r\n"); _ = writer.Flush() }
				reply("220 fixture ESMTP")
				authenticated, delivered := false, false
				for {
					line, e := reader.ReadString('\n')
					if e != nil {
						result <- e
						return
					}
					line = strings.TrimSpace(line)
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if encrypted {
							reply("250-fixture\r\n250 AUTH PLAIN")
						} else {
							reply("250-fixture\r\n250 STARTTLS")
						}
					case line == "STARTTLS":
						reply("220 Ready for TLS")
						secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
						if e = secured.Handshake(); e != nil {
							result <- e
							return
						}
						conn = secured
						encrypted = true
						reader = bufio.NewReader(conn)
						writer = bufio.NewWriter(conn)
					case strings.HasPrefix(line, "AUTH PLAIN "):
						credentials, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
						if !encrypted || e != nil || string(credentials) != "\x00fixture-user\x00fixture-password" {
							result <- fmt.Errorf("invalid fixture authentication")
							return
						}
						authenticated = true
						reply("235 Authentication successful")
					case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
						if !authenticated {
							result <- fmt.Errorf("unauthenticated SMTP envelope")
							return
						}
						reply("250 OK")
					case line == "DATA":
						reply("354 End with dot")
						var body strings.Builder
						for {
							part, e := reader.ReadString('\n')
							if e != nil {
								result <- e
								return
							}
							if strings.TrimSpace(part) == "." {
								break
							}
							body.WriteString(part)
						}
						if !strings.Contains(body.String(), "Fixture delivery") {
							result <- fmt.Errorf("message body missing")
							return
						}
						delivered = true
						reply("250 queued")
					case line == "QUIT":
						reply("221 bye")
						if !delivered {
							result <- fmt.Errorf("message not delivered")
						} else {
							result <- nil
						}
						return
					default:
						result <- fmt.Errorf("unexpected SMTP command")
						return
					}
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSettingsSMTPChild$")
			command.Env = append(os.Environ(), "MEETSPACE_SMTP_TEST_CHILD=1", "MEETSPACE_SMTP_TEST_PORT="+strconv.Itoa(listener.Addr().(*net.TCPAddr).Port), "MEETSPACE_SMTP_TEST_MODE="+mode, "SSL_CERT_FILE="+ca, "SSL_CERT_DIR="+directory)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("SMTP subprocess: %v %s", err, output)
			}
			if err = <-result; err != nil {
				t.Fatal(err)
			}
		})
	}
}
