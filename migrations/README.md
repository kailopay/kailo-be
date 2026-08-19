# Database migrations

`cmd/automigrate` is intentionally limited to `local` and `test` environments
for fast bootstrap. It is not a production migration mechanism.

Shared and production databases must use reviewed, ordered migration files in
this directory. Keep schema changes backward-compatible with the deployed
application, test them against an empty PostgreSQL database, and deploy them
before code that depends on the new schema.
