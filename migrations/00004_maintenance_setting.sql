-- +goose Up
-- Maintenance mode, switched from Admin › টিম ও সেটিংস. "until" is shown to people as the expected return time.
INSERT INTO app_settings (key, value) VALUES ('maintenance', '{"enabled": false, "until": null}');

-- +goose Down
DELETE FROM app_settings WHERE key = 'maintenance';
