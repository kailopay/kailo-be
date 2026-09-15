# KailoPay Postman collection

Import these files into Postman:

- `KailoPay.postman_collection.json`
- `KailoPay.postman_environment.json`

Select **KailoPay local sandbox** as the active environment and set `base_url`
if the API is not running at `http://localhost:8080`.

## Recommended run order

1. Register, verify the email from the API/worker output, then Login.
2. Run **Persona KYC → Start or resume Persona inquiry**.
3. Complete the real Persona sandbox inquiry in the frontend. Persona must be
   configured to deliver events to `/callbacks/kyc/persona` through an HTTPS
   public URL.
4. Run **Persona KYC → Get KYC status** and confirm `approved`.
5. Run **Auth and Profile → Update profile** once to enable Developer Mode.
6. Run **API Keys → Create sandbox API key**. The test script stores the one-time
   plaintext key in `api_key`.
7. Run the Orders and SEP-24 requests with the captured key.

The synthetic Persona callback is useful for local signature and state tests.
Set `persona_webhook_secret` to the configured secret and run it only after an
inquiry has been created. The callback body is signed by the request's
pre-request script. Change `persona_event_id` when you want a new event rather
than a replay.

Callback fixtures do not create provider payment state. Use the real Xendit
sandbox callback or provider dashboard fixture for payment evidence.

Never commit the exported environment after entering secrets. Do not publish
session cookies, API keys, webhook secrets, identity documents, or raw provider
payloads.
