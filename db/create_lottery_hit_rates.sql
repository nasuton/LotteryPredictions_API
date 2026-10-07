-- ロト種別・予想パターンごとの最新ヒット率を保存するテーブル。
-- 対象データベースを選択してから実行する。
-- 例: mysql -u <user> -p <database> < db/create_lottery_hit_rates.sql
--
-- CSVの割合は '%' を除いた数値で登録する（例: '12.3%' -> 12.3）。
-- loto6: match_7_rate = NULL
-- miniloto: match_6_rate = NULL, match_7_rate = NULL
-- 個別パターンの登録時は、CSV末尾の '【全体】' 行を除外する。
-- 登録処理では指定 lottery_type の既存行を削除してから、今回のCSVを登録する。
-- CSVを検証し、削除・登録を同じトランザクションで行う。

CREATE TABLE IF NOT EXISTS `lottery_hit_rates` (
    `lottery_type`     VARCHAR(16)  NOT NULL COMMENT 'loto6 / loto7 / miniloto',
    `pattern`          VARCHAR(255) NOT NULL COMMENT '予想パターン',
    `prediction_count` INT UNSIGNED NOT NULL COMMENT '集計対象の予想件数',
    `mean_hits`        DECIMAL(4,2) NOT NULL COMMENT '平均一致数',
    `hit_rate`         DECIMAL(5,2) NOT NULL COMMENT '3個以上一致率（0～100）',
    `match_3_rate`     DECIMAL(5,2) NOT NULL COMMENT '3個一致率（0～100）',
    `match_4_rate`     DECIMAL(5,2) NOT NULL COMMENT '4個一致率（0～100）',
    `match_5_rate`     DECIMAL(5,2) NOT NULL COMMENT '5個一致率（0～100）',
    `match_6_rate`     DECIMAL(5,2) NULL DEFAULT NULL COMMENT '6個一致率（minilotoは対象外）',
    `match_7_rate`     DECIMAL(5,2) NULL DEFAULT NULL COMMENT '7個一致率（loto7のみ対象）',
    `created_at`       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`lottery_type`, `pattern`)
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='ロト種別・予想パターンごとの最新ヒット率';
