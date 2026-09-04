-- Second factors: authenticator codes and passkeys.
--
-- Both are optional and independent. A passkey is a full sign-in on its own —
-- the authenticator has already checked a fingerprint or a PIN — so it does not
-- ask for a code afterwards. An authenticator code is a second step after a
-- password, and never applies to an OAuth sign-in, where Google has already
-- done the checking.

-- The shared secret, encrypted at rest with the same key that protects the AI
-- provider keys. Without ENCRYPTION_KEY set, two-factor cannot be enrolled at
-- all; a secret in the clear in the database is the whole attack.
ALTER TABLE users ADD COLUMN totp_secret_enc TEXT NOT NULL DEFAULT '';

-- Enrolment is two steps, and this is the second. A secret is generated and
-- shown as a QR code before it is proven, and until a code from the phone has
-- verified it the account must keep signing in without one — otherwise a
-- mis-scanned QR locks you out of your own account.
ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN totp_confirmed_at DATETIME;

-- Recovery codes, stored as hashes. Shown once at enrolment; a lost phone is
-- the ordinary case, not the exceptional one.
CREATE TABLE user_recovery_codes (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT    NOT NULL,
    -- Set the moment a code is spent. A recovery code works exactly once.
    used_at    DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_recovery_codes_user ON user_recovery_codes(user_id, used_at);

-- Registered authenticators.
CREATE TABLE user_passkeys (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- The credential id the browser hands back. Unique across every account:
    -- one authenticator cannot belong to two people.
    credential_id    BLOB    NOT NULL UNIQUE,
    public_key       BLOB    NOT NULL,
    -- Only ever moves forward on a genuine authenticator, which is what makes
    -- a cloned credential detectable.
    sign_count       INTEGER NOT NULL DEFAULT 0,
    -- True for a credential synced to a password manager rather than bound to
    -- one device, which decides whether losing the device loses the key.
    backed_up        INTEGER NOT NULL DEFAULT 0,
    transports       TEXT    NOT NULL DEFAULT '',
    attestation_type TEXT    NOT NULL DEFAULT '',
    -- A name the owner can recognise: "MacBook Touch ID", "YubiKey".
    name             TEXT    NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at     DATETIME
);

CREATE INDEX idx_passkeys_user ON user_passkeys(user_id);

-- WebAuthn ceremonies are two round trips, and the challenge issued by the
-- first has to survive until the second.
--
-- Held server-side rather than handed to the browser: the challenge is what
-- the signature is checked against, so letting the client carry it back would
-- let the client choose it. A sign-in ceremony has no user yet — a
-- discoverable credential says who it is only at the end — so user_id is null
-- there.
CREATE TABLE passkey_sessions (
    id         TEXT    PRIMARY KEY,
    user_id    INTEGER REFERENCES users(id) ON DELETE CASCADE,
    data       BLOB    NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_passkey_sessions_expiry ON passkey_sessions(expires_at);
