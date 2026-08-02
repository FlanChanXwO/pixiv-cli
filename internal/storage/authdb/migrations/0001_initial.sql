-- 0001_initial.sql
-- v1 initial auth database. Table names are singular; sort_order is never
-- renumbered after account deletion.

CREATE TABLE pixiv_account (
    user_id             INTEGER PRIMARY KEY,
    sort_order          INTEGER NOT NULL UNIQUE,
    username            TEXT NOT NULL,
    refresh_token       BLOB NOT NULL,
    credential_revision INTEGER NOT NULL,
    premium_status      INTEGER NULL,
    premium_checked_at  INTEGER NULL,
    pool_frozen_until   INTEGER NULL,
    pool_last_selected  INTEGER NOT NULL DEFAULT 0,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    CHECK (user_id > 0),
    CHECK (sort_order > 0),
    CHECK (credential_revision > 0),
    CHECK (premium_status IS NULL OR premium_status IN (0, 1)),
    CHECK (pool_last_selected IN (0, 1))
);

CREATE UNIQUE INDEX pixiv_account_one_pool_last_selected
    ON pixiv_account(pool_last_selected)
    WHERE pool_last_selected = 1;

CREATE TABLE fanbox_account (
    user_id             INTEGER PRIMARY KEY,
    sort_order          INTEGER NOT NULL UNIQUE,
    display_name        TEXT NOT NULL,
    creator_id          TEXT NULL,
    session_id          BLOB NOT NULL,
    credential_revision INTEGER NOT NULL,
    validated_at        INTEGER NOT NULL,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    CHECK (user_id > 0),
    CHECK (sort_order > 0),
    CHECK (credential_revision > 0)
);

CREATE TABLE schema_migration (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    checksum   TEXT NOT NULL,
    applied_at INTEGER NOT NULL
);
