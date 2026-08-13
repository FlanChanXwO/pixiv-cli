-- 0002_fanbox_creator_id_not_null.sql
-- 将旧 schema 中合法的 NULL creator_id 收敛为公开模型使用的空字符串。

CREATE TABLE fanbox_account_v2 (
    user_id             INTEGER PRIMARY KEY,
    sort_order          INTEGER NOT NULL UNIQUE,
    display_name        TEXT NOT NULL,
    creator_id          TEXT NOT NULL DEFAULT '',
    session_id          BLOB NOT NULL,
    credential_revision INTEGER NOT NULL,
    validated_at        INTEGER NOT NULL,
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    CHECK (user_id > 0),
    CHECK (sort_order > 0),
    CHECK (credential_revision > 0)
);

INSERT INTO fanbox_account_v2 (
    user_id, sort_order, display_name, creator_id, session_id,
    credential_revision, validated_at, created_at, updated_at
)
SELECT
    user_id, sort_order, display_name, COALESCE(creator_id, ''), session_id,
    credential_revision, validated_at, created_at, updated_at
FROM fanbox_account;

DROP TABLE fanbox_account;
ALTER TABLE fanbox_account_v2 RENAME TO fanbox_account;
