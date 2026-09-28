CREATE TABLE IF NOT EXISTS lottery_predictions (
    id           INT          NOT NULL AUTO_INCREMENT COMMENT '自動カウント',
    lottery_type VARCHAR(10)  NOT NULL COMMENT 'loto6 や loto7 などの対象名',
    pattern      VARCHAR(100) NOT NULL COMMENT '数字の選択に使用したパターン名',
    predicted_at DATE         NOT NULL COMMENT 'レコードの作成日',
    numbers      VARCHAR(30)  NOT NULL COMMENT '選択された数字(カンマ区切り)',
    PRIMARY KEY (id),
    KEY idx_type_date (lottery_type, predicted_at)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4;

INSERT INTO lottery_predictions (lottery_type, pattern, predicted_at, numbers) VALUES
    ('loto6', 'random',    '2026-09-03', '5,12,19,23,31,41'),
    ('loto7', 'frequency', '2026-09-04', '2,8,14,20,26,30,35');
