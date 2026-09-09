# Server deployment

This deployment uses Docker Compose for the API, migration command, worker,
PostgreSQL, and self-hosted MinIO. The root `.env` is the only operator-managed
environment file.

## Prepare the server

Install Docker Engine with the Compose plugin, clone the repository, and run
the following commands from the repository root:

```sh
cp .env.example .env
```

Replace every placeholder in `.env`. Set `APP_ENV=production`, use HTTPS for
`HTTP_ALLOWED_ORIGINS`, `AUTH_SUCCESS_REDIRECT_URL`, and
`AUTH_EMAIL_LINK_BASE_URL`, and provide real sandbox provider credentials.
The Stellar signing secrets may remain in this one file; Compose passes them
only to the worker container.

Create the internal MinIO certificate files by following
[`MINIO-TLS.md`](MINIO-TLS.md). The certificate must contain `minio` as its
DNS name. The API trusts the matching `ca.crt` through a Docker secret.

## Build and start

Validate the interpolated Compose file first:

```sh
docker compose -f compose.deploy.yaml config --quiet
```

Build the application image and start the stack:

```sh
docker compose -f compose.deploy.yaml build
docker compose -f compose.deploy.yaml up -d
```

Compose starts PostgreSQL, waits for its healthcheck, runs the versioned
migrations once, then starts the API and worker. The API is bound to
`127.0.0.1:${API_PORT}` by default. Put a reverse proxy with public HTTPS in
front of it and proxy to that loopback address.

## Verify the deployment

```sh
docker compose -f compose.deploy.yaml ps
curl --fail http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8080/docs/
docker compose -f compose.deploy.yaml logs --tail=100 api
docker compose -f compose.deploy.yaml logs --tail=100 worker
```

The API container does not receive either Stellar signing secret. The worker
receives them from the unified `.env` and submits settlement work. PostgreSQL
and MinIO have no host ports in this stack. Use an SSH tunnel for temporary
MinIO console access instead of exposing its admin port.

## Operations

Apply a later migration by rebuilding the image and running the migration
service before restarting the API:

```sh
docker compose -f compose.deploy.yaml build
docker compose -f compose.deploy.yaml up migrate
docker compose -f compose.deploy.yaml up -d api worker
```

Back up PostgreSQL with `pg_dump` and back up the `minio_data` volume before
upgrades. Do not use `docker compose down -v` on a server unless you intend to
delete the database and object-storage data.
