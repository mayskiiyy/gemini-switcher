# Security

Gemini Switcher handles authenticated Antigravity sessions. The local account
pool contains credential blobs in plaintext, including its backup file. Windows
Credential Manager protects the active credential; it does not encrypt the
saved account pool.

Do not upload account files, backups, tokens, credential exports, or screenshots
containing private account information to issues or pull requests. Redact logs
before sharing them. Keep the data directory out of shared and cloud-synced
folders.

Report vulnerabilities without secrets in the report. For sensitive details,
contact the maintainer privately through a channel listed on their profile.

This project is an independent utility and is not affiliated with Google.
Antigravity integrations may change between releases. No external security
audit has been performed.
