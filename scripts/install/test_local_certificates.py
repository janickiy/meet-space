"""Generated local certificates must work with strict TLS verification; no live files or sockets."""

import os
from pathlib import Path
import shutil
import ssl
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


@unittest.skipUnless(shutil.which('openssl') and hasattr(ssl, 'VERIFY_X509_STRICT'),
                     'OpenSSL CLI and strict Python TLS verification are required')
class LocalCertificateTest(unittest.TestCase):
    def test_generated_ca_and_leaf_complete_strict_verified_tls_handshake(self):
        with tempfile.TemporaryDirectory(prefix='meetspace-local-cert-test-') as directory:
            certificates = Path(directory) / 'certs'
            environment = {key: os.environ[key] for key in ('PATH', 'HOME', 'LANG') if key in os.environ}
            environment.update(CERT_DIR=str(certificates), HTTPS_CERT_IP='127.0.0.1',
                               HTTPS_CERT_DNS='localhost')
            generated = subprocess.run(['sh', str(ROOT / 'scripts/generate-https-certs.sh')],
                                       env=environment, text=True, capture_output=True, timeout=60)
            self.assertEqual(generated.returncode, 0, generated.stderr)
            server_context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            server_context.load_cert_chain(certificates / 'go-recorder-server.crt',
                                           certificates / 'go-recorder-server.key')
            client_context = ssl.create_default_context(cafile=str(certificates / 'go-recorder-local-ca.pem'))
            client_context.verify_flags |= ssl.VERIFY_X509_STRICT
            self.assertTrue(client_context.check_hostname)
            self.assertEqual(client_context.verify_mode, ssl.CERT_REQUIRED)

            for hostname in ('localhost', '127.0.0.1'):
                with self.subTest(hostname=hostname):
                    client_in, client_out = ssl.MemoryBIO(), ssl.MemoryBIO()
                    server_in, server_out = ssl.MemoryBIO(), ssl.MemoryBIO()
                    client = client_context.wrap_bio(client_in, client_out, server_hostname=hostname)
                    server = server_context.wrap_bio(server_in, server_out, server_side=True)
                    finished = [False, False]
                    for _ in range(100):
                        for index, endpoint in enumerate((client, server)):
                            if not finished[index]:
                                try:
                                    endpoint.do_handshake()
                                    finished[index] = True
                                except ssl.SSLWantReadError:
                                    pass
                        for outgoing, incoming in ((client_out, server_in), (server_out, client_in)):
                            if outgoing.pending:
                                incoming.write(outgoing.read())
                        if all(finished):
                            break
                    self.assertTrue(all(finished), 'Strict TLS handshake did not finish')
                    self.assertIn(('DNS', 'localhost'), client.getpeercert()['subjectAltName'])

            for name in ('go-recorder-local-ca.key', 'go-recorder-server.key'):
                self.assertEqual((certificates / name).stat().st_mode & 0o777, 0o600)


if __name__ == '__main__':
    unittest.main()
