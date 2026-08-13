-- 0003_pixiv_account_schedulable.sql
-- 既有账号默认参与调度；后续 pool 管理通过该列表达 membership。

ALTER TABLE pixiv_account
    ADD COLUMN schedulable INTEGER NOT NULL DEFAULT 1
    CHECK (schedulable IN (0, 1));
