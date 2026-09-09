# Self-hosted MinIO certificates

The deployment Compose file keeps MinIO on the private Docker network and
requires TLS because the backend rejects plaintext object storage outside
`APP_ENV=local`.

Run these commands on the deployment server from the repository root. The
certificate uses `minio` as its DNS name because the API connects to the
Compose service at `minio:9000`.

```sh
mkdir -p deploy/minio/certs

openssl req -x509 -nodes -newkey rsa:4096 -days 3650 \
  -keyout deploy/minio/certs/private.key \
  -out deploy/minio/certs/public.crt \
  -subj "/CN=minio" \
  -addext "subjectAltName=DNS:minio"

cp deploy/minio/certs/public.crt deploy/minio/certs/ca.crt
chmod 600 deploy/minio/certs/private.key
chmod 644 deploy/minio/certs/public.crt deploy/minio/certs/ca.crt
```

`public.crt`, `private.key`, and `ca.crt` are ignored by Git. The API trusts
`ca.crt` through the Compose secret mounted in the container's system
certificate directory at `/etc/ssl/certs/minio-ca.crt`. Do not expose MinIO's
ports to the public internet. Use an SSH tunnel if you need temporary access
to the MinIO console.
