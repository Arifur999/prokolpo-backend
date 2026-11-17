-- +goose Up
-- Design v2: the owner can open a manager's login while creating a project. That account
-- belongs to the owner: managed_by names them, and password_locked keeps its password in
-- the owner's hands (the manager can't change or reset it themselves).
ALTER TABLE users ADD COLUMN managed_by uuid REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN password_locked boolean NOT NULL DEFAULT false;

-- Whether a manager may add their own staff to the project.
ALTER TABLE project_members ADD COLUMN can_add_staff boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE project_members DROP COLUMN can_add_staff;
ALTER TABLE users DROP COLUMN password_locked;
ALTER TABLE users DROP COLUMN managed_by;
